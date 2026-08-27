//go:build aiik_descriptor_vfs && aiik_descriptor_vfs_test && cgo && (darwin || linux) && !libsqlite3

package sqlite3

/*
#cgo CFLAGS: -DSQLITE_AIIK_DESCRIPTOR_VFS -DSQLITE_AIIK_DESCRIPTOR_VFS_TEST
#include "sqlite3-binding.h"
*/
import "C"

func aiikTestForbiddenOpen(conn *SQLiteConn) error {
	if rc := C.sqlite3_aiik_descriptor_test_forbidden_open(conn.db); rc != C.SQLITE_OK {
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

func aiikTestMainLifecycle(conn *SQLiteConn) error {
	if rc := C.sqlite3_aiik_descriptor_test_main_lifecycle(conn.db); rc != C.SQLITE_OK {
		return Error{Code: ErrNo(rc)}
	}
	return nil
}

func aiikTestTokenCollision(enabled bool, attempts int) {
	value := C.int(0)
	if enabled {
		value = 1
	}
	C.sqlite3_aiik_descriptor_test_token_collision(value, C.int(attempts))
}
