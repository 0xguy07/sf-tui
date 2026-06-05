package sf

import (
	"fmt"
	"strings"
	"testing"
)

func TestFormatUpdateValues(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]string
		want string
	}{
		{"simple", map[string]string{"Name": "Acme"}, `Name="Acme"`},
		{"multi sorted", map[string]string{"Phone": "555", "Name": "Acme"}, `Name="Acme" Phone="555"`},
		{"empty value clears", map[string]string{"Name": ""}, `Name=""`},
		{"apostrophe uses double-quote wrap", map[string]string{"Name": "O'Brien"}, `Name="O'Brien"`},
		{"value with double quote uses single-quote wrap", map[string]string{"Note": `a "q" b`}, `Note='a "q" b'`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FormatUpdateValues(c.in)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestFormatUpdateValuesRoundTrip is the test that actually matters: it feeds the
// encoded --values string through a faithful port of the sf CLI's own parser
// (stringToDictionary / parseKeyValueSequence from salesforcecli/plugin-data) and
// asserts every field survives. Asserting the encoded string alone is not enough —
// the previous encoding looked plausible but the parser turned O'Brien into O\Brien.
func TestFormatUpdateValuesRoundTrip(t *testing.T) {
	cases := []map[string]string{
		{"Name": "Acme"},
		{"Name": "Acme Inc"},
		{"Name": "O'Brien"},
		{"Name": "Macy's"},
		{"City": "Coeur d'Alene"},
		{"Note": `a "quoted" word`},
		{"Note": "a=b"},
		{"Name": ""},
		{"Name": "O'Brien", "Website": "acme example com"},
		{"Name": "O'Brien", "City": "Coeur d'Alene"},
	}
	for _, in := range cases {
		encoded := FormatUpdateValues(in)
		got, err := sfParseValues(encoded)
		if err != nil {
			t.Fatalf("input %v encoded to %q, which the sf --values parser rejects: %v", in, encoded, err)
		}
		for k, want := range in {
			if got[k] != want {
				t.Errorf("field %q: round-trip got %q, want %q (encoded: %q)", k, got[k], want, encoded)
			}
		}
	}
}

// sfParseValues is a faithful Go port of parseKeyValueSequence +
// transformKeyValueSequence from salesforcecli/plugin-data (src/dataUtils.ts) —
// the exact logic `sf data update record --values` uses to parse its argument.
// Test-only: it lets us prove round-trip fidelity without a live org.
func sfParseValues(text string) (map[string]string, error) {
	text = strings.TrimSpace(text)
	singleCount := strings.Count(text, "'")
	doubleCount := strings.Count(text, `"`)

	inSingle, inDouble := false, false
	var cur []rune
	var pairs []string
	for _, c := range text {
		isSep := c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
		if c == '\'' && !inDouble && singleCount >= 2 {
			inSingle = !inSingle
			continue
		} else if c == '"' && !inSingle && doubleCount >= 2 {
			inDouble = !inDouble
			continue
		}
		if !inSingle && !inDouble && isSep {
			if len(cur) > 0 {
				pairs = append(pairs, string(cur))
				cur = nil
			}
		} else {
			cur = append(cur, c)
		}
	}
	if len(cur) > 0 {
		pairs = append(pairs, string(cur))
	}

	out := map[string]string{}
	for _, p := range pairs {
		eq := strings.IndexByte(p, '=')
		if eq == -1 {
			return nil, fmt.Errorf("malformed key=value pair: %q", p)
		}
		out[p[:eq]] = p[eq+1:]
	}
	return out, nil
}
