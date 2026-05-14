package integrations

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallJetBrainsArchiveReplacesExistingBAFTPlugin(t *testing.T) {
	pluginDir := t.TempDir()
	targetDir := filepath.Join(pluginDir, jetbrainsArchiveRoot)
	if err := os.MkdirAll(filepath.Join(targetDir, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "lib", "baft-intellij-0.0.9.jar"), pluginJar(t, jetbrainsPluginID, "BAFT", "0.0.9"), 0o644); err != nil {
		t.Fatalf("WriteFile old jar: %v", err)
	}

	archive := pluginArchive(t, jetbrainsArchiveRoot, "baft-intellij-0.2.0.jar", jetbrainsPluginID, "BAFT", "0.2.0")
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	err = installJetBrainsArchive(IDEInstallation{DisplayName: "GoLand", PluginDir: pluginDir}, reader)
	if err != nil {
		t.Fatalf("installJetBrainsArchive returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "lib", "baft-intellij-0.2.0.jar")); err != nil {
		t.Fatalf("expected new jar to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "lib", "baft-intellij-0.0.9.jar")); !os.IsNotExist(err) {
		t.Fatalf("expected old jar to be removed, stat err = %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(pluginDir, jetbrainsArchiveRoot+".backup-*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("expected backup directory cleanup, found %v", backups)
	}
}

func TestInstallJetBrainsArchiveRefusesUnknownExistingPlugin(t *testing.T) {
	pluginDir := t.TempDir()
	targetDir := filepath.Join(pluginDir, jetbrainsArchiveRoot)
	if err := os.MkdirAll(filepath.Join(targetDir, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "lib", "other-plugin.jar"), pluginJar(t, "com.example.other", "Other Plugin", "1.0.0"), 0o644); err != nil {
		t.Fatalf("WriteFile old jar: %v", err)
	}

	archive := pluginArchive(t, jetbrainsArchiveRoot, "baft-intellij-0.2.0.jar", jetbrainsPluginID, "BAFT", "0.2.0")
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	err = installJetBrainsArchive(IDEInstallation{DisplayName: "GoLand", PluginDir: pluginDir}, reader)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Manual install:") {
		t.Fatalf("expected manual install instructions, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(targetDir, "lib", "other-plugin.jar")); statErr != nil {
		t.Fatalf("expected existing plugin to remain in place: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(targetDir, "lib", "baft-intellij-0.2.0.jar")); !os.IsNotExist(statErr) {
		t.Fatalf("expected new BAFT plugin not to be installed, stat err = %v", statErr)
	}
}

func TestJetBrainsVerifyRejectsStaleInstalledVersion(t *testing.T) {
	pluginDir := t.TempDir()
	targetDir := filepath.Join(pluginDir, jetbrainsArchiveRoot)
	if err := os.MkdirAll(filepath.Join(targetDir, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	staleVersion := "0.1.0"
	if err := os.WriteFile(filepath.Join(targetDir, "lib", "baft-intellij-"+staleVersion+".jar"), pluginJar(t, jetbrainsPluginID, "BAFT", staleVersion), 0o644); err != nil {
		t.Fatalf("WriteFile stale jar: %v", err)
	}

	installer := &jetbrainsInstaller{cliVersion: "v0.1.0"}
	err := installer.Verify(context.Background(), IDEInstallation{
		ID:          "goland",
		DisplayName: "GoLand",
		PluginDir:   pluginDir,
	})
	if err == nil {
		t.Fatal("expected version mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "expected "+expectedPluginVersion(FamilyJetBrains)) {
		t.Fatalf("expected version mismatch message, got %v", err)
	}
}

func pluginArchive(t *testing.T, rootDir, jarName, pluginID, name, version string) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	if _, err := writer.Create(rootDir + "/"); err != nil {
		t.Fatalf("Create root dir: %v", err)
	}
	if _, err := writer.Create(rootDir + "/lib/"); err != nil {
		t.Fatalf("Create lib dir: %v", err)
	}
	jarFile, err := writer.Create(rootDir + "/lib/" + jarName)
	if err != nil {
		t.Fatalf("Create jar file: %v", err)
	}
	if _, err := jarFile.Write(pluginJar(t, pluginID, name, version)); err != nil {
		t.Fatalf("Write jar file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close archive: %v", err)
	}
	return archive.Bytes()
}

func pluginJar(t *testing.T, pluginID, name, version string) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	pluginXML, err := writer.Create("META-INF/plugin.xml")
	if err != nil {
		t.Fatalf("Create plugin.xml: %v", err)
	}
	content := fmt.Sprintf("<idea-plugin><id>%s</id><name>%s</name><version>%s</version></idea-plugin>", pluginID, name, version)
	if _, err := pluginXML.Write([]byte(content)); err != nil {
		t.Fatalf("Write plugin.xml: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close jar archive: %v", err)
	}
	return archive.Bytes()
}
