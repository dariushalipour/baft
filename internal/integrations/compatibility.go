package integrations

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

const protocolVersion = 4

// Stable, machine-readable classifications of a compatibility report.
// Clients switch on these instead of matching the human-readable message.
const (
	CodeOK                     = "ok"
	CodeUnsupportedIntegration = "unsupported_integration"
	CodeVersionUnavailable     = "expected_version_unavailable"
	CodeProtocolMismatch       = "protocol_mismatch"
	CodeVersionMismatch        = "version_mismatch"
)

type CompatibilityReport struct {
	Compatible      bool   `json:"compatible"`
	Code            string `json:"code"`
	IntegrationID   string `json:"integration_id"`
	Family          string `json:"family"`
	Protocol        int    `json:"protocol"`
	PluginVersion   string `json:"plugin_version"`
	ExpectedVersion string `json:"expected_version,omitempty"`
	CLIVersion      string `json:"cli_version"`
	Message         string `json:"message"`
}

var (
	embeddedVersionsOnce   sync.Once
	embeddedPluginVersions map[string]string
	embeddedVersionsErr    error
)

func getEmbeddedVersions() {
	embeddedVersionsOnce.Do(func() {
		vscodeVer, vscodeErr := getEmbeddedVSCodeVersion()
		jetbrainsVer, jetbrainsErr := getEmbeddedJetBrainsVersion()

		if vscodeErr != nil || jetbrainsErr != nil {
			embeddedVersionsErr = fmt.Errorf("could not load embedded plugin versions: %v; %v", vscodeErr, jetbrainsErr)
			return
		}

		embeddedPluginVersions = map[string]string{
			FamilyVSCode:    vscodeVer,
			FamilyJetBrains: jetbrainsVer,
		}
	})
}

// VerifyCompatibility reports whether a plugin build may talk to this CLI.
// Plugins ship embedded in the CLI, so the policy is exact version equality.
func VerifyCompatibility(cliVersion, integrationID, pluginVersion string, protocol int) CompatibilityReport {
	report := CompatibilityReport{
		IntegrationID: integrationID,
		Family:        FamilyForID(integrationID),
		Protocol:      protocol,
		PluginVersion: pluginVersion,
		CLIVersion:    cliVersion,
	}
	if report.Family == "" {
		return report.fail(CodeUnsupportedIntegration, "unsupported integration: "+integrationID)
	}

	expectedVersion, err := expectedPluginVersion(report.Family)
	if err != nil {
		return report.fail(CodeVersionUnavailable, "Baft CLI could not determine the expected plugin version for "+report.Family+": "+err.Error())
	}
	if protocol != protocolVersion {
		return report.fail(CodeProtocolMismatch, fmt.Sprintf("Baft plugin protocol mismatch: plugin uses protocol %d, CLI expects protocol %d", protocol, protocolVersion))
	}
	if pluginVersion != expectedVersion {
		report.ExpectedVersion = expectedVersion
		return report.fail(CodeVersionMismatch, fmt.Sprintf("Baft plugin version mismatch: expected %s, got %s", expectedVersion, pluginVersion))
	}

	report.Compatible = true
	report.Code = CodeOK
	report.Message = "compatible"
	return report
}

func (r CompatibilityReport) fail(code, message string) CompatibilityReport {
	r.Code = code
	r.Message = message
	return r
}

// FamilyForID maps an IDE identifier to its integration family.
func FamilyForID(id string) string {
	switch id {
	case "vscode", "vscode-insiders":
		return FamilyVSCode
	case "jetbrains", "goland", "intellij-ultimate", "intellij-community", "webstorm", "rider", "android-studio", "rustrover":
		return FamilyJetBrains
	default:
		return ""
	}
}

func expectedPluginVersion(family string) (string, error) {
	getEmbeddedVersions()
	if embeddedVersionsErr != nil {
		return "", embeddedVersionsErr
	}
	return embeddedPluginVersions[family], nil
}

func getEmbeddedVSCodeVersion() (string, error) {
	asset, err := embeddedAssets.ReadFile(vscodeAssetPath)
	if err != nil {
		return "", fmt.Errorf("read embedded VS Code extension: %w", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(asset), int64(len(asset)))
	if err != nil {
		return "", fmt.Errorf("open embedded VS Code extension: %w", err)
	}
	for _, file := range reader.File {
		if file.Name != "extension/package.json" {
			continue
		}
		src, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("open embedded VS Code package.json: %w", err)
		}
		content, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			return "", fmt.Errorf("read embedded VS Code package.json: %w", err)
		}
		var manifest struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(content, &manifest); err != nil {
			return "", fmt.Errorf("parse embedded VS Code package.json: %w", err)
		}
		if strings.TrimSpace(manifest.Version) == "" {
			return "", fmt.Errorf("embedded VS Code package.json is missing a version")
		}
		return strings.TrimSpace(manifest.Version), nil
	}
	return "", fmt.Errorf("embedded VS Code package.json not found")
}

func getEmbeddedJetBrainsVersion() (string, error) {
	asset, err := embeddedAssets.ReadFile(jetbrainsAssetPath)
	if err != nil {
		return "", fmt.Errorf("read embedded JetBrains plugin: %w", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(asset), int64(len(asset)))
	if err != nil {
		return "", fmt.Errorf("open embedded JetBrains plugin: %w", err)
	}
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, jetbrainsArchiveRoot+"/lib/") || !strings.HasSuffix(file.Name, ".jar") {
			continue
		}
		src, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("open embedded JetBrains plugin jar: %w", err)
		}
		content, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			return "", fmt.Errorf("read embedded JetBrains plugin jar: %w", err)
		}
		jarReader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
		if err != nil {
			return "", fmt.Errorf("open embedded JetBrains plugin jar: %w", err)
		}
		descriptor, err := readJetBrainsPluginDescriptorFromZip(jarReader)
		if err == nil && strings.TrimSpace(descriptor.Version) != "" {
			return strings.TrimSpace(descriptor.Version), nil
		}
	}
	return "", fmt.Errorf("embedded JetBrains plugin version not found")
}
