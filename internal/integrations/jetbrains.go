package integrations

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type jetbrainsInstaller struct {
	cliVersion string
}

type jetbrainsProductInfo struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	DataDirectoryName string `json:"dataDirectoryName"`
	Launch            []struct {
		LauncherPath string `json:"launcherPath"`
	} `json:"launch"`
}

type jetbrainsPluginDescriptor struct {
	XMLName xml.Name `xml:"idea-plugin"`
	ID      string   `xml:"id"`
	Name    string   `xml:"name"`
	Version string   `xml:"version"`
}

func newJetBrainsInstaller(cliVersion string) Installer {
	return &jetbrainsInstaller{cliVersion: cliVersion}
}

func (i *jetbrainsInstaller) Family() string {
	return FamilyJetBrains
}

func (i *jetbrainsInstaller) Detect(ctx context.Context) ([]IDEInstallation, error) {
	_ = ctx
	home, _ := os.UserHomeDir()
	paths, err := findJetBrainsProductInfoFiles(home)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var installations []IDEInstallation
	for _, infoPath := range paths {
		ide, err := loadJetBrainsInstallation(infoPath, home)
		if err != nil || ide.ID == "" {
			continue
		}
		key := ide.ID + ":" + ide.InstallPath
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		installations = append(installations, ide)
	}
	sort.SliceStable(installations, func(a, b int) bool {
		return installations[a].DisplayName < installations[b].DisplayName
	})
	return installations, nil
}

func (i *jetbrainsInstaller) Install(ctx context.Context, ide IDEInstallation) error {
	_ = ctx
	if ide.PluginDir == "" {
		return fmt.Errorf("could not determine JetBrains plugin directory for %s", ide.DisplayName)
	}
	asset, err := embeddedAssets.ReadFile(jetbrainsAssetPath)
	if err != nil {
		return fmt.Errorf("could not read embedded JetBrains plugin package: %w", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(asset), int64(len(asset)))
	if err != nil {
		return fmt.Errorf("could not read embedded JetBrains plugin archive: %w", err)
	}
	return installJetBrainsArchive(ide, reader)
}

func (i *jetbrainsInstaller) Verify(ctx context.Context, ide IDEInstallation) error {
	_ = ctx
	report := VerifyCompatibility(i.cliVersion, ide.ID, expectedPluginVersion(FamilyJetBrains), expectedProtocol(FamilyJetBrains))
	if !report.Compatible {
		return fmt.Errorf(report.Message)
	}
	targetDir := filepath.Join(ide.PluginDir, jetbrainsArchiveRoot)
	descriptor, _, err := readJetBrainsPluginDescriptor(targetDir)
	if err != nil {
		return fmt.Errorf("JetBrains plugin was not found after installation: %w", err)
	}
	if descriptor.ID != jetbrainsPluginID {
		return fmt.Errorf("JetBrains plugin at %s is not the BAFT plugin", targetDir)
	}
	if descriptor.Version != expectedPluginVersion(FamilyJetBrains) {
		return fmt.Errorf("JetBrains plugin version mismatch after installation: expected %s, found %s", expectedPluginVersion(FamilyJetBrains), descriptor.Version)
	}
	return nil
}

func installJetBrainsArchive(ide IDEInstallation, reader *zip.Reader) error {
	rootDir, err := zipRootDirectory(reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(ide.PluginDir, 0o755); err != nil {
		return jetbrainsManualInstallError(ide, filepath.Join(ide.PluginDir, rootDir), "could not create the JetBrains plugins directory", err)
	}

	stageDir, err := os.MkdirTemp(ide.PluginDir, ".baft-install-*")
	if err != nil {
		return jetbrainsManualInstallError(ide, filepath.Join(ide.PluginDir, rootDir), "could not create a temporary staging directory", err)
	}
	defer os.RemoveAll(stageDir)

	if err := extractArchive(reader, stageDir); err != nil {
		return jetbrainsManualInstallError(ide, filepath.Join(ide.PluginDir, rootDir), "could not extract the embedded JetBrains plugin archive", err)
	}

	stagedPluginDir := filepath.Join(stageDir, rootDir)
	stagedDescriptor, _, err := readJetBrainsPluginDescriptor(stagedPluginDir)
	if err != nil {
		return fmt.Errorf("embedded JetBrains plugin archive is invalid: %w", err)
	}
	if stagedDescriptor.ID != jetbrainsPluginID {
		return fmt.Errorf("embedded JetBrains plugin archive is not the BAFT plugin")
	}

	targetDir := filepath.Join(ide.PluginDir, rootDir)
	backupDir := ""
	if info, statErr := os.Stat(targetDir); statErr == nil {
		if !info.IsDir() {
			return jetbrainsManualInstallError(ide, targetDir, "the existing JetBrains plugin path is not a directory", nil)
		}
		existingDescriptor, _, descriptorErr := readJetBrainsPluginDescriptor(targetDir)
		if descriptorErr != nil {
			return jetbrainsManualInstallError(ide, targetDir, "found an existing plugin directory but could not confirm that it belongs to BAFT", descriptorErr)
		}
		if existingDescriptor.ID != jetbrainsPluginID {
			return jetbrainsManualInstallError(ide, targetDir, "found an existing plugin directory that is not the BAFT plugin", nil)
		}

		backupDir = filepath.Join(ide.PluginDir, rootDir+".backup-"+time.Now().UTC().Format("20060102150405"))
		if err := os.Rename(targetDir, backupDir); err != nil {
			return jetbrainsManualInstallError(ide, targetDir, "could not move the existing BAFT plugin out of the way; close the IDE and retry", err)
		}
	}

	if err := os.Rename(stagedPluginDir, targetDir); err != nil {
		if rollbackErr := rollbackJetBrainsPlugin(targetDir, backupDir); rollbackErr != nil {
			return jetbrainsManualInstallError(ide, targetDir, "could not move the new BAFT plugin into place and rollback also failed", errorsJoin(err, rollbackErr))
		}
		return jetbrainsManualInstallError(ide, targetDir, "could not move the new BAFT plugin into place", err)
	}

	installedDescriptor, _, err := readJetBrainsPluginDescriptor(targetDir)
	if err != nil || installedDescriptor.ID != jetbrainsPluginID {
		rollbackErr := rollbackJetBrainsPlugin(targetDir, backupDir)
		if rollbackErr != nil {
			return jetbrainsManualInstallError(ide, targetDir, "the new BAFT plugin was copied but could not be validated, and rollback failed", errorsJoin(err, rollbackErr))
		}
		return jetbrainsManualInstallError(ide, targetDir, "the new BAFT plugin was copied but could not be validated", err)
	}

	if backupDir != "" {
		_ = os.RemoveAll(backupDir)
	}
	return nil
}

func findJetBrainsProductInfoFiles(home string) ([]string, error) {
	var roots []string
	switch runtime.GOOS {
	case "darwin":
		roots = []string{
			"/Applications",
			filepath.Join(home, "Applications"),
		}
	case "windows":
		roots = []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "JetBrains", "Toolbox", "apps"),
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
		}
	default:
		roots = []string{
			"/opt",
			"/usr/local",
			filepath.Join(home, ".local", "share", "JetBrains", "Toolbox", "apps"),
		}
	}
	var found []string
	seen := make(map[string]struct{})
	for _, root := range roots {
		if root == "" {
			continue
		}
		entries, err := scanForProductInfo(root, 5)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if _, ok := seen[entry]; ok {
				continue
			}
			seen[entry] = struct{}{}
			found = append(found, entry)
		}
	}
	sort.Strings(found)
	return found, nil
}

func scanForProductInfo(root string, maxDepth int) ([]string, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}
	type item struct {
		path  string
		depth int
	}
	stack := []item{{path: root, depth: 0}}
	var found []string
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		entries, err := os.ReadDir(current.path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			child := filepath.Join(current.path, entry.Name())
			if entry.IsDir() {
				if runtime.GOOS == "darwin" && strings.HasSuffix(entry.Name(), ".app") {
					infoPath := filepath.Join(child, "Contents", "Resources", "product-info.json")
					if _, err := os.Stat(infoPath); err == nil {
						found = append(found, infoPath)
					}
					continue
				}
				if current.depth < maxDepth {
					stack = append(stack, item{path: child, depth: current.depth + 1})
				}
				continue
			}
			if entry.Name() == "product-info.json" {
				found = append(found, child)
			}
		}
	}
	return found, nil
}

func loadJetBrainsInstallation(infoPath, home string) (IDEInstallation, error) {
	data, err := os.ReadFile(infoPath)
	if err != nil {
		return IDEInstallation{}, err
	}
	var info jetbrainsProductInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return IDEInstallation{}, err
	}
	id := jetbrainsIDFromName(info.Name)
	if id == "" || info.DataDirectoryName == "" {
		return IDEInstallation{}, nil
	}
	installPath := jetbrainsInstallPath(infoPath)
	return IDEInstallation{
		ID:          id,
		Family:      FamilyJetBrains,
		DisplayName: info.Name,
		Version:     strings.TrimSpace(info.Version),
		InstallPath: installPath,
		Executable:  jetbrainsExecutable(installPath, info, infoPath),
		PluginDir:   jetbrainsPluginDir(home, info.DataDirectoryName),
	}, nil
}

func jetbrainsIDFromName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "goland"):
		return "goland"
	case strings.Contains(lower, "intellij") && strings.Contains(lower, "community"):
		return "intellij-community"
	case strings.Contains(lower, "intellij"):
		return "intellij-ultimate"
	case strings.Contains(lower, "webstorm"):
		return "webstorm"
	case strings.Contains(lower, "rider"):
		return "rider"
	default:
		return ""
	}
}

func jetbrainsInstallPath(infoPath string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Dir(filepath.Dir(filepath.Dir(infoPath)))
	}
	return filepath.Dir(infoPath)
}

func jetbrainsExecutable(installPath string, info jetbrainsProductInfo, infoPath string) string {
	for _, launch := range info.Launch {
		if launch.LauncherPath == "" {
			continue
		}
		for _, candidate := range []string{
			filepath.Join(installPath, launch.LauncherPath),
			filepath.Join(filepath.Dir(infoPath), launch.LauncherPath),
		} {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return installPath
}

func jetbrainsPluginDir(home, dataDirName string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "JetBrains", dataDirName, "plugins")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "JetBrains", dataDirName, "plugins")
	default:
		candidates := []string{
			filepath.Join(home, ".local", "share", "JetBrains", dataDirName, "plugins"),
			filepath.Join(home, ".config", "JetBrains", dataDirName, "plugins"),
			filepath.Join(home, "."+dataDirName, "config", "plugins"),
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		return candidates[0]
	}
}

func zipRootDirectory(reader *zip.Reader) (string, error) {
	root := ""
	for _, file := range reader.File {
		trimmed := strings.TrimPrefix(file.Name, "/")
		if trimmed == "" {
			continue
		}
		parts := strings.Split(trimmed, "/")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if root == "" {
			root = parts[0]
			continue
		}
		if parts[0] != root {
			return "", fmt.Errorf("JetBrains plugin archive contains multiple root directories")
		}
	}
	if root == "" {
		return "", fmt.Errorf("JetBrains plugin archive is empty")
	}
	return root, nil
}

func extractArchive(reader *zip.Reader, dest string) error {
	base := filepath.Clean(dest)
	for _, file := range reader.File {
		target := filepath.Join(dest, file.Name)
		cleanTarget := filepath.Clean(target)
		if cleanTarget != base && !strings.HasPrefix(cleanTarget, base+string(os.PathSeparator)) {
			return fmt.Errorf("archive entry escapes plugin directory: %s", file.Name)
		}
		isDir := file.FileInfo().IsDir() || strings.HasSuffix(file.Name, "/")
		mode := file.Mode()
		if isDir {
			if mode.Perm() == 0 || mode.Perm()&0o111 == 0 {
				mode = 0o755
			}
		} else if mode.Perm() == 0 {
			mode = 0o644
		}
		if isDir {
			if err := os.MkdirAll(cleanTarget, mode); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func readJetBrainsPluginDescriptor(pluginDir string) (jetbrainsPluginDescriptor, string, error) {
	libDir := filepath.Join(pluginDir, "lib")
	entries, err := os.ReadDir(libDir)
	if err != nil {
		return jetbrainsPluginDescriptor{}, "", fmt.Errorf("could not read JetBrains plugin lib directory %s: %w", libDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jar" {
			continue
		}
		jarPath := filepath.Join(libDir, entry.Name())
		descriptor, err := readJetBrainsPluginDescriptorFromJar(jarPath)
		if err == nil {
			return descriptor, jarPath, nil
		}
	}
	return jetbrainsPluginDescriptor{}, "", fmt.Errorf("could not find plugin metadata in %s", libDir)
}

func readJetBrainsPluginDescriptorFromJar(jarPath string) (jetbrainsPluginDescriptor, error) {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return jetbrainsPluginDescriptor{}, err
	}
	defer reader.Close()
	return readJetBrainsPluginDescriptorFromZip(&reader.Reader)
}

func readJetBrainsPluginDescriptorFromZip(reader *zip.Reader) (jetbrainsPluginDescriptor, error) {
	for _, file := range reader.File {
		if file.Name != "META-INF/plugin.xml" {
			continue
		}
		src, err := file.Open()
		if err != nil {
			return jetbrainsPluginDescriptor{}, err
		}
		defer src.Close()
		var descriptor jetbrainsPluginDescriptor
		if err := xml.NewDecoder(src).Decode(&descriptor); err != nil {
			return jetbrainsPluginDescriptor{}, err
		}
		return descriptor, nil
	}
	return jetbrainsPluginDescriptor{}, fmt.Errorf("META-INF/plugin.xml not found")
}

func rollbackJetBrainsPlugin(targetDir, backupDir string) error {
	if backupDir == "" {
		_ = os.RemoveAll(targetDir)
		return nil
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return err
	}
	return os.Rename(backupDir, targetDir)
}

func jetbrainsManualInstallError(ide IDEInstallation, targetDir, reason string, cause error) error {
	message := fmt.Sprintf("could not update the BAFT JetBrains plugin for %s: %s", ide.DisplayName, reason)
	if cause != nil {
		message += ": " + cause.Error()
	}
	message += "\n\nManual install:\n"
	message += fmt.Sprintf("1. Close %s.\n", ide.DisplayName)
	message += fmt.Sprintf("2. Remove the existing BAFT plugin directory if it exists: %s\n", targetDir)
	message += "3. Install the BAFT JetBrains plugin ZIP manually from the public repository or a local checkout: internal/integrations/embedded/jetbrains/baft-intellij.zip\n"
	message += "4. In the IDE, open Settings > Plugins > gear icon > Install Plugin from Disk..., choose baft-intellij.zip, and restart the IDE."
	return errors.New(message)
}

func errorsJoin(first, second error) error {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return fmt.Errorf("%v; %v", first, second)
}
