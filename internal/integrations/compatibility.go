package integrations

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const protocolVersion = 3

type CompatibilityReport struct {
	Compatible    bool   `json:"compatible"`
	IntegrationID string `json:"integration_id"`
	Family        string `json:"family"`
	Protocol      int    `json:"protocol"`
	PluginVersion string `json:"plugin_version"`
	CLIVersion    string `json:"cli_version"`
	CLIMin        string `json:"cli_min"`
	Message       string `json:"message"`
	Warning       string `json:"warning,omitempty"`
}

type compatibilitySpec struct {
	Family   string
	CLIMin   string
	Protocol int
}

var compatibilitySpecs = map[string]compatibilitySpec{
	FamilyVSCode: {
		Family:   FamilyVSCode,
		CLIMin:   "0.1.0",
		Protocol: protocolVersion,
	},
	FamilyJetBrains: {
		Family:   FamilyJetBrains,
		CLIMin:   "0.1.0",
		Protocol: protocolVersion,
	},
}

var embeddedPluginVersions = map[string]string{
	FamilyVSCode:    mustEmbeddedVSCodeVersion(),
	FamilyJetBrains: mustEmbeddedJetBrainsVersion(),
}

func VerifyCompatibility(cliVersion, integrationID, pluginVersion string, protocol int) CompatibilityReport {
	family := familyForIntegrationID(integrationID)
	spec, ok := compatibilitySpecs[family]
	if !ok {
		return CompatibilityReport{
			Compatible:    false,
			IntegrationID: integrationID,
			CLIVersion:    cliVersion,
			PluginVersion: pluginVersion,
			Protocol:      protocol,
			Message:       "unsupported integration: " + integrationID,
		}
	}

	report := CompatibilityReport{
		Compatible:    true,
		IntegrationID: integrationID,
		Family:        family,
		Protocol:      protocol,
		PluginVersion: pluginVersion,
		CLIVersion:    cliVersion,
		CLIMin:        spec.CLIMin,
		Message:       "compatible",
	}
	expectedVersion := expectedPluginVersion(family)
	if expectedVersion == "" {
		report.Compatible = false
		report.Message = "BAFT CLI could not determine the expected plugin version for " + family
		return report
	}

	if protocol != spec.Protocol {
		report.Compatible = false
		report.Message = fmt.Sprintf("BAFT plugin protocol mismatch: plugin uses protocol %d, CLI expects protocol %d", protocol, spec.Protocol)
		return report
	}
	if pluginVersion != expectedVersion {
		report.Compatible = false
		report.Message = fmt.Sprintf("BAFT plugin version mismatch: expected %s, got %s", expectedVersion, pluginVersion)
		return report
	}
	if isDevVersion(cliVersion) {
		report.Warning = "CLI version is a development build; semantic version checks were skipped"
		return report
	}

	cmp, err := compareSemver(cliVersion, spec.CLIMin)
	if err != nil {
		report.Warning = "CLI version is not a semantic version; semantic version checks were skipped"
		return report
	}
	if cmp < 0 {
		report.Compatible = false
		report.Message = fmt.Sprintf("BAFT plugin requires CLI >= %s. Current version: %s", spec.CLIMin, cliVersion)
	}
	return report
}

func familyForIntegrationID(id string) string {
	switch id {
	case "vscode", "vscode-insiders":
		return FamilyVSCode
	case "jetbrains", "goland", "intellij-ultimate", "intellij-community", "webstorm", "rider":
		return FamilyJetBrains
	default:
		return ""
	}
}

func expectedPluginVersion(family string) string {
	return embeddedPluginVersions[family]
}

func expectedProtocol(family string) int {
	return compatibilitySpecs[family].Protocol
}

func isDevVersion(version string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(version))
	return trimmed == "" || trimmed == "dev" || trimmed == "(devel)"
}

type semver struct {
	major int
	minor int
	patch int
}

func compareSemver(left, right string) (int, error) {
	lhs, err := parseSemver(left)
	if err != nil {
		return 0, err
	}
	rhs, err := parseSemver(right)
	if err != nil {
		return 0, err
	}
	if lhs.major != rhs.major {
		if lhs.major < rhs.major {
			return -1, nil
		}
		return 1, nil
	}
	if lhs.minor != rhs.minor {
		if lhs.minor < rhs.minor {
			return -1, nil
		}
		return 1, nil
	}
	if lhs.patch != rhs.patch {
		if lhs.patch < rhs.patch {
			return -1, nil
		}
		return 1, nil
	}
	return 0, nil
}

func parseSemver(value string) (semver, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(value, "v"))
	trimmed = strings.SplitN(trimmed, "-", 2)[0]
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("invalid semantic version: %s", value)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, fmt.Errorf("invalid semantic version: %s", value)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, fmt.Errorf("invalid semantic version: %s", value)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semver{}, fmt.Errorf("invalid semantic version: %s", value)
	}
	return semver{major: major, minor: minor, patch: patch}, nil
}

func mustEmbeddedVSCodeVersion() string {
	asset, err := embeddedAssets.ReadFile(vscodeAssetPath)
	if err != nil {
		panic(fmt.Errorf("read embedded VS Code extension: %w", err))
	}
	reader, err := zip.NewReader(bytes.NewReader(asset), int64(len(asset)))
	if err != nil {
		panic(fmt.Errorf("open embedded VS Code extension: %w", err))
	}
	for _, file := range reader.File {
		if file.Name != "extension/package.json" {
			continue
		}
		src, err := file.Open()
		if err != nil {
			panic(fmt.Errorf("open embedded VS Code package.json: %w", err))
		}
		content, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			panic(fmt.Errorf("read embedded VS Code package.json: %w", err))
		}
		var manifest struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(content, &manifest); err != nil {
			panic(fmt.Errorf("parse embedded VS Code package.json: %w", err))
		}
		if strings.TrimSpace(manifest.Version) == "" {
			panic(fmt.Errorf("embedded VS Code package.json is missing a version"))
		}
		return strings.TrimSpace(manifest.Version)
	}
	panic(fmt.Errorf("embedded VS Code package.json not found"))
}

func mustEmbeddedJetBrainsVersion() string {
	asset, err := embeddedAssets.ReadFile(jetbrainsAssetPath)
	if err != nil {
		panic(fmt.Errorf("read embedded JetBrains plugin: %w", err))
	}
	reader, err := zip.NewReader(bytes.NewReader(asset), int64(len(asset)))
	if err != nil {
		panic(fmt.Errorf("open embedded JetBrains plugin: %w", err))
	}
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, jetbrainsArchiveRoot+"/lib/") || !strings.HasSuffix(file.Name, ".jar") {
			continue
		}
		src, err := file.Open()
		if err != nil {
			panic(fmt.Errorf("open embedded JetBrains plugin jar: %w", err))
		}
		content, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			panic(fmt.Errorf("read embedded JetBrains plugin jar: %w", err))
		}
		jarReader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
		if err != nil {
			panic(fmt.Errorf("open embedded JetBrains plugin jar: %w", err))
		}
		descriptor, err := readJetBrainsPluginDescriptorFromZip(jarReader)
		if err == nil && strings.TrimSpace(descriptor.Version) != "" {
			return strings.TrimSpace(descriptor.Version)
		}
	}
	panic(fmt.Errorf("embedded JetBrains plugin version not found"))
}
