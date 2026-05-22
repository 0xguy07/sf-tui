package sf

import (
	"regexp"
	"strings"
)

var (
	selectStartRe = regexp.MustCompile(`(?i)^\s*select\s+`)
	sobjectRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)
)

// SelectInfo describes the columns and target SObject parsed from a SOQL SELECT.
type SelectInfo struct {
	Columns []string // dotted paths in source order, with subqueries dropped
	SObject string
}

// ParseSelect extracts the column list and FROM target. Returns ok=false if
// the SOQL doesn't parse cleanly (subqueries, malformed input, etc.). Tracks
// parenthesis depth so a subquery's inner FROM doesn't fool the outer parse.
func ParseSelect(soql string) (SelectInfo, bool) {
	loc := selectStartRe.FindStringIndex(soql)
	if loc == nil {
		return SelectInfo{}, false
	}
	rest := soql[loc[1]:]
	// Walk forward, tracking depth, to find the outer FROM keyword.
	depth := 0
	fromAt := -1
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && i+4 <= len(rest) {
			// Match \bfrom\b case-insensitively at this position.
			if (rest[i] == 'f' || rest[i] == 'F') &&
				(rest[i+1] == 'r' || rest[i+1] == 'R') &&
				(rest[i+2] == 'o' || rest[i+2] == 'O') &&
				(rest[i+3] == 'm' || rest[i+3] == 'M') &&
				!isWord(rest, i-1) && !isWord(rest, i+4) {
				fromAt = i
				break
			}
		}
	}
	if fromAt < 0 {
		return SelectInfo{}, false
	}
	rawCols := rest[:fromAt]
	afterFrom := strings.TrimLeft(rest[fromAt+4:], " \t\n\r")
	m := sobjectRe.FindString(afterFrom)
	if m == "" {
		return SelectInfo{}, false
	}
	sobject := m

	cols := splitTopLevel(rawCols)
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		// Drop subqueries entirely; they need their own table.
		if strings.HasPrefix(c, "(") {
			continue
		}
		// Drop aggregate functions and aliases for now (e.g. "COUNT(Id) total").
		if strings.ContainsAny(c, "() ") {
			continue
		}
		out = append(out, c)
	}
	return SelectInfo{Columns: out, SObject: sobject}, true
}

func isWord(s string, pos int) bool {
	if pos < 0 || pos >= len(s) {
		return false
	}
	c := s[pos]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_'
}

// splitTopLevel splits a comma-separated list while respecting parenthesis
// depth (so subqueries aren't broken apart).
func splitTopLevel(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// ResolvePath walks a dotted path through a record. Missing intermediate
// values return "". A non-nil terminal value is stringified by the caller.
func ResolvePath(rec QueryRecord, path string) any {
	parts := strings.Split(path, ".")
	var cur any = map[string]any(rec)
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		// Salesforce JSON keys come back PascalCase; users may type lowercase.
		// Match exact first, then case-insensitively.
		if v, ok := m[p]; ok {
			cur = v
			continue
		}
		lower := strings.ToLower(p)
		matched := false
		for k, v := range m {
			if strings.ToLower(k) == lower {
				cur = v
				matched = true
				break
			}
		}
		if !matched {
			return nil
		}
	}
	return cur
}
