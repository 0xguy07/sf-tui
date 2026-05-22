package sf

import "testing"

func TestAnalyze(t *testing.T) {
	cases := []struct {
		name       string
		text       string // cursor is always at end of this substring
		wantKind   CompletionKind
		wantPrefix string
		wantSObj   string
		wantChain  []string
	}{
		{"empty", "", KindNone, "", "", nil},
		{"select nothing yet", "SELECT ", KindNone, "", "", nil},
		{"select field prefix without from", "SELECT Id, Na", KindNone, "Na", "", nil},
		// Cursor right after partial field name, FROM already typed earlier:
		{"where field typing", "SELECT Id FROM Case WHERE Sub", KindField, "Sub", "Case", nil},
		{"order by field typing", "SELECT Id FROM Case ORDER BY Crea", KindField, "Crea", "Case", nil},
		{"group by field typing", "SELECT COUNT() FROM Opp GROUP BY Sta", KindField, "Sta", "Opp", nil},
		// Cursor after FROM typing sobject:
		{"from prefix", "SELECT Id FROM Acc", KindSObject, "Acc", "Acc", nil},
		// Dotted reference walks — cursor after the partial final segment:
		{"dotted ref one level", "SELECT Account.Na", KindField, "Na", "", []string{"Account"}},
		{"dotted ref two levels", "SELECT Account.Owner.Em", KindField, "Em", "", []string{"Account", "Owner"}},
		// Empty prefix mid-SELECT, no FROM yet → nothing useful to suggest:
		{"empty prefix after comma no from", "SELECT Id, ", KindNone, "", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Analyze(c.text, len(c.text))
			if got.Kind != c.wantKind {
				t.Errorf("Kind = %v, want %v", got.Kind, c.wantKind)
			}
			if got.Prefix != c.wantPrefix {
				t.Errorf("Prefix = %q, want %q", got.Prefix, c.wantPrefix)
			}
			if got.SObject != c.wantSObj {
				t.Errorf("SObject = %q, want %q", got.SObject, c.wantSObj)
			}
			if !sameSlice(got.RefChain, c.wantChain) {
				t.Errorf("RefChain = %v, want %v", got.RefChain, c.wantChain)
			}
		})
	}
}

func sameSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
