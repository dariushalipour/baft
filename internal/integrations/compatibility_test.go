package integrations

import "testing"

func TestVerifyCompatibilityRejectsProtocolMismatch(t *testing.T) {
	report := VerifyCompatibility("v0.1.0", "vscode", "0.0.1", expectedProtocol(FamilyVSCode)-1)
	if report.Compatible {
		t.Fatalf("expected incompatible report, got %+v", report)
	}
}

func TestVerifyCompatibilityAllowsDevBuild(t *testing.T) {
	report := VerifyCompatibility("dev", "goland", expectedPluginVersion(FamilyJetBrains), expectedProtocol(FamilyJetBrains))
	if !report.Compatible {
		t.Fatalf("expected compatible report, got %+v", report)
	}
	if report.Warning == "" {
		t.Fatalf("expected warning for dev build, got %+v", report)
	}
}

func TestVerifyCompatibilityVersionMismatchIncludesExpectedVersion(t *testing.T) {
	expected := expectedPluginVersion(FamilyVSCode)
	report := VerifyCompatibility("v0.2.0", "vscode", "0.0.1", expectedProtocol(FamilyVSCode))
	if report.Compatible {
		t.Fatalf("expected incompatible report, got %+v", report)
	}
	if report.ExpectedVersion != expected {
		t.Fatalf("expected ExpectedVersion = %q, got %q", expected, report.ExpectedVersion)
	}
	if report.PluginVersion != "0.0.1" {
		t.Fatalf("expected PluginVersion = %q, got %q", "0.0.1", report.PluginVersion)
	}
}

func TestVerifyCompatibilityMatchingVersionIsCompatible(t *testing.T) {
	expected := expectedPluginVersion(FamilyVSCode)
	report := VerifyCompatibility("v0.2.0", "vscode", expected, expectedProtocol(FamilyVSCode))
	if !report.Compatible {
		t.Fatalf("expected compatible report, got %+v", report)
	}
}
