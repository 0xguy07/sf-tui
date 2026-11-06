package panes

// cutRunes shortens s to at most max display runes, ending with "…" when it
// had to cut. It slices on rune boundaries so multi-byte text (accented
// labels, CJK field names) is never split mid-character.
func cutRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
