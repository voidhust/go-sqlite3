//go:build aiik_descriptor_vfs && cgo && (darwin || linux)

package sqlite3

import (
	"os"
	"testing"
)

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
