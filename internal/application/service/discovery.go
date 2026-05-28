package service

import (
	"context"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/port"
)

type CapsuleEntry struct {
	Capsule  port.Capsule
	LangName string
}

type CapsuleDiscovery struct {
	manifests         map[string]port.ManifestInfo // lang name -> manifest info
	baseIgnoreEntries map[string]bool              // aggregated base ignore entries from all registered languages
}

func NewCapsuleDiscovery() *CapsuleDiscovery {
	return &CapsuleDiscovery{
		manifests:         make(map[string]port.ManifestInfo),
		baseIgnoreEntries: make(map[string]bool),
	}
}

func (d *CapsuleDiscovery) Register(name string, info port.ManifestInfo) {
	d.manifests[name] = info
	for _, dir := range info.BaseIgnoreEntries {
		d.baseIgnoreEntries[dir] = true
	}
}

func (d *CapsuleDiscovery) BaseIgnoreEntries() map[string]bool {
	return d.baseIgnoreEntries
}

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
			var candidates []string
			if isGlob(manifestName) {
				entries, err := fsys.ReadDir(dir)
				if err != nil {
					continue
				}
				for _, entry := range entries {
					if entry.IsDir() {
						continue
					}
					if matched, err := filepath.Match(manifestName, entry.Name()); err == nil && matched {
						candidates = append(candidates, filepath.Join(dir, entry.Name()))
					}
				}
				sort.Strings(candidates)
			} else {
				manifestPath := filepath.Join(dir, manifestName)
				if _, err := fsys.Stat(manifestPath); err == nil {
					candidates = append(candidates, manifestPath)
				}
			}
			for _, candidate := range candidates {
				capsuleID, parseErr := info.ParseFunc(fsys, candidate)
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
	}
	return CapsuleEntry{}, false
}

func isGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[]")
}

func (d *CapsuleDiscovery) Discover(ctx context.Context, fsys port.FileSystem, rootDir string) ([]CapsuleEntry, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	var out []CapsuleEntry

	// Phase 1 — check rootDir and walk upward to find a manifest.
	dir := absRoot
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
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
	err = fsys.WalkDir(ctx, absRoot, func(abs string, entry fs.DirEntry) error {
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
