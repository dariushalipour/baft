package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/port"
)

// A stray contract above the checked root must never be adopted, and an
// absolute root must behave exactly like the equivalent relative one.
func TestFindContractStopsAtRoot(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	fsys := memfs.New()
	if err := fsys.WriteFile(filepath.Join(filepath.Dir(root), port.ContractFile), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}

	capsuleDir := filepath.Join(root, "svc")
	want := filepath.Join(capsuleDir, port.ContractFile)
	for _, start := range []string{root, "."} {
		if got := FindContract(fsys, start, capsuleDir); got != want {
			t.Errorf("FindContract(%q) = %q, want %q", start, got, want)
		}
		if dir, exists := FindOrCreateContractDir(fsys, start, capsuleDir); exists || dir != root {
			t.Errorf("FindOrCreateContractDir(%q) = (%q, %v), want (%q, false)", start, dir, exists, root)
		}
	}
}

// A root below the capsule still climbs up to the capsule's contract.
func TestFindContractClimbsWithinCapsule(t *testing.T) {
	fsys := memfs.New()
	capsuleDir := filepath.FromSlash("/repo")
	want := filepath.Join(capsuleDir, port.ContractFile)
	if err := fsys.WriteFile(want, []byte("contract"), 0o644); err != nil {
		t.Fatal(err)
	}

	startDir := filepath.Join(capsuleDir, "internal", "app")
	if got := FindContract(fsys, startDir, capsuleDir); got != want {
		t.Errorf("FindContract(%q) = %q, want %q", startDir, got, want)
	}
	if dir, exists := FindOrCreateContractDir(fsys, startDir, capsuleDir); !exists || dir != capsuleDir {
		t.Errorf("FindOrCreateContractDir(%q) = (%q, %v), want (%q, true)", startDir, dir, exists, capsuleDir)
	}
}
