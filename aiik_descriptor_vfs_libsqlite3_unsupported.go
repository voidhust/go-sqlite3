//go:build aiik_descriptor_vfs && libsqlite3 && cgo && (darwin || linux)

package sqlite3

/*
#error "aiik_descriptor_vfs requires the fork-bundled SQLite amalgamation; libsqlite3 is unsupported"
*/
import "C"
