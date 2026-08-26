//go:build aiik_descriptor_vfs && cgo && (darwin || linux)

package sqlite3

/*
#ifndef USE_LIBSQLITE3
#include "sqlite3-binding.h"
#else
#include <sqlite3.h>
#endif
*/
import "C"

import (
	"errors"
)

// AIIKDescriptorFile identifies a borrowed Unix regular-file descriptor.
// A zero expected identity field means "do not assert this field". FD is
// always required. The fork validates the descriptor synchronously and never
// closes the caller's descriptor.
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

// AIIKUnixConnectionInfo is reserved for the tagged fork adapter. It is a
// value-only report so callers never receive a locator or an owned descriptor.
type AIIKUnixConnectionInfo struct {
	DescriptorVFS bool
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

// OpenAIIKDescriptorSource validates the complete fixed source endpoint.
// Descriptor-backed opening remains disabled until the later synthetic WAL
// fixture establishes the required macOS/Linux selector behavior; this
// function intentionally fails rather than falling back to a pathname, DSN,
// or default VFS.
func OpenAIIKDescriptorSource(source AIIKDescriptorSource) (*SQLiteConn, error) {
	for _, file := range [...]AIIKDescriptorFile{source.Database, source.WAL, source.SHM} {
		if err := aiikValidateDescriptor(file); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("sqlite3: AIIK descriptor VFS binding is unavailable pending WAL selector validation")
}

// OpenAIIKDescriptorDestination validates the fixed destination endpoint and
// fails closed until the later destination-auxiliary feasibility fixture.
func OpenAIIKDescriptorDestination(destination AIIKDescriptorDestination) (*SQLiteConn, error) {
	if err := aiikValidateDescriptor(destination.Database); err != nil {
		return nil, err
	}
	return nil, errors.New("sqlite3: AIIK descriptor VFS binding is unavailable pending destination auxiliary validation")
}

// InspectAIIKUnixConnection refuses a nil connection. Detailed inspection is
// intentionally unavailable before an active descriptor-bound connection can
// exist; it exposes neither a pathname nor a raw file descriptor.
func InspectAIIKUnixConnection(conn *SQLiteConn) (AIIKUnixConnectionInfo, error) {
	if conn == nil || conn.db == nil {
		return AIIKUnixConnectionInfo{}, errors.New("sqlite3: nil AIIK Unix connection")
	}
	return AIIKUnixConnectionInfo{}, errors.New("sqlite3: AIIK descriptor VFS inspection is unavailable without a descriptor-bound connection")
}
