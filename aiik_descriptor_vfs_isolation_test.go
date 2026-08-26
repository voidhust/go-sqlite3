//go:build cgo && (darwin || linux) && !aiik_descriptor_vfs

package sqlite3

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAIIKDescriptorVFSIsAbsentWithoutBuildTag(t *testing.T) {
	conn, err := (&SQLiteDriver{}).Open(
		"file:" + filepath.Join(t.TempDir(), "ordinary.db") + "?vfs=aiik-descriptor-unix",
	)
	if err == nil {
		_ = conn.Close()
		t.Fatal("untagged build opened the private AIIK descriptor VFS")
	}
	if !strings.Contains(err.Error(), "no such vfs") {
		t.Fatalf("untagged build resolved the private VFS: %v", err)
	}
}
