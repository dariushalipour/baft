package service

import (
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/dariushalipour/baft/internal/port"
)

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
	manifests         map[string]port.ManifestInfo // lang name -> manifest info
	baseIgnoreEntries map[string]bool              // aggregated base ignore entries from all registered languages
}

// NewCapsuleDiscovery returns a new CapsuleDiscovery instance.
func NewCapsuleDiscovery() *CapsuleDiscovery {
	return &CapsuleDiscovery{
		manifests:         make(map[string]port.ManifestInfo),
		baseIgnoreEntries: make(map[string]bool),
	}
}

// Register adds a language's manifest info to the discovery service.
func (d *CapsuleDiscovery) Register(name string, info port.ManifestInfo) {
	d.manifests[name] = info
	for _, dir := range info.BaseIgnoreEntries {
		d.baseIgnoreEntries[dir] = true
	}
}

// BaseIgnoreEntries returns the aggregated set of base ignore entries.
func (d *CapsuleDiscovery) BaseIgnoreEntries() map[string]bool {
	return d.baseIgnoreEntries
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
