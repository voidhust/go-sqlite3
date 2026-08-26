//go:build aiik_descriptor_vfs && cgo && (darwin || linux) && !libsqlite3

package sqlite3

/*
#cgo CFLAGS: -DSQLITE_AIIK_DESCRIPTOR_VFS
#ifndef USE_LIBSQLITE3
#include "sqlite3-binding.h"
#else
#include <sqlite3.h>
#endif
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"time"
)

// AIIKDescriptorFile identifies a borrowed Unix regular-file descriptor.
// Every field is mandatory and must exactly match the descriptor at open
// time. The fork validates and duplicates the descriptor synchronously; it
// never closes the caller's descriptor.
type AIIKDescriptorFile struct {
	FD        int
	Device    uint64
	Inode     uint64
	LinkCount uint64
	FileType  uint32
}

// AIIKDescriptorSource is the fixed source endpoint. A source always binds
// its database, WAL, and SHM descriptors; omitted sidecars fail closed.
type AIIKDescriptorSource struct {
	Database AIIKDescriptorFile
	WAL      AIIKDescriptorFile
	SHM      AIIKDescriptorFile
}

// AIIKDescriptorDestination is the fixed exclusive destination endpoint.
// Auxiliary destination files are deliberately unsupported in this slice.
type AIIKDescriptorDestination struct {
	Database AIIKDescriptorFile
}

// AIIKDescriptorIdentity is a value-only endpoint identity.
type AIIKDescriptorIdentity struct {
	Device    uint64
	Inode     uint64
	LinkCount uint64
	FileType  uint32
}

// AIIKUnixConnectionInfo is a value-only descriptor-VFS report. It contains
// neither a locator, VFS routing token, nor an owned descriptor.
type AIIKUnixConnectionInfo struct {
	DescriptorVFS bool
	Database      AIIKDescriptorIdentity
	Filesystem    string
	LockingMethod string
}

func aiikValidateDescriptor(file AIIKDescriptorFile) error {
	cfile := C.sqlite3_aiik_descriptor_file{
		fd:         C.int(file.FD),
		device:     C.sqlite3_uint64(file.Device),
		inode:      C.sqlite3_uint64(file.Inode),
		link_count: C.sqlite3_uint64(file.LinkCount),
		file_type:  C.uint(file.FileType),
	}
	if rc := C.sqlite3_aiik_descriptor_validate(&cfile, 1); rc != C.SQLITE_OK {
		return Error{Code: ErrNo(rc)}
	}
	return nil
}

func aiikOpenDescriptor(files []AIIKDescriptorFile, kind C.int) (*SQLiteConn, error) {
	if len(files) != 1 && len(files) != 3 {
		return nil, errors.New("sqlite3: invalid AIIK descriptor endpoint")
	}
	var cfiles [3]C.sqlite3_aiik_descriptor_file
	for i, file := range files {
		cfiles[i] = C.sqlite3_aiik_descriptor_file{
			fd:         C.int(file.FD),
			device:     C.sqlite3_uint64(file.Device),
			inode:      C.sqlite3_uint64(file.Inode),
			link_count: C.sqlite3_uint64(file.LinkCount),
			file_type:  C.uint(file.FileType),
		}
	}
	var db *C.sqlite3
	if rc := C.sqlite3_aiik_descriptor_open(&cfiles[0], kind, &db); rc != C.SQLITE_OK {
		return nil, Error{Code: ErrNo(rc)}
	}
	if db == nil {
		return nil, errors.New("sqlite3: descriptor VFS succeeded without returning a database")
	}
	conn := &SQLiteConn{db: db, loc: time.Local, txlock: "BEGIN"}
	runtime.SetFinalizer(conn, (*SQLiteConn).Close)
	return conn, nil
}

// OpenAIIKDescriptorSource opens a complete, exact source endpoint through
// the fork-private descriptor VFS. It bypasses DSN and URI parsing.
func OpenAIIKDescriptorSource(source AIIKDescriptorSource) (*SQLiteConn, error) {
	return aiikOpenDescriptor(
		[]AIIKDescriptorFile{source.Database, source.WAL, source.SHM},
		C.AIIK_DESCRIPTOR_SOURCE,
	)
}

// OpenAIIKDescriptorDestination opens only the pre-created destination main
// database. The private VFS rejects every destination auxiliary file.
func OpenAIIKDescriptorDestination(destination AIIKDescriptorDestination) (*SQLiteConn, error) {
	return aiikOpenDescriptor(
		[]AIIKDescriptorFile{destination.Database},
		C.AIIK_DESCRIPTOR_DESTINATION,
	)
}

// InspectAIIKUnixConnection returns value-only metadata for an active private
// descriptor-VFS connection.
func InspectAIIKUnixConnection(conn *SQLiteConn) (AIIKUnixConnectionInfo, error) {
	if conn == nil || conn.db == nil {
		return AIIKUnixConnectionInfo{}, errors.New("sqlite3: nil AIIK Unix connection")
	}
	var info C.sqlite3_aiik_descriptor_info
	if rc := C.sqlite3_aiik_descriptor_inspect(conn.db, &info); rc != C.SQLITE_OK {
		return AIIKUnixConnectionInfo{}, Error{Code: ErrNo(rc)}
	}
	return AIIKUnixConnectionInfo{
		DescriptorVFS: info.descriptor_vfs != 0,
		Database: AIIKDescriptorIdentity{
			Device:    uint64(info.device),
			Inode:     uint64(info.inode),
			LinkCount: uint64(info.link_count),
			FileType:  uint32(info.file_type),
		},
		Filesystem:    aiikFilesystemName(uint32(info.filesystem)),
		LockingMethod: aiikLockingMethodName(uint32(info.locking_method)),
	}, nil
}

func aiikFilesystemName(value uint32) string {
	if runtime.GOOS == "darwin" && value == 1 {
		return "apfs"
	}
	if value == 0 {
		return "unknown"
	}
	return fmt.Sprintf("unix-fstype-0x%x", value)
}

func aiikLockingMethodName(value uint32) string {
	if value == 1 {
		return "posix"
	}
	return "unknown"
}

func aiikTestForbiddenOpen() error {
	if rc := C.sqlite3_aiik_descriptor_test_forbidden_open(); rc != C.SQLITE_OK {
		return Error{Code: ErrNo(rc)}
	}
	return nil
}

func aiikTestInjectEarlyFailure(enabled bool) {
	value := C.int(0)
	if enabled {
		value = 1
	}
	C.sqlite3_aiik_descriptor_test_inject_early_failure(value)
}

func aiikTestOutstandingDuplicates() int {
	return int(C.sqlite3_aiik_descriptor_test_outstanding_duplicates())
}

func aiikTestConsumeWAL(conn *SQLiteConn) error {
	if rc := C.sqlite3_aiik_descriptor_test_consume_wal(conn.db); rc != C.SQLITE_OK {
		return Error{Code: ErrNo(rc)}
	}
	return nil
}

func aiikTestAdoptSHM(conn *SQLiteConn) error {
	if rc := C.sqlite3_aiik_descriptor_test_adopt_shm(conn.db); rc != C.SQLITE_OK {
		return Error{Code: ErrNo(rc)}
	}
	return nil
}

func aiikTestMismatchSHM(conn *SQLiteConn) error {
	if rc := C.sqlite3_aiik_descriptor_test_mismatch_shm(conn.db); rc != C.SQLITE_OK {
		return Error{Code: ErrNo(rc)}
	}
	return nil
}

func aiikTestHoldStockSHM(conn *SQLiteConn) error {
	if rc := C.sqlite3_aiik_descriptor_test_hold_stock_shm(conn.db); rc != C.SQLITE_OK {
		return Error{Code: ErrNo(rc)}
	}
	return nil
}

func aiikTestReleaseStockSHM(conn *SQLiteConn) error {
	if rc := C.sqlite3_aiik_descriptor_test_release_stock_shm(conn.db); rc != C.SQLITE_OK {
		return Error{Code: ErrNo(rc)}
	}
	return nil
}
