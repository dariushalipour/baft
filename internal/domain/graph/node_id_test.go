package graph

import "testing"

func TestNodeID(t *testing.T) {
	cases := []struct {
		glob, id string
	}{
		{"src/domain", "src_slash_domain"},
		{"src/model.ts", "src_slash_model_dot_ts"},
		{"my-pkg", "my_dash_pkg"},
		{"internal/api/**", "internal_slash_api_slash__asterisk__asterisk_"},
		{"@scope/pkg", "_atsign_scope_slash_pkg"},
		{"pkg[name]", "pkg_lsqb_name_rsqb_"},
		{"pkg{ver}", "pkg_lbrace_ver_rbrace_"},
		{".", "root"},
		{"", "root"},
		{"123abc", "n123abc"},
		{"my+pkg", "my_plus_pkg"},
		{"a?b", "a_qmark_b"},
		{"x,y", "x_comma_y"},
		{"hello world", "hello_space_world"},
		{"a\tb", "a_tab_b"},
		{"a\nb", "a_newline_b"},
		{"a\rb", "a_carriage_return_b"},
		{"a\x0bb", "a_vertical_tab_b"},
		{"a\x0cb", "a_form_feed_b"},
		{"a#b", "a_b"},
		{"日本", "______"},
	}
	for _, tc := range cases {
		if got := NodeID(tc.glob); got != tc.id {
			t.Errorf("NodeID(%q) = %q, want %q", tc.glob, got, tc.id)
		}
		if got := NodeID(tc.id); got != tc.id {
			t.Errorf("NodeID(%q) = %q, want it to be the identity on ids", tc.id, got)
		}
		if !IsNodeID(tc.id) {
			t.Errorf("IsNodeID(%q) = false, want true", tc.id)
		}
	}
}

func TestIsNodeID(t *testing.T) {
	valid := []string{"root", "n8n", "Already_Lower", "_x", "a1"}
	invalid := []string{"", ".", "1a", "a-b", "a/b", "a b", "a#b"}
	for _, s := range valid {
		if !IsNodeID(s) {
			t.Errorf("IsNodeID(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if IsNodeID(s) {
			t.Errorf("IsNodeID(%q) = true, want false", s)
		}
	}
}
