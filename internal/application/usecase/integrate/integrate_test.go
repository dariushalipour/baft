package integrate

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/integrations"
)

type fakeManager struct {
	detected   []integrations.IDEInstallation
	detectErr  error
	installed  []string
	verified   []string
	installErr error
	verifyErr  error
}

func (f *fakeManager) Detect(context.Context) ([]integrations.IDEInstallation, error) {
	return append([]integrations.IDEInstallation(nil), f.detected...), f.detectErr
}

func (f *fakeManager) Install(_ context.Context, ide integrations.IDEInstallation) error {
	f.installed = append(f.installed, ide.ID)
	return f.installErr
}

func (f *fakeManager) Verify(_ context.Context, ide integrations.IDEInstallation) error {
	f.verified = append(f.verified, ide.ID)
	return f.verifyErr
}

func TestRunInstallsSelectedIntegration(t *testing.T) {
	manager := &fakeManager{
		detected: []integrations.IDEInstallation{
			{ID: "vscode", Family: integrations.FamilyVSCode, DisplayName: "VS Code", Version: "1.100.0"},
			{ID: "goland", Family: integrations.FamilyJetBrains, DisplayName: "GoLand", Version: "2026.1"},
		},
	}
	var out bytes.Buffer

	err := Run(context.Background(), manager, Options{
		In:  strings.NewReader("1\n"),
		Out: &out,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got, want := manager.installed, []string{"goland"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("installed = %v, want %v", got, want)
	}
	if got, want := manager.verified, []string{"goland"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("verified = %v, want %v", got, want)
	}
	if !strings.Contains(out.String(), "Available integrations:") {
		t.Fatalf("output did not include integration list: %s", out.String())
	}
	if !strings.Contains(out.String(), "GoLand 2026.1") {
		t.Fatalf("output did not include selected integration label: %s", out.String())
	}
}

func TestRunRepromptsInvalidSelection(t *testing.T) {
	manager := &fakeManager{
		detected: []integrations.IDEInstallation{{
			ID:          "vscode",
			Family:      integrations.FamilyVSCode,
			DisplayName: "VS Code",
			Version:     "1.100.0",
		}},
	}
	var out bytes.Buffer

	err := Run(context.Background(), manager, Options{
		In:  strings.NewReader("9\n1\n"),
		Out: &out,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "Enter a number between 1 and 1.") {
		t.Fatalf("output did not include validation prompt: %s", out.String())
	}
}

func TestRunReturnsDetectErrorWhenNothingFound(t *testing.T) {
	manager := &fakeManager{detectErr: errors.New("detector failed")}

	err := Run(context.Background(), manager, Options{In: strings.NewReader("")})
	if err == nil || err.Error() != "detector failed" {
		t.Fatalf("Run error = %v, want detector failed", err)
	}
}
