package integrations

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type vscodeInstaller struct {
	cliVersion string
}

type vscodeCandidate struct {
	ID          string
	DisplayName string
	Commands    []string
}

func newVSCodeInstaller(cliVersion string) Installer {
	return &vscodeInstaller{cliVersion: cliVersion}
}

func (i *vscodeInstaller) Family() string {
	return FamilyVSCode
}

func (i *vscodeInstaller) Detect(ctx context.Context) ([]IDEInstallation, error) {
	home, _ := os.UserHomeDir()
	var installations []IDEInstallation
	for _, candidate := range []vscodeCandidate{
		{ID: "vscode", DisplayName: "VS Code", Commands: []string{"code", "code.cmd"}},
		{ID: "vscode-insiders", DisplayName: "VS Code Insiders", Commands: []string{"code-insiders", "code-insiders.cmd"}},
	} {
		executable := lookPath(candidate.Commands)
		if executable == "" {
			executable = firstExistingFile(vscodeKnownPaths(candidate.ID, home))
		}
		if executable == "" {
			continue
		}
		installations = append(installations, IDEInstallation{
			ID:          candidate.ID,
			Family:      FamilyVSCode,
			DisplayName: candidate.DisplayName,
			Version:     commandVersion(ctx, executable),
			InstallPath: filepath.Dir(executable),
			Executable:  executable,
		})
	}
	return installations, nil
}

func (i *vscodeInstaller) Install(ctx context.Context, ide IDEInstallation) error {
	asset, err := embeddedAssets.ReadFile(vscodeAssetPath)
	if err != nil {
		return fmt.Errorf("could not read embedded VS Code integration package: %w", err)
	}

	tempFile, err := os.CreateTemp("", "baft-*.vsix")
	if err != nil {
		return fmt.Errorf("could not create temporary VSIX file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	if _, err := tempFile.Write(asset); err != nil {
		tempFile.Close()
		return fmt.Errorf("could not write temporary VSIX file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("could not finalize temporary VSIX file: %w", err)
	}

	cmd := exec.CommandContext(ctx, ide.Executable, "--install-extension", tempFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not install VS Code integration with %s: %w\n%s", ide.Executable, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (i *vscodeInstaller) Verify(ctx context.Context, ide IDEInstallation) error {
	report := VerifyCompatibility(i.cliVersion, ide.ID, expectedPluginVersion(FamilyVSCode), expectedProtocol(FamilyVSCode))
	if !report.Compatible {
		return fmt.Errorf(report.Message)
	}

	cmd := exec.CommandContext(ctx, ide.Executable, "--list-extensions", "--show-versions")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not verify VS Code installation with %s: %w\n%s", ide.Executable, err, strings.TrimSpace(string(output)))
	}
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == vscodeExtensionID || strings.HasPrefix(trimmed, vscodeExtensionID+"@") {
			return nil
		}
	}
	return fmt.Errorf("VS Code integration was not found after installation. Checked extension id %s", vscodeExtensionID)
}

func vscodeKnownPaths(id, home string) []string {
	switch runtime.GOOS {
	case "darwin":
		if id == "vscode-insiders" {
			return []string{
				"/Applications/Visual Studio Code - Insiders.app/Contents/Resources/app/bin/code-insiders",
				filepath.Join(home, "Applications", "Visual Studio Code - Insiders.app", "Contents", "Resources", "app", "bin", "code-insiders"),
			}
		} else {
			return []string{
				"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code",
				filepath.Join(home, "Applications", "Visual Studio Code.app", "Contents", "Resources", "app", "bin", "code"),
			}
		}
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if id == "vscode-insiders" {
			return []string{
				filepath.Join(localAppData, "Programs", "Microsoft VS Code Insiders", "bin", "code-insiders.cmd"),
			}
		} else {
			return []string{
				filepath.Join(localAppData, "Programs", "Microsoft VS Code", "bin", "code.cmd"),
			}
		}
	default:
		if id == "vscode-insiders" {
			return []string{"/usr/bin/code-insiders", "/snap/bin/code-insiders", "/var/lib/flatpak/exports/bin/com.visualstudio.code-insiders"}
		} else {
			return []string{"/usr/bin/code", "/snap/bin/code", "/var/lib/flatpak/exports/bin/com.visualstudio.code"}
		}
	}
}

func commandVersion(ctx context.Context, executable string) string {
	cmd := exec.CommandContext(ctx, executable, "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}

func lookPath(names []string) string {
	for _, name := range names {
		resolved, err := exec.LookPath(name)
		if err == nil {
			return resolved
		}
	}
	return ""
}

func firstExistingFile(paths []string) string {
	for _, candidate := range paths {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
