package main

import (
	"fmt"
	"strings"

	"github.com/0xguy07/sf-tui/sf"
)

// renderLimitsText renders a LimitsReport as plain text, no ANSI.
func renderLimitsText(r sf.LimitsReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "API limits for %s  (%d limits)", r.Org, len(r.Limits))
	if r.Warn != nil {
		fmt.Fprintf(&b, "  warn at %d%%", *r.Warn)
	}
	b.WriteString("\n\n")
	breached := map[string]bool{}
	for _, n := range r.Breached {
		breached[n] = true
	}
	for _, l := range r.Limits {
		fmt.Fprintf(&b, "  %-40s  %s / %s   %.1f%%", l.Name, sf.HumanInt(l.Used), sf.HumanInt(l.Max), l.Pct)
		if breached[l.Name] {
			b.WriteString("  ⚠")
		}
		b.WriteString("\n")
	}
	if r.Warn != nil {
		fmt.Fprintf(&b, "\n%d of %d limits at or above %d%%\n", len(r.Breached), len(r.Limits), *r.Warn)
	}
	return b.String()
}
