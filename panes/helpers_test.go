package panes

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/0xguy07/sf-tui/sf"
	"github.com/charmbracelet/bubbles/list"
)

func TestItoa(t *testing.T) {
	for n, want := range map[int]string{0: "0", 7: "7", 42: "42", -5: "-5", 1234567: "1234567"} {
		if got := itoa(n); got != want {
			t.Errorf("itoa(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestCompactRanges(t *testing.T) {
	cases := []struct {
		in   []int
		want string
	}{
		{nil, ""},
		{[]int{4}, "4"},
		{[]int{1, 2, 3}, "1-3"},
		{[]int{1, 2, 3, 7, 10, 11, 12}, "1-3, 7, 10-12"},
		{[]int{1, 3, 5}, "1, 3, 5"},
	}
	for _, c := range cases {
		if got := compactRanges(c.in); got != c.want {
			t.Errorf("compactRanges(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruncateHelpers(t *testing.T) {
	type fn struct {
		name string
		f    func(string, int) string
	}
	for _, h := range []fn{{"truncate", truncate}, {"oneLine", oneLine}, {"truncateOne", truncateOne}} {
		if got := h.f("short", 10); got != "short" {
			t.Errorf("%s left-short = %q", h.name, got)
		}
		if got := h.f("abcdefghij", 5); got != "abcd…" {
			t.Errorf("%s(abcdefghij,5) = %q, want abcd…", h.name, got)
		}
		// Multi-byte text must never be cut mid-rune.
		if got := h.f("ééééééé", 4); !utf8.ValidString(got) {
			t.Errorf("%s produced invalid UTF-8: %q", h.name, got)
		}
	}
	if got := oneLine("a\nb", 10); got != "a b" {
		t.Errorf("oneLine newline = %q", got)
	}
	if got := truncateOne("a\nb", 10); got != "a b" {
		t.Errorf("truncateOne newline = %q", got)
	}
}

func TestClip(t *testing.T) {
	if got := clip("abcdef", 4); got != "abc…" {
		t.Errorf("clip = %q", got)
	}
	if got := clip("abcdef", 1); got != "abcdef" {
		t.Errorf("clip width<=1 should not truncate, got %q", got)
	}
	if got := clip("a\nb", 10); got != "a\nb" {
		// short strings are returned untouched; only over-width strings are flattened
		t.Errorf("clip short = %q", got)
	}
	if got := clip("ab\ncdefgh", 5); strings.Contains(got, "\n") {
		t.Errorf("clip should strip newlines when truncating, got %q", got)
	}
	if got := clip("ééééééé", 4); !utf8.ValidString(got) {
		t.Errorf("clip produced invalid UTF-8: %q", got)
	}
}

func TestFormatCell(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hi", "hi"},
		{true, "true"},
		{false, "false"},
		{float64(3), "3"},
		{3.25, "3.25"},
		{map[string]any{"attributes": 1}, "{relationship}"},
		{[]any{1, 2, 3}, "[3]"},
		{int(9), "9"},
	}
	for _, c := range cases {
		if got := formatCell(c.in, 40); got != c.want {
			t.Errorf("formatCell(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStringValue(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"x", "x"},
		{true, "true"},
		{float64(12), "12"},
		{1.5, "1.5"},
	}
	for _, c := range cases {
		if got := stringValue(c.in); got != c.want {
			t.Errorf("stringValue(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOrDashAndFmtValue(t *testing.T) {
	if orDash("") != "∅" || orDash("x") != "x" {
		t.Error("orDash")
	}
	if got := fmtValue(float64(5)); got != "5" {
		t.Errorf("fmtValue(5.0) = %q", got)
	}
	if got := fmtValue("a\nb"); got != "a b" {
		t.Errorf("fmtValue newline = %q", got)
	}
	if got := fmtValue(strings.Repeat("x", 200)); len(got) > 82 {
		t.Errorf("fmtValue should truncate long values, len=%d", len(got))
	}
}

func TestTypeDescription(t *testing.T) {
	cases := []struct {
		f    sf.Field
		want string
	}{
		{sf.Field{Type: "string", Length: 80, Nillable: true}, "string(80)"},
		{sf.Field{Type: "boolean", Nillable: true}, "boolean"},
		{sf.Field{Type: "reference", ReferenceTo: []string{"Account", "Lead"}, Nillable: true}, "reference→Account,Lead"},
		{sf.Field{Type: "double", Nillable: false, Custom: true}, "double  [required,custom]"},
	}
	for _, c := range cases {
		if got := typeDescription(c.f); got != c.want {
			t.Errorf("typeDescription = %q, want %q", got, c.want)
		}
	}
}

func TestGlyphs(t *testing.T) {
	for _, s := range []string{"only-a", "only-b", "both"} {
		if strings.TrimSpace(diffGlyph(s)) == "" {
			t.Errorf("diffGlyph(%q) empty", s)
		}
	}
	if diffGlyph("zzz") != " " {
		t.Error("unknown diff state should be blank")
	}
	for _, a := range []string{"Deploy", "Delete", "Conflict", "Ignored"} {
		if strings.TrimSpace(actionGlyph(a)) == "" {
			t.Errorf("actionGlyph(%q) empty", a)
		}
	}
	if actionGlyph("zzz") != " " {
		t.Error("unknown action should be blank")
	}
}

func TestObjectsPrefixHelpers(t *testing.T) {
	mk := func(n string) list.Item { return sobjectItem{s: sf.SObjectSummary{Name: n}} }
	items := []list.Item{mk("Account"), mk("Contact"), mk("custom__c")}
	if got := findPrefix(items, "co"); got != 1 {
		t.Errorf("findPrefix co = %d", got)
	}
	if got := findPrefix(items, "cu"); got != 2 {
		t.Errorf("findPrefix is case-insensitive on names, got %d", got)
	}
	if got := findPrefix(items, "zz"); got != -1 {
		t.Errorf("findPrefix miss = %d", got)
	}
	if firstLetter(items[0]) != 'a' || firstLetter(items[2]) != 'c' {
		t.Error("firstLetter should lowercase")
	}
	for r, want := range map[rune]bool{'a': true, 'Z': true, '5': true, '_': true, '-': false, ' ': false} {
		if isJumpRune(r) != want {
			t.Errorf("isJumpRune(%q) != %v", r, want)
		}
	}
}
