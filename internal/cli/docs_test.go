package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCLIAssetsListEveryLanguage(t *testing.T) {
	for _, asset := range []string{"check-usage.txt", "dump-usage.txt", "help-intro.txt"} {
		doc := readFile(t, "../../docs/cli-assets/"+asset)
		for _, name := range languageNames {
			if !regexp.MustCompile(`\b` + name + `\b`).MatchString(doc) {
				t.Errorf("docs/cli-assets/%s never names language %q", asset, name)
			}
		}
	}
}

func TestManualDocumentsEveryRuleID(t *testing.T) {
	manual := readFile(t, "../../docs/manual.md")
	sources, err := filepath.Glob("../application/usecase/check/*.go")
	if err != nil {
		t.Fatal(err)
	}
	ruleID := regexp.MustCompile(`"([a-z]+(?:-[a-z]+)+)"`)
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		for _, m := range ruleID.FindAllStringSubmatch(readFile(t, source), -1) {
			if !strings.Contains(manual, "`"+m[1]+"`") {
				t.Errorf("%s: rule id %q is not in the docs/manual.md tables", filepath.Base(source), m[1])
			}
		}
	}
}
