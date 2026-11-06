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
	nameW, usageW := 0, 0
	usages := make([]string, len(r.Limits))
	for i, l := range r.Limits {
		usages[i] = sf.HumanInt(l.Used) + " / " + sf.HumanInt(l.Max)
		if len(l.Name) > nameW {
			nameW = len(l.Name)
		}
		if len(usages[i]) > usageW {
			usageW = len(usages[i])
		}
	}
	for i, l := range r.Limits {
		fmt.Fprintf(&b, "  %-*s  %*s  %6.1f%%", nameW, l.Name, usageW, usages[i], l.Pct)
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
