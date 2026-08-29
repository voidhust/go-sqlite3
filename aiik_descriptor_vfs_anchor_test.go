//go:build aiik_descriptor_vfs && cgo && (darwin || linux) && !libsqlite3

package sqlite3

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
)

func TestAIIKUnixConnectionInspectionReportsStockAnchorValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anchor.db")
	conn, err := (&SQLiteDriver{}).Open(path)
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
	if info.DescriptorVFS || !info.WALPresent || !info.SHMPresent || info.Filesystem == "unknown" || info.LockingMethod != "posix" {
		t.Fatalf("unexpected value-only stock anchor report: %#v", info)
	}
	for _, check := range []struct {
		path string
		got  AIIKDescriptorIdentity
	}{
		{path, info.Database},
		{path + "-wal", info.WAL},
		{path + "-shm", info.SHM},
	} {
		file, err := os.Open(check.path)
		if err != nil {
			t.Fatalf("open %s for independent fstat: %v", check.path, err)
		}
		var stat syscall.Stat_t
		err = syscall.Fstat(int(file.Fd()), &stat)
		_ = file.Close()
		if err != nil {
			t.Fatalf("fstat %s: %v", check.path, err)
		}
		if check.got.Device != uint64(stat.Dev) || check.got.Inode != uint64(stat.Ino) || check.got.LinkCount != uint64(stat.Nlink) || check.got.FileType != uint32(stat.Mode&syscall.S_IFMT) {
			t.Fatalf("inspection identity for %s = %#v, want independent fstat %#v", check.path, check.got, stat)
		}
	}
}

func TestAIIKUnixConnectionInspectionSynchronizesClose(t *testing.T) {
	conn, err := (&SQLiteDriver{}).Open(filepath.Join(t.TempDir(), "close.db"))
	if err != nil {
		t.Fatal(err)
	}
	stock := conn.(*SQLiteConn)
	started := make(chan struct{})
	var inspectors sync.WaitGroup
	inspectors.Add(1)
	go func() {
		defer inspectors.Done()
		close(started)
		for i := 0; i < 100; i++ {
			if _, err := InspectAIIKUnixConnection(stock); err != nil {
				return
			}
		}
	}()
	<-started
	if err := stock.Close(); err != nil {
		t.Fatal(err)
	}
	inspectors.Wait()
}

func TestAIIKUnixConnectionInspectionRejectsNonExactStockProfiles(t *testing.T) {
	for _, profile := range []string{"unix-excl", "unix-none", "unix-dotfile"} {
		t.Run(profile, func(t *testing.T) {
			conn, err := (&SQLiteDriver{}).Open("file:" + filepath.Join(t.TempDir(), "profile.db") + "?vfs=" + profile)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if _, err := InspectAIIKUnixConnection(conn.(*SQLiteConn)); err == nil {
				t.Fatalf("inspection accepted non-exact stock profile %q", profile)
			}
		})
	}
}

func TestAIIKUnixConnectionInspectionRejectsReadOnlyWALAnchor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly.db")
	writerRaw, err := (&SQLiteDriver{}).Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writerRaw.Close()
	writer := writerRaw.(*SQLiteConn)
	if _, err := writer.Exec("PRAGMA journal_mode=WAL", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec("CREATE TABLE readonly_fixture (id INTEGER)", nil); err != nil {
		t.Fatal(err)
	}

	readerRaw, err := (&SQLiteDriver{}).Open("file:" + path + "?mode=ro&vfs=unix")
	if err != nil {
		t.Fatal(err)
	}
	defer readerRaw.Close()
	if _, err := InspectAIIKUnixConnection(readerRaw.(*SQLiteConn)); err == nil {
		t.Fatal("inspection accepted read-only stock unix WAL anchor")
	}
}

func TestAIIKDescriptorLinuxFilesystemCapabilityFailsClosed(t *testing.T) {
	if !aiikTestLinuxFilesystemAllowed(0xEF53) {
		t.Fatal("descriptor VFS rejected reviewed ext4 filesystem")
	}
	for _, filesystem := range []uint32{0x6969, 0xFF534D42, 0x794C7630} { // NFS, CIFS, overlay
		if aiikTestLinuxFilesystemAllowed(filesystem) {
			t.Fatalf("descriptor VFS accepted unreviewed Linux filesystem %#x", filesystem)
		}
	}
}
