//go:build aiik_descriptor_vfs && aiik_descriptor_vfs_test && cgo && (darwin || linux) && !libsqlite3

package sqlite3

import (
	"database/sql/driver"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func aiikTestDescriptorFile(t *testing.T, file *os.File) AIIKDescriptorFile {
	t.Helper()
	var st syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &st); err != nil {
		t.Fatal(err)
	}
	return AIIKDescriptorFile{
		FD:        int(file.Fd()),
		Device:    uint64(st.Dev),
		Inode:     uint64(st.Ino),
		LinkCount: uint64(st.Nlink),
		FileType:  uint32(st.Mode & syscall.S_IFMT),
	}
}

func aiikTestDatabaseFile(t *testing.T, dir, name string) *os.File {
	t.Helper()
	path := filepath.Join(dir, name)
	conn, err := (&SQLiteDriver{}).Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func aiikTestSource(t *testing.T, dir string) AIIKDescriptorSource {
	t.Helper()
	path := filepath.Join(dir, "source.db")
	conn, err := (&SQLiteDriver{}).Open(path)
	if err != nil {
		t.Fatal(err)
	}
	stock := conn.(*SQLiteConn)
	if _, err := stock.Exec("PRAGMA journal_mode=WAL", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := stock.Exec("CREATE TABLE source_fixture (id INTEGER)", nil); err != nil {
		t.Fatal(err)
	}
	anchor, err := InspectAIIKUnixConnection(stock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	db, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	wal, err := os.Open(filepath.Join(dir, "source.db-wal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wal.Close() })
	shm, err := os.OpenFile(filepath.Join(dir, "source.db-shm"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shm.Close() })
	source := AIIKDescriptorSource{
		Database: aiikTestDescriptorFile(t, db),
		WAL:      aiikTestDescriptorFile(t, wal),
		SHM:      aiikTestDescriptorFile(t, shm),
	}
	source.Anchor = anchor
	return source
}

func TestAIIKDescriptorVFSRejectsMissingSourceSidecars(t *testing.T) {
	db, err := os.CreateTemp(t.TempDir(), "source-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = OpenAIIKDescriptorSource(AIIKDescriptorSource{Database: AIIKDescriptorFile{FD: int(db.Fd())}})
	if err == nil {
		t.Fatal("OpenAIIKDescriptorSource accepted missing WAL and SHM descriptors")
	}
}

func TestAIIKDescriptorVFSRequiresEveryExactIdentityField(t *testing.T) {
	db := aiikTestDatabaseFile(t, t.TempDir(), "source.db")
	file := aiikTestDescriptorFile(t, db)
	file.Inode = 0

	_, err := OpenAIIKDescriptorDestination(AIIKDescriptorDestination{Database: file})
	if err == nil {
		t.Fatal("OpenAIIKDescriptorDestination accepted a zero exact identity field")
	}
}

func TestAIIKDescriptorVFSOpensExactSourceThroughPrivateVFS(t *testing.T) {
	dir := t.TempDir()
	source := aiikTestSource(t, dir)

	conn, err := OpenAIIKDescriptorSource(source)
	if err != nil {
		t.Fatalf("OpenAIIKDescriptorSource() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	info, err := InspectAIIKUnixConnection(conn)
	if err != nil {
		t.Fatal(err)
	}
	if !info.DescriptorVFS {
		t.Fatal("source did not reach the private descriptor VFS")
	}
	if err := aiikTestMainLifecycle(conn); err != nil {
		t.Fatalf("descriptor main lifecycle consulted its opaque token namespace: %v", err)
	}

	if _, err := OpenAIIKDescriptorSource(source); err == nil {
		t.Fatal("OpenAIIKDescriptorSource accepted a duplicate live endpoint use")
	}
}

func TestAIIKDescriptorVFSTokenCollisionExhaustionIsBounded(t *testing.T) {
	aiikTestTokenCollision(true, 1)
	t.Cleanup(func() { aiikTestTokenCollision(false, 0) })

	first, err := OpenAIIKDescriptorSource(aiikTestSource(t, t.TempDir()))
	if err != nil {
		t.Fatalf("first fixed-token source open: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	if _, err := OpenAIIKDescriptorSource(aiikTestSource(t, t.TempDir())); err == nil {
		t.Fatal("fixed-token collision did not exhaust its bounded registration attempts")
	}
}

func TestAIIKDescriptorVFSRequiresReadOnlySourceMainAndWAL(t *testing.T) {
	dir := t.TempDir()
	db := aiikTestDatabaseFile(t, dir, "source.db")
	wal, err := os.Create(filepath.Join(dir, "source.db-wal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wal.Close() })
	shm, err := os.Create(filepath.Join(dir, "source.db-shm"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shm.Close() })

	_, err = OpenAIIKDescriptorSource(AIIKDescriptorSource{
		Database: aiikTestDescriptorFile(t, db),
		WAL:      aiikTestDescriptorFile(t, wal),
		SHM:      aiikTestDescriptorFile(t, shm),
	})
	if err == nil {
		t.Fatal("source accepted a writable database and WAL descriptor")
	}
}

func TestAIIKDescriptorVFSRejectsMissingOrMismatchedAnchorAuthority(t *testing.T) {
	source := aiikTestSource(t, t.TempDir())
	source.Anchor = AIIKUnixConnectionInfo{}
	if _, err := OpenAIIKDescriptorSource(source); err == nil {
		t.Fatal("source accepted missing stock-anchor authority")
	}
	source = aiikTestSource(t, t.TempDir())
	source.Anchor.LockingMethod = "unknown"
	if _, err := OpenAIIKDescriptorSource(source); err == nil {
		t.Fatal("source accepted mismatched stock locking authority")
	}
	source = aiikTestSource(t, t.TempDir())
	source.Anchor.DescriptorVFS = true
	if _, err := OpenAIIKDescriptorSource(source); err == nil {
		t.Fatal("source accepted a non-stock anchor authority")
	}
}

func TestAIIKDescriptorVFSDestinationRequiresEmptyExclusiveFile(t *testing.T) {
	db := aiikTestDatabaseFile(t, t.TempDir(), "destination.db")
	if _, err := db.WriteAt([]byte{1}, 0); err != nil {
		t.Fatal(err)
	}
	_, err := OpenAIIKDescriptorDestination(AIIKDescriptorDestination{
		Database: aiikTestDescriptorFile(t, db),
	})
	if err == nil {
		t.Fatal("destination accepted a non-empty database file")
	}
}

func TestAIIKDescriptorVFSRejectsLinkedDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "destination.db")
	db, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := os.Link(path, filepath.Join(dir, "destination-link.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAIIKDescriptorDestination(AIIKDescriptorDestination{
		Database: aiikTestDescriptorFile(t, db),
	}); err == nil {
		t.Fatal("destination accepted a linked descriptor")
	}
}

func TestAIIKDescriptorVFSOpensExactDestinationThroughPrivateVFS(t *testing.T) {
	db := aiikTestDatabaseFile(t, t.TempDir(), "destination.db")
	conn, err := OpenAIIKDescriptorDestination(AIIKDescriptorDestination{
		Database: aiikTestDescriptorFile(t, db),
	})
	if err != nil {
		t.Fatalf("OpenAIIKDescriptorDestination() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	info, err := InspectAIIKUnixConnection(conn)
	if err != nil {
		t.Fatal(err)
	}
	if !info.DescriptorVFS {
		t.Fatal("destination did not reach the private descriptor VFS")
	}
	if _, err := OpenAIIKDescriptorDestination(AIIKDescriptorDestination{
		Database: aiikTestDescriptorFile(t, db),
	}); err == nil {
		t.Fatal("OpenAIIKDescriptorDestination accepted a duplicate live endpoint use")
	}
}

func TestAIIKDescriptorVFSDeferredCloseUsesStockUnusedFD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "destination.db")
	db := aiikTestDatabaseFile(t, dir, "destination.db")
	descriptorConn, err := OpenAIIKDescriptorDestination(AIIKDescriptorDestination{
		Database: aiikTestDescriptorFile(t, db),
	})
	if err != nil {
		t.Fatal(err)
	}
	ordinaryConn, err := (&SQLiteDriver{}).Open(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ordinaryConn.(*SQLiteConn).Query("SELECT name FROM sqlite_master", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := descriptorConn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ordinaryConn.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAIIKDescriptorVFSSourceRejectsWritesBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	source := aiikTestSource(t, dir)
	conn, err := OpenAIIKDescriptorSource(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.Exec("PRAGMA journal_mode=OFF; CREATE TABLE forbidden_write (id INTEGER)", nil); err == nil {
		t.Fatal("descriptor source accepted a direct database write")
	}
}

func TestAIIKDescriptorVFSConsumesRegisteredWALRoleWithoutPathLookup(t *testing.T) {
	source := aiikTestSource(t, t.TempDir())
	conn, err := OpenAIIKDescriptorSource(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := aiikTestConsumeWAL(conn); err != nil {
		t.Fatalf("registered WAL role was not consumed through the private VFS: %v", err)
	}
}

func TestAIIKDescriptorVFSAdoptsAndRejectsMismatchedSHM(t *testing.T) {
	dir := t.TempDir()
	source := aiikTestSource(t, dir)
	ordinary, err := (&SQLiteDriver{}).Open(filepath.Join(dir, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ordinary.Close() })
	if err := aiikTestHoldStockSHM(ordinary.(*SQLiteConn)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = aiikTestReleaseStockSHM(ordinary.(*SQLiteConn)) })
	conn, err := OpenAIIKDescriptorSource(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := aiikTestMismatchSHM(conn); err != nil {
		t.Fatalf("mismatched SHM identity was not rejected: %v", err)
	}
	if err := aiikTestAdoptSHM(conn); err != nil {
		t.Fatalf("registered SHM descriptor was not adopted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "source.db-shm")); err != nil {
		t.Fatalf("descriptor SHM unmap unlinked the borrowed endpoint: %v", err)
	}
}

func TestAIIKDescriptorVFSReusesMatchingStockSHMWithoutUnlink(t *testing.T) {
	dir := t.TempDir()
	source := aiikTestSource(t, dir)
	ordinary, err := (&SQLiteDriver{}).Open(filepath.Join(dir, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ordinary.Close() })
	if err := aiikTestHoldStockSHM(ordinary.(*SQLiteConn)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = aiikTestReleaseStockSHM(ordinary.(*SQLiteConn)) })

	descriptor, err := OpenAIIKDescriptorSource(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = descriptor.Close() })
	if err := aiikTestAdoptSHM(descriptor); err != nil {
		t.Fatalf("matching stock SHM node was not reused: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "source.db-shm")); err != nil {
		t.Fatalf("reused descriptor SHM node unlinked the borrowed endpoint: %v", err)
	}
}

func TestAIIKDescriptorVFSDoesNotChangeDefaultDriverOpen(t *testing.T) {
	conn, err := (&SQLiteDriver{}).Open(filepath.Join(t.TempDir(), "ordinary.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, ok := conn.(driver.Conn); !ok {
		t.Fatal("ordinary SQLiteDriver.Open did not return a driver connection")
	}
}

func TestAIIKDescriptorVFSRejectsForbiddenRoleBeforePathOpen(t *testing.T) {
	source := aiikTestSource(t, t.TempDir())
	conn, err := OpenAIIKDescriptorSource(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := aiikTestForbiddenOpen(conn); err != nil {
		t.Fatalf("forbidden registered endpoint role was not rejected cleanly: %v", err)
	}
}

func TestAIIKDescriptorVFSEarlyOpenFailureClosesAllDuplicates(t *testing.T) {
	dir := t.TempDir()
	source := aiikTestSource(t, dir)
	aiikTestInjectEarlyFailure(true)
	t.Cleanup(func() { aiikTestInjectEarlyFailure(false) })

	_, err := OpenAIIKDescriptorSource(source)
	if err == nil {
		t.Fatal("injected private VFS early failure unexpectedly opened source")
	}
	if outstanding := aiikTestOutstandingDuplicates(); outstanding != 0 {
		t.Fatalf("early failure leaked %d duplicated descriptors", outstanding)
	}
}

func TestAIIKDescriptorVFSRejectsInvalidDestinationIdentity(t *testing.T) {
	db, err := os.CreateTemp(t.TempDir(), "destination-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = OpenAIIKDescriptorDestination(AIIKDescriptorDestination{
		Database: AIIKDescriptorFile{FD: int(db.Fd()), Device: ^uint64(0)},
	})
	if err == nil {
		t.Fatal("OpenAIIKDescriptorDestination accepted an invalid device identity")
	}
}

func TestAIIKUnixConnectionInspectionRejectsNil(t *testing.T) {
	if _, err := InspectAIIKUnixConnection(nil); err == nil {
		t.Fatal("InspectAIIKUnixConnection accepted nil")
	}
}
