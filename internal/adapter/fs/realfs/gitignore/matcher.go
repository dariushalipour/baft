package gitignore

// Matcher defines a global multi-pattern matcher for gitignore patterns.
type Matcher interface {
	// Match matches patterns in the order of priorities. As soon as an
	// inclusion or exclusion is found, not further matching is performed.
	// Returns true if the path is ignored.
	Match(path []string, isDir bool) bool
}

type matcher struct {
	patterns []Pattern
}

// NewMatcher constructs a new global matcher. Patterns must be given in the
// order of increasing priority. That is most generic settings files first,
// then the content of the repo .gitignore, then content of .gitignore down
// the path, and then the content of command line arguments.
func NewMatcher(ps []Pattern) Matcher {
	return &matcher{ps}
}

func (m *matcher) Match(path []string, isDir bool) bool {
	for i := len(m.patterns) - 1; i >= 0; i-- {
		if match := m.patterns[i].Match(path, isDir); match > NoMatch {
			return match == Exclude
		}
	}
	return false
}
