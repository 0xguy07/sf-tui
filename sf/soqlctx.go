package sf

import (
	"regexp"
	"strings"
)

type CompletionKind int

const (
	KindNone CompletionKind = iota
	KindSObject
	KindField
	KindKeyword
)

type SOQLContext struct {
	Kind       CompletionKind
	Prefix     string   // current partial token (may be empty)
	PrefixStart int     // byte offset in the full text where the prefix begins
	SObject    string   // inferred FROM object, if known
	// Reference chain: e.g. "Account.Owner." → []string{"Account", "Owner"}; current prefix starts fresh after the last dot.
	RefChain []string
}

var (
	// Match FROM <Name> ignoring case. We look at the last FROM before the cursor.
	fromRe = regexp.MustCompile(`(?i)\bfrom\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// Word boundary for the current token being typed at the cursor.
	// Accept letters, digits, underscore, and dots (for relationship walks).
	tokenTailRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]*)*$`)
)

// Analyze inspects text up to the cursor position and returns what kind of
// completion (if any) the user is likely after.
func Analyze(text string, cursor int) SOQLContext {
	if cursor > len(text) {
		cursor = len(text)
	}
	left := text[:cursor]

	// Find current token at the cursor (may be empty).
	tok := tokenTailRe.FindString(left)
	prefixStart := len(left) - len(tok)

	// Find FROM object anywhere before the cursor (last match wins).
	sobject := ""
	if ms := fromRe.FindAllStringSubmatch(left, -1); len(ms) > 0 {
		sobject = ms[len(ms)-1][1]
	}

	// Determine the clause context — what keyword most recently preceded the cursor?
	clause := lastClauseBefore(left, prefixStart)

	ctx := SOQLContext{Prefix: tok, PrefixStart: prefixStart, SObject: sobject}

	// Dotted path → resolve each segment against the previous SObject via reference fields.
	if strings.Contains(tok, ".") {
		parts := strings.Split(tok, ".")
		ctx.RefChain = parts[:len(parts)-1]
		ctx.Prefix = parts[len(parts)-1]
		ctx.Kind = KindField
		return ctx
	}

	switch clause {
	case "select", "where", "group by", "order by", "having":
		if sobject != "" {
			ctx.Kind = KindField
		} else {
			ctx.Kind = KindNone
		}
	case "from":
		ctx.Kind = KindSObject
	default:
		ctx.Kind = KindNone
	}

	return ctx
}

// lastClauseBefore scans left-to-right and tracks the most recent SOQL clause
// keyword that applies at the cursor. This is intentionally loose: it doesn't
// parse the full grammar, just enough to decide "field vs sobject vs none."
func lastClauseBefore(left string, prefixStart int) string {
	scan := strings.ToLower(left[:prefixStart])
	// Order matters: check longer phrases first.
	keywords := []string{"group by", "order by", "having", "select", "from", "where"}

	idx := map[string]int{}
	for _, k := range keywords {
		if i := lastIndexWord(scan, k); i >= 0 {
			idx[k] = i
		}
	}
	best := ""
	bestAt := -1
	for k, at := range idx {
		if at > bestAt {
			bestAt = at
			best = k
		}
	}
	return best
}

// lastIndexWord finds the last occurrence of phrase in s where the phrase is
// surrounded by word boundaries (start/end of string or non-word char).
func lastIndexWord(s, phrase string) int {
	start := 0
	last := -1
	for {
		i := strings.Index(s[start:], phrase)
		if i < 0 {
			break
		}
		abs := start + i
		if isWordBoundary(s, abs) && isWordBoundary(s, abs+len(phrase)) {
			last = abs
		}
		start = abs + 1
		if start >= len(s) {
			break
		}
	}
	return last
}

func isWordBoundary(s string, pos int) bool {
	if pos <= 0 || pos >= len(s) {
		return true
	}
	return !isWordChar(s[pos-1]) || !isWordChar(s[pos])
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_'
}
