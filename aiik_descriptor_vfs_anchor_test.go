//go:build aiik_descriptor_vfs && cgo && (darwin || linux) && !libsqlite3

package sqlite3

import (
	"path/filepath"
	"testing"
)

func TestAIIKUnixConnectionInspectionReportsStockAnchorValues(t *testing.T) {
	conn, err := (&SQLiteDriver{}).Open(filepath.Join(t.TempDir(), "anchor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	stock := conn.(*SQLiteConn)
	if _, err := stock.Exec("PRAGMA journal_mode=WAL", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := stock.Exec("CREATE TABLE anchor_fixture (id INTEGER)", nil); err != nil {
		t.Fatal(err)
	}
	info, err := InspectAIIKUnixConnection(stock)
	if err != nil {
		t.Fatal(err)
	}
	if info.DescriptorVFS || !info.WALPresent || !info.SHMPresent || info.Database.Inode == 0 || info.WAL.Inode == 0 || info.SHM.Inode == 0 || info.Filesystem != "apfs" || info.LockingMethod != "posix" {
		t.Fatalf("unexpected value-only stock anchor report: %#v", info)
	}
}
