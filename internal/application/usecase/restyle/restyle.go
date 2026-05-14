package restyle

import (
	"fmt"
	"io/fs"
	"sort"

	"github.com/dariushalipour/baft/internal/port"
)

type Result struct {
	Contracts []ContractRestyle
	Errors    []RestyleError
}

type ContractRestyle struct {
	ContractPath string
	Changed      bool
}

type RestyleError struct {
	ContractPath string
	Err          error
}

func (e RestyleError) Error() string {
	return fmt.Sprintf("%s: %s", e.ContractPath, e.Err)
}

func RestyleContract(content string, repo port.GraphRepository, saveOpts port.GraphSaveOptions) (string, bool, error) {
	g, err := repo.Load(content)
	if err != nil {
		return "", false, err
	}
	restyled := repo.Save(g, saveOpts)
	return restyled, restyled != content, nil
}

func Run(fsys port.FileSystem, rootDir string, repo port.GraphRepository, saveOpts port.GraphSaveOptions) (*Result, error) {
	paths, err := discoverContracts(fsys, rootDir)
	if err != nil {
		return nil, err
	}

	result := &Result{}
	for _, contractPath := range paths {
		raw, err := fsys.ReadFile(contractPath)
		if err != nil {
			result.Errors = append(result.Errors, RestyleError{ContractPath: contractPath, Err: err})
			continue
		}
		content, changed, err := RestyleContract(string(raw), repo, saveOpts)
		if err != nil {
			result.Errors = append(result.Errors, RestyleError{ContractPath: contractPath, Err: err})
			continue
		}
		if changed {
			if err := fsys.WriteFile(contractPath, []byte(content), 0o644); err != nil {
				result.Errors = append(result.Errors, RestyleError{ContractPath: contractPath, Err: err})
				continue
			}
		}
		result.Contracts = append(result.Contracts, ContractRestyle{
			ContractPath: contractPath,
			Changed:      changed,
		})
	}

	return result, nil
}

func discoverContracts(fsys port.FileSystem, rootDir string) ([]string, error) {
	var paths []string
	if err := fsys.WalkDir(rootDir, func(abs string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		if d.Name() == port.ContractFile {
			paths = append(paths, abs)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}
