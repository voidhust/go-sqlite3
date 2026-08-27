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
	"strconv"
	"strings"
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
	// Anchor is the value-only inspection of the retained stock Unix
	// connection that owns the live source endpoint.
	Anchor AIIKUnixConnectionInfo
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
	WALPresent    bool
	WAL           AIIKDescriptorIdentity
	SHMPresent    bool
	SHM           AIIKDescriptorIdentity
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

func aiikIdentityFromC(value C.sqlite3_aiik_descriptor_identity) AIIKDescriptorIdentity {
	return AIIKDescriptorIdentity{
		Device:    uint64(value.device),
		Inode:     uint64(value.inode),
		LinkCount: uint64(value.link_count),
		FileType:  uint32(value.file_type),
	}
}

func aiikAnchorInfo(source *AIIKDescriptorSource) C.sqlite3_aiik_descriptor_info {
	if source == nil {
		return C.sqlite3_aiik_descriptor_info{}
	}
	anchor := source.Anchor
	info := C.sqlite3_aiik_descriptor_info{
		descriptor_vfs: C.int(0),
		database: C.sqlite3_aiik_descriptor_identity{
			device: C.sqlite3_uint64(anchor.Database.Device), inode: C.sqlite3_uint64(anchor.Database.Inode),
			link_count: C.sqlite3_uint64(anchor.Database.LinkCount), file_type: C.uint(anchor.Database.FileType),
		},
		wal_present: C.int(0),
		wal: C.sqlite3_aiik_descriptor_identity{
			device: C.sqlite3_uint64(anchor.WAL.Device), inode: C.sqlite3_uint64(anchor.WAL.Inode),
			link_count: C.sqlite3_uint64(anchor.WAL.LinkCount), file_type: C.uint(anchor.WAL.FileType),
		},
		shm_present: C.int(0),
		shm: C.sqlite3_aiik_descriptor_identity{
			device: C.sqlite3_uint64(anchor.SHM.Device), inode: C.sqlite3_uint64(anchor.SHM.Inode),
			link_count: C.sqlite3_uint64(anchor.SHM.LinkCount), file_type: C.uint(anchor.SHM.FileType),
		},
		filesystem:     C.uint(aiikFilesystemValue(anchor.Filesystem)),
		locking_method: C.uint(aiikLockingMethodValue(anchor.LockingMethod)),
	}
	if anchor.DescriptorVFS {
		info.descriptor_vfs = 1
	}
	return info
}

func aiikOpenDescriptor(files []AIIKDescriptorFile, kind C.int, source *AIIKDescriptorSource) (*SQLiteConn, error) {
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
	anchor := aiikAnchorInfo(source)
	if source != nil {
		if source.Anchor.WALPresent {
			anchor.wal_present = 1
		}
		if source.Anchor.SHMPresent {
			anchor.shm_present = 1
		}
	}
	var db *C.sqlite3
	if rc := C.sqlite3_aiik_descriptor_open(&cfiles[0], kind, &anchor, &db); rc != C.SQLITE_OK {
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
		&source,
	)
}

// OpenAIIKDescriptorDestination opens only the pre-created destination main
// database. The private VFS rejects every destination auxiliary file.
func OpenAIIKDescriptorDestination(destination AIIKDescriptorDestination) (*SQLiteConn, error) {
	return aiikOpenDescriptor(
		[]AIIKDescriptorFile{destination.Database},
		C.AIIK_DESCRIPTOR_DESTINATION,
		nil,
	)
}

// InspectAIIKUnixConnection returns value-only metadata for an active Unix
// connection, including a retained stock anchor.
func InspectAIIKUnixConnection(conn *SQLiteConn) (AIIKUnixConnectionInfo, error) {
	if conn == nil {
		return AIIKUnixConnectionInfo{}, errors.New("sqlite3: nil AIIK Unix connection")
	}
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if conn.db == nil {
		return AIIKUnixConnectionInfo{}, errors.New("sqlite3: nil AIIK Unix connection")
	}
	var info C.sqlite3_aiik_descriptor_info
	if rc := C.sqlite3_aiik_descriptor_inspect(conn.db, &info); rc != C.SQLITE_OK {
		return AIIKUnixConnectionInfo{}, Error{Code: ErrNo(rc)}
	}
	return AIIKUnixConnectionInfo{
		DescriptorVFS: info.descriptor_vfs != 0,
		Database:      aiikIdentityFromC(info.database),
		WALPresent:    info.wal_present != 0,
		WAL:           aiikIdentityFromC(info.wal),
		SHMPresent:    info.shm_present != 0,
		SHM:           aiikIdentityFromC(info.shm),
		Filesystem:    aiikFilesystemName(uint32(info.filesystem)),
		LockingMethod: aiikLockingMethodName(uint32(info.locking_method)),
	}, nil
}

func aiikFilesystemValue(value string) uint32 {
	if runtime.GOOS == "darwin" && value == "apfs" {
		return 1
	}
	const prefix = "unix-fstype-0x"
	if strings.HasPrefix(value, prefix) {
		parsed, err := strconv.ParseUint(strings.TrimPrefix(value, prefix), 16, 32)
		if err == nil {
			return uint32(parsed)
		}
	}
	return 0
}

func aiikLockingMethodValue(value string) uint32 {
	if value == "posix" {
		return 1
	}
	return 0
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
