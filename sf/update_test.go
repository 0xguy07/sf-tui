package sf

import "testing"

func TestFormatUpdateValues(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]string
		want string
	}{
		{"simple", map[string]string{"Name": "Acme"}, "Name='Acme'"},
		{"multi sorted", map[string]string{"Phone": "555", "Name": "Acme"}, "Name='Acme' Phone='555'"},
		{"empty value clears", map[string]string{"Name": ""}, `Name=""`},
		{"single quote escaped", map[string]string{"Name": "O'Brien"}, `Name='O'\''Brien'`},
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
