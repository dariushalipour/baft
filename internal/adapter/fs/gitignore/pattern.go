package gitignore

import (
	"strings"
)

// MatchResult defines outcomes of a match: no match, exclusion or inclusion.
type MatchResult int

const (
	// NoMatch defines the no match outcome of a match check.
	NoMatch MatchResult = iota
	// Exclude defines an exclusion of a file as a result of a match check.
	Exclude
	// Include defines an explicit inclusion of a file as a result of a match check.
	Include
)

const (
	inclusionPrefix = "!"
	zeroToManyDirs  = "**"
	patternDirSep   = "/"
)

// Pattern defines a single gitignore pattern.
type Pattern interface {
	// Match matches the given path to the pattern.
	Match(path []string, isDir bool) MatchResult
}

type pattern struct {
	domain    []string
	pattern   []string
	inclusion bool
	dirOnly   bool
	isGlob    bool
}

// ParsePattern parses a gitignore pattern string into the Pattern structure.
func ParsePattern(p string, domain []string) Pattern {
	domain = append([]string(nil), domain...)
	res := pattern{domain: domain}

	if strings.HasPrefix(p, inclusionPrefix) {
		res.inclusion = true
		p = p[1:]
	}

	if !strings.HasSuffix(p, "\\ ") {
		p = strings.TrimRight(p, " ")
	}

	if strings.HasSuffix(p, patternDirSep) {
		res.dirOnly = true
		p = p[:len(p)-1]
	}

	if strings.Contains(p, patternDirSep) {
		res.isGlob = true
	}

	res.pattern = strings.Split(p, patternDirSep)
	return &res
}

func (p *pattern) Match(path []string, isDir bool) MatchResult {
	if len(path) <= len(p.domain) {
		return NoMatch
	}
	for i, e := range p.domain {
		if path[i] != e {
			return NoMatch
		}
	}

	path = path[len(p.domain):]
	if p.isGlob && !p.globMatch(path, isDir) {
		return NoMatch
	} else if !p.isGlob && !p.simpleNameMatch(path, isDir) {
		return NoMatch
	}

	if p.inclusion {
		return Include
	}
	return Exclude
}

func wildmatch(pattern, text string) bool {
	pi, ti := 0, 0
	plen, tlen := len(pattern), len(text)

	starIdx, matchIdx := -1, -1

	for ti < tlen {
		if pi < plen {
			pc := pattern[pi]

			switch pc {
			case '\\':
				if pi+1 >= plen {
					break
				}
				pi++
				if pattern[pi] == text[ti] {
					pi++
					ti++
					continue
				}

			case '?':
				pi++
				ti++
				continue

			case '*':
				starIdx = pi
				matchIdx = ti
				pi++
				continue

			case '[':
				bracketEnd := findBracketEnd(pattern, pi)
				if bracketEnd > pi && matchBracket(pattern[pi:bracketEnd+1], text[ti]) {
					pi = bracketEnd + 1
					ti++
					continue
				}
				if starIdx >= 0 {
					pi = starIdx + 1
					matchIdx++
					ti = matchIdx
					continue
				}
				return false

			default:
				if pc == text[ti] {
					pi++
					ti++
					continue
				}
			}
		}

		if starIdx >= 0 {
			pi = starIdx + 1
			matchIdx++
			ti = matchIdx
			continue
		}

		return false
	}

	for pi < plen && pattern[pi] == '*' {
		pi++
	}

	return pi == plen
}

func findBracketEnd(pattern string, start int) int {
	if start >= len(pattern) || pattern[start] != '[' {
		return -1
	}

	i := start + 1
	if i < len(pattern) && (pattern[i] == '!' || pattern[i] == '^') {
		i++
	}
	if i < len(pattern) && pattern[i] == ']' {
		i++
	}

	for i < len(pattern) {
		if pattern[i] == ']' {
			return i
		}
		switch {
		case pattern[i] == '\\' && i+1 < len(pattern):
			i += 2
		case pattern[i] == '[' && i+1 < len(pattern) && pattern[i+1] == ':':
			if _, end, ok := parsePosixClass(pattern, i); ok {
				i = end
			} else {
				i++
			}
		default:
			i++
		}
	}
	return -1
}

func matchBracket(bracketExpr string, ch byte) bool {
	if len(bracketExpr) < 2 || bracketExpr[0] != '[' {
		return false
	}

	i := 1
	var prevCh byte
	var pCh byte
	negate := false
	matched := false

	if i >= len(bracketExpr) {
		return false
	}
	pCh = bracketExpr[i]
	if pCh == '^' {
		pCh = '!'
	}
	if pCh == '!' {
		negate = true
		i++
	}

	prevCh = 0

	for {
		if i >= len(bracketExpr) {
			return false
		}
		pCh = bracketExpr[i]

		switch {
		case pCh == '\\':
			i++
			if i >= len(bracketExpr) {
				return false
			}
			pCh = bracketExpr[i]
			if ch == pCh {
				matched = true
			}
		case pCh == '-' && prevCh != 0 && i+1 < len(bracketExpr) && bracketExpr[i+1] != ']':
			i++
			pCh = bracketExpr[i]
			if pCh == '\\' {
				i++
				if i >= len(bracketExpr) {
					return false
				}
				pCh = bracketExpr[i]
			}
			if ch <= pCh && ch >= prevCh {
				matched = true
			}
			pCh = 0
		case pCh == '[' && i+1 < len(bracketExpr) && bracketExpr[i+1] == ':':
			if className, end, ok := parsePosixClass(bracketExpr, i); ok {
				classMatch, valid := matchCharClass(className, ch)
				if !valid {
					return false
				}
				if classMatch {
					matched = true
				}
				i = end - 1
				pCh = 0
			} else if ch == '[' {
				matched = true
			}
		case ch == pCh:
			matched = true
		}

		prevCh = pCh
		i++
		if i < len(bracketExpr) && bracketExpr[i] == ']' {
			break
		}
	}

	if negate {
		return !matched
	}
	return matched
}

func parsePosixClass(s string, start int) (name string, end int, ok bool) {
	classStart := start + 2
	j := classStart
	for j < len(s) && s[j] != ']' {
		j++
	}
	if j >= len(s) || j == classStart || s[j-1] != ':' {
		return "", 0, false
	}
	return s[classStart : j-1], j + 1, true
}

func matchCharClass(class string, ch byte) (bool, bool) {
	switch class {
	case "alnum":
		return isASCIIAlpha(ch) || isASCIIDigit(ch), true
	case "alpha":
		return isASCIIAlpha(ch), true
	case "blank":
		return ch == ' ' || ch == '\t', true
	case "cntrl":
		return ch < 0x20 || ch == 0x7f, true
	case "digit":
		return isASCIIDigit(ch), true
	case "graph":
		return ch > ' ' && ch < 0x7f, true
	case "lower":
		return ch >= 'a' && ch <= 'z', true
	case "print":
		return ch >= ' ' && ch < 0x7f, true
	case "punct":
		return isASCIIPunct(ch), true
	case "space":
		return ch == ' ' || ch == '\t' || ch == '\n' ||
			ch == '\v' || ch == '\f' || ch == '\r', true
	case "upper":
		return ch >= 'A' && ch <= 'Z', true
	case "xdigit":
		return isASCIIDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F'), true
	default:
		return false, false
	}
}

func isASCIIAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isASCIIDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isASCIIPunct(ch byte) bool {
	return (ch >= '!' && ch <= '/') ||
		(ch >= ':' && ch <= '@') ||
		(ch >= '[' && ch <= '`') ||
		(ch >= '{' && ch <= '~')
}

func (p *pattern) simpleNameMatch(path []string, isDir bool) bool {
	for i, name := range path {
		if !wildmatch(p.pattern[0], name) {
			continue
		}
		if p.dirOnly && !isDir && i == len(path)-1 {
			return false
		}
		return true
	}
	return false
}

func (p *pattern) globMatch(path []string, isDir bool) bool {
	path = append(append([]string(nil), path...), "")
	defer func() { path[len(path)-1] = "" }()

	matched := false
	canTraverse := false
	trailingStar := false
	for i, pat := range p.pattern {
		if pat == "" {
			canTraverse = false
			continue
		}
		if pat == zeroToManyDirs {
			if i == len(p.pattern)-1 {
				if len(path) > 1 || isDir {
					matched = true
					trailingStar = true
				}
				break
			}
			canTraverse = true
			continue
		}
		if len(path) <= 1 {
			return false
		}
		if canTraverse {
			canTraverse = false
			for len(path) > 1 {
				e := path[0]
				path = path[1:]
				if wildmatch(pat, e) {
					matched = true
					break
				} else if len(path) == 1 {
					matched = false
				}
			}
		} else {
			if !wildmatch(pat, path[0]) {
				return false
			}
			matched = true
			path = path[1:]
			if len(path) == 1 && i < len(p.pattern)-1 {
				// Check if remaining pattern is only "**" — if so, allow early termination.
				if !stillHasMore(p.pattern, i) {
					break
				}
				matched = false
			}
		}
	}
	if matched && p.dirOnly && !isDir && (len(path) == 1 || trailingStar) {
		matched = false
	}
	return matched
}

// stillHasMore reports whether p[i+1:] contains any non-empty, non-"**" segments.
func stillHasMore(p []string, i int) bool {
	for j := i + 1; j < len(p); j++ {
		if p[j] != "" && p[j] != zeroToManyDirs {
			return true
		}
	}
	return false
}
