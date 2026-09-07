package integrations

import "testing"

func TestVerifyCompatibilityRejectsProtocolMismatch(t *testing.T) {
	report := VerifyCompatibility("v0.1.0", "vscode", "0.0.1", protocolVersion-1)
	if report.Compatible || report.Code != CodeProtocolMismatch {
		t.Fatalf("expected protocol mismatch report, got %+v", report)
	}
}

func TestVerifyCompatibilityRejectsUnknownIntegration(t *testing.T) {
	report := VerifyCompatibility("v0.1.0", "emacs", "0.0.1", protocolVersion)
	if report.Compatible || report.Code != CodeUnsupportedIntegration {
		t.Fatalf("expected unsupported integration report, got %+v", report)
	}
}

func TestVerifyCompatibilityAllowsDevBuild(t *testing.T) {
	jetbrainsVersion, err := expectedPluginVersion(FamilyJetBrains)
	if err != nil {
		t.Fatalf("could not get embedded JetBrains version: %v", err)
	}
	report := VerifyCompatibility("dev", "goland", jetbrainsVersion, protocolVersion)
	if !report.Compatible || report.Code != CodeOK {
		t.Fatalf("expected compatible report, got %+v", report)
	}
}

func TestVerifyCompatibilityVersionMismatchIncludesExpectedVersion(t *testing.T) {
	expected, err := expectedPluginVersion(FamilyVSCode)
	if err != nil {
		t.Fatalf("could not get embedded VS Code version: %v", err)
	}
	report := VerifyCompatibility("v0.2.0", "vscode", "0.0.1", protocolVersion)
	if report.Compatible || report.Code != CodeVersionMismatch {
		t.Fatalf("expected version mismatch report, got %+v", report)
	}
	if report.ExpectedVersion != expected {
		t.Fatalf("expected ExpectedVersion = %q, got %q", expected, report.ExpectedVersion)
	}
	if report.PluginVersion != "0.0.1" {
		t.Fatalf("expected PluginVersion = %q, got %q", "0.0.1", report.PluginVersion)
	}
}

func TestVerifyCompatibilityMatchingVersionIsCompatible(t *testing.T) {
	expected, err := expectedPluginVersion(FamilyVSCode)
	if err != nil {
		t.Fatalf("could not get embedded VS Code version: %v", err)
	}
	report := VerifyCompatibility("v0.2.0", "vscode", expected, protocolVersion)
	if !report.Compatible || report.Code != CodeOK {
		t.Fatalf("expected compatible report, got %+v", report)
	}
}
