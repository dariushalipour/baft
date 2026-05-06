package service

import (
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/dariushalipour/strata/internal/port"
)

// ManifestInfo describes how to find and parse a build manifest for a language.
type ManifestInfo struct {
	// Names are the file names to look for (e.g. ["go.mod"], ["build.gradle.kts", "build.gradle"]).
	Names []string
	// ParseFunc reads a manifest file and extracts the capsule identifier.
	// It returns an empty string when the manifest is valid but has no capsule ID
	// (e.g. a package.json without a "name" field). In that case the caller
	// may choose to skip the capsule entirely.
	// If ParseFunc returns a non-empty capsule ID alongside an error, the error
	// is ignored and the capsule is included (this accommodates parsers like
	// Kotlin's findBaseCapsule that return errors for edge cases but still
	// produce a valid capsule ID).
	ParseFunc func(fsys port.FileSystem, path string) (string, error)
}

// CapsuleEntry is a capsule discovered by the discovery service, paired with
// the language name it belongs to.
type CapsuleEntry struct {
	Capsule port.Capsule
	// LangName is the language name used for registration (e.g. "go", "dart").
	LangName string
}

// CapsuleDiscovery handles all capsule discovery logic that was previously
// duplicated across language adapters. Each language registers with it by
// providing manifest file names and a parser function.
type CapsuleDiscovery struct {
	manifests map[string]ManifestInfo // lang name -> manifest info
}

// NewCapsuleDiscovery returns a new CapsuleDiscovery instance.
func NewCapsuleDiscovery() *CapsuleDiscovery {
	return &CapsuleDiscovery{
		manifests: make(map[string]ManifestInfo),
	}
}

// Register adds a language's manifest info to the discovery service.
func (d *CapsuleDiscovery) Register(name string, info ManifestInfo) {
	d.manifests[name] = info
}

// checkManifest attempts to find and parse a manifest file in the given directory.
// It returns a CapsuleEntry if found, or an empty entry if not.
func (d *CapsuleDiscovery) checkManifest(fsys port.FileSystem, dir string) (CapsuleEntry, bool) {
	// Iterate over registered languages in sorted order for deterministic behavior.
	langs := make([]string, 0, len(d.manifests))
	for name := range d.manifests {
		langs = append(langs, name)
	}
	sort.Strings(langs)

	for _, langName := range langs {
		info := d.manifests[langName]
		for _, manifestName := range info.Names {
			manifestPath := filepath.Join(dir, manifestName)
			if _, err := fsys.Stat(manifestPath); err != nil {
				continue
			}
			capsuleID, parseErr := info.ParseFunc(fsys, manifestPath)
			if capsuleID != "" {
				parseErr = nil
			}
			if parseErr != nil || capsuleID == "" {
				continue
			}
			return CapsuleEntry{
				Capsule:  port.Capsule{Dir: dir, CapsuleID: capsuleID},
				LangName: langName,
			}, true
		}
	}
	return CapsuleEntry{}, false
}

func (d *CapsuleDiscovery) Discover(fsys port.FileSystem, rootDir string) ([]CapsuleEntry, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	var out []CapsuleEntry

	// Phase 1 — check rootDir and walk upward to find a manifest.
	// Start at absRoot (covers rootDir) and climb to the filesystem root.
	dir := absRoot
	for {
		if entry, ok := d.checkManifest(fsys, dir); ok {
			out = append(out, entry)
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Phase 2 — walk downward to discover all capsules.
	// Skip absRoot since it was already checked in Phase 1.
	err = fsys.WalkDir(absRoot, func(abs string, entry fs.DirEntry) error {
		if !entry.IsDir() {
			return nil
		}
		if abs == absRoot {
			return nil
		}
		if entry, ok := d.checkManifest(fsys, abs); ok {
			out = append(out, entry)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Capsule.Dir < out[j].Capsule.Dir })
	return out, nil
}
