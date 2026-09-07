package graph

import "strings"

var nodeIDReplacer = strings.NewReplacer(
	"/", "_slash_",
	".", "_dot_",
	"-", "_dash_",
	"*", "_asterisk_",
	"@", "_atsign_",
	"[", "_lsqb_",
	"]", "_rsqb_",
	"{", "_lbrace_",
	"}", "_rbrace_",
	"+", "_plus_",
	"?", "_qmark_",
	",", "_comma_",
	" ", "_space_",
	"\t", "_tab_",
	"\n", "_newline_",
	"\r", "_carriage_return_",
	"\x0b", "_vertical_tab_",
	"\x0c", "_form_feed_",
)

// IsNodeID reports whether s can be written as a node id verbatim, i.e. it is a
// mermaid identifier: [A-Za-z_][A-Za-z0-9_]*.
func IsNodeID(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return s != ""
}

// NodeID encodes a glob into a mermaid identifier. It is the identity on ids
// that are already identifiers, so encoding an encoded id is a no-op.
func NodeID(glob string) string {
	if IsNodeID(glob) {
		return glob
	}
	if glob == "" || glob == "." {
		return "root"
	}
	var b strings.Builder
	for _, c := range []byte(nodeIDReplacer.Replace(glob)) {
		switch {
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			b.WriteByte(c)
		case c >= '0' && c <= '9':
			if b.Len() == 0 {
				b.WriteByte('n')
			}
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "root"
	}
	return b.String()
}
