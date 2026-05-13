package gitignore

// Matcher defines a global multi-pattern matcher for gitignore patterns.
type Matcher interface {
	// Match matches patterns in the order of priorities. As soon as an
	// inclusion or exclusion is found, not further matching is performed.
	// Returns true if the path is ignored.
	Match(path []string, isDir bool) bool

	// MatchResult returns the highest-priority match result for the path,
	// or NoMatch if no pattern matched.
	MatchResult(path []string, isDir bool) MatchResult
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
	switch m.MatchResult(path, isDir) {
	case Exclude:
		return true
	case Include:
		return false
	default:
		return false
	}
}

func (m *matcher) MatchResult(path []string, isDir bool) MatchResult {
	for i := len(m.patterns) - 1; i >= 0; i-- {
		if match := m.patterns[i].Match(path, isDir); match > NoMatch {
			return match
		}
	}
	return NoMatch
}
