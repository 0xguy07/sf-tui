package sf

import "testing"

func TestParseSelect(t *testing.T) {
	cases := []struct {
		name    string
		soql    string
		ok      bool
		cols    []string
		sobject string
	}{
		{"simple", "SELECT Id, Name FROM Account", true, []string{"Id", "Name"}, "Account"},
		{"dotted", "SELECT Id, Account.Name FROM Contact", true, []string{"Id", "Account.Name"}, "Contact"},
		{"two dots", "SELECT Account.Owner.Name FROM Contact", true, []string{"Account.Owner.Name"}, "Contact"},
		{"drops subqueries", "SELECT Id, (SELECT Id FROM Contacts) FROM Account", true, []string{"Id"}, "Account"},
		{"drops aggregates", "SELECT COUNT(Id) FROM Account", true, nil, "Account"},
		{"missing from", "SELECT Id, Name", false, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info, ok := ParseSelect(c.soql)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if info.SObject != c.sobject {
				t.Errorf("SObject = %q, want %q", info.SObject, c.sobject)
			}
			if !sameSlice(info.Columns, c.cols) {
				t.Errorf("Columns = %v, want %v", info.Columns, c.cols)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	rec := QueryRecord{
		"Id":   "001xx",
		"Name": "Acme",
		"Account": map[string]any{
			"Name": "Acme Parent",
			"Owner": map[string]any{
				"Email": "ceo@acme.test",
			},
		},
	}
	cases := []struct {
		path string
		want any
	}{
		{"Id", "001xx"},
		{"Account.Name", "Acme Parent"},
		{"Account.Owner.Email", "ceo@acme.test"},
		{"account.name", "Acme Parent"}, // case-insensitive fallback
		{"Account.Missing", nil},
		{"Nope", nil},
	}
	for _, c := range cases {
		got := ResolvePath(rec, c.path)
		if got != c.want {
			t.Errorf("ResolvePath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
