package integrations

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// findTestBinary locates the baft binary for contract tests.
// Contract tests require an explicit BAFT_BINARY env var pointing to the
// built binary, so that tests don't silently run against a different
// installed version. Without it, the tests are skipped.
func findTestBinary(t *testing.T) string {
	t.Helper()

	if path := os.Getenv("BAFT_BINARY"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		t.Fatalf("BAFT_BINARY=%s does not exist", path)
	}

	t.Skip("contract tests skipped — set BAFT_BINARY to the path of the built baft binary")
	return ""
}

func TestContractVerifyCompatibleMatchingVersion(t *testing.T) {
	binary := findTestBinary(t)
	protocol := expectedProtocol(FamilyVSCode)
	expectedVersion := expectedPluginVersion(FamilyVSCode)

	cmd := exec.Command(binary, "integrate", "--verify-compatible",
		"--integration=vscode",
		"--plugin-version="+expectedVersion,
		"--protocol="+formatInt(protocol),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got exit error: %v\noutput: %s", err, out)
	}

	var report CompatibilityReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, out)
	}

	if !report.Compatible {
		t.Fatalf("expected compatible=true, got compatible=false; message: %s", report.Message)
	}

	if report.IntegrationID != "vscode" {
		t.Fatalf("expected integration_id=vscode, got %s", report.IntegrationID)
	}
}

func TestContractVerifyCompatibleVersionMismatch(t *testing.T) {
	binary := findTestBinary(t)
	protocol := expectedProtocol(FamilyVSCode)

	cmd := exec.Command(binary, "integrate", "--verify-compatible",
		"--integration=vscode",
		"--plugin-version=0.0.1",
		"--protocol="+formatInt(protocol),
	)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code for version mismatch")
	}

	var report CompatibilityReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, out)
	}

	if report.Compatible {
		t.Fatalf("expected compatible=false, got true")
	}

	if report.ExpectedVersion == "" {
		t.Fatal("expected expected_version to be populated, got empty string")
	}

	if report.PluginVersion != "0.0.1" {
		t.Fatalf("expected plugin_version=0.0.1, got %s", report.PluginVersion)
	}

	if !strings.Contains(report.Message, "version mismatch") {
		t.Fatalf("expected message to contain 'version mismatch', got: %s", report.Message)
	}
}

func TestContractVerifyCompatibleProtocolMismatch(t *testing.T) {
	binary := findTestBinary(t)
	expectedVersion := expectedPluginVersion(FamilyVSCode)

	wrongProtocol := expectedProtocol(FamilyVSCode) - 1
	cmd := exec.Command(binary, "integrate", "--verify-compatible",
		"--integration=vscode",
		"--plugin-version="+expectedVersion,
		"--protocol="+formatInt(wrongProtocol),
	)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code for protocol mismatch")
	}

	var report CompatibilityReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, out)
	}

	if report.Compatible {
		t.Fatalf("expected compatible=false, got true")
	}

	if !strings.Contains(report.Message, "protocol mismatch") {
		t.Fatalf("expected message to contain 'protocol mismatch', got: %s", report.Message)
	}
}

func TestContractVerifyCompatibleMissingFlags(t *testing.T) {
	binary := findTestBinary(t)

	cmd := exec.Command(binary, "integrate", "--verify-compatible")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code when required flags are missing")
	}

	output := string(out)
	if !strings.Contains(output, "requires") {
		t.Fatalf("expected error message about required flags, got: %s", output)
	}
}

func TestContractIntegrateHelp(t *testing.T) {
	binary := findTestBinary(t)

	cmd := exec.Command(binary, "integrate", "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}

	output := string(out)
	if !strings.Contains(output, "--yes") {
		t.Fatal("expected --help output to mention --yes flag")
	}

	if !strings.Contains(output, "--integration") {
		t.Fatal("expected --help output to mention --integration flag")
	}
}

func TestContractVersionOutput(t *testing.T) {
	binary := findTestBinary(t)

	cmd := exec.Command(binary, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}

	version := strings.TrimSpace(string(out))
	if version == "" {
		t.Fatal("expected non-empty version output")
	}
}

func formatInt(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
