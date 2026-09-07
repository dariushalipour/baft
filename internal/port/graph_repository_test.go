package port

import "testing"

func TestParseErrorMessage(t *testing.T) {
	for want, err := range map[string]ParseError{
		"bad edge (BAFT.md:7)": {File: "BAFT.md", Line: 7, Msg: "bad edge"},
		"bad edge (BAFT.md)":   {File: "BAFT.md", Msg: "bad edge"},
		"bad edge (line 7)":    {Line: 7, Msg: "bad edge"},
		"bad edge":             {Msg: "bad edge"},
		"unrecognized mermaid line: a --> (line 7)": {Line: 7, Raw: "  a --> \n"},
	} {
		if got := err.Error(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}
