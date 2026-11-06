package main

import (
	"fmt"
	"strings"

	"github.com/0xguy07/sf-tui/sf"
)

// renderTraceText renders a Trace as plain text with no ANSI escapes, for
// `sftui why --format text`. Timestamps are UTC so output is stable across
// machines.
func renderTraceText(t sf.Trace) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Why did %s change?\n", t.Record.ID)
	fmt.Fprintf(&b, "Object: %s (%s)\n", t.Object.Label, t.Object.Name)
	tracked := "none"
	if len(t.TrackedFields) > 0 {
		tracked = strings.Join(t.TrackedFields, ", ")
	}
	fmt.Fprintf(&b, "Tracked fields: %s\n\n", tracked)

	b.WriteString("Saves (newest first):\n\n")
	if len(t.Saves) == 0 {
		b.WriteString("  No tracked field changes found. Field history tracking may be off for this object.\n\n")
	}
	for _, s := range t.Saves {
		writeSave(&b, s)
	}
	if t.HistoryTruncated {
		b.WriteString("(history truncated: only the most recent rows were loaded)\n\n")
	}

	b.WriteString("Automation (order of execution):\n\n")
	for _, sec := range t.Automation {
		title := sec.Title
		if sec.Note != "" {
			title += " — " + sec.Note
		}
		fmt.Fprintln(&b, title)
		if len(sec.Items) == 0 {
			b.WriteString("  (none)\n")
		}
		for _, it := range sec.Items {
			fmt.Fprintf(&b, "  %s\n", automationLine(it))
		}
		b.WriteString("\n")
	}

	if len(t.Errors) > 0 {
		b.WriteString("Errors:\n")
		for _, e := range t.Errors {
			fmt.Fprintf(&b, "  %s: %s\n", e.Section, e.Message)
		}
	}
	return b.String()
}

func writeSave(b *strings.Builder, s sf.Save) {
	head := s.At.UTC().Format("2006-01-02 15:04:05 UTC")
	if u := s.User; u != nil {
		head += "  " + u.Name
		if u.Badge != "" {
			head += " [" + u.Badge + "]"
		}
		fmt.Fprintln(b, head)
		meta := []string{u.Username, "UserType: " + u.UserType}
		if u.Profile != "" {
			meta = append(meta, "Profile: "+u.Profile)
		}
		if u.License != "" {
			meta = append(meta, "License: "+u.License)
		}
		if !u.IsActive {
			meta = append(meta, "inactive")
		}
		fmt.Fprintf(b, "  %s\n", strings.Join(meta, " · "))
	} else {
		fmt.Fprintf(b, "%s  %s\n", head, s.CreatedByID)
	}
	if s.Hint != "" {
		fmt.Fprintf(b, "  hint: %s\n", s.Hint)
	}
	for _, c := range s.Changes {
		if c.Created {
			b.WriteString("  Record created\n")
			continue
		}
		fmt.Fprintf(b, "  %s: %s → %s\n", c.Field, plain(c.Old), plain(c.New))
		if len(c.Writers) == 0 {
			b.WriteString("    (no automation on this object writes this field)\n")
		}
		for _, w := range c.Writers {
			line := "    ↳ " + w.Name + " · " + w.Section
			if w.Note != "" {
				line += " — " + w.Note
			}
			fmt.Fprintln(b, line)
		}
	}
	b.WriteString("\n")
}

func automationLine(it sf.AutomationItem) string {
	line := it.Display()
	if it.Kind != sf.KindValidation && it.Kind != sf.KindAssignment {
		var ev strings.Builder
		for _, e := range []string{"C", "U", "D"} {
			on := false
			for _, x := range it.Events {
				on = on || x == e
			}
			if on {
				ev.WriteString("[" + e + "]")
			} else {
				ev.WriteString("[ ]")
			}
		}
		line = ev.String() + " " + line
	}
	var extra []string
	if it.TriggerOrder != nil {
		extra = append(extra, fmt.Sprintf("order %d", *it.TriggerOrder))
	}
	if !it.Active {
		extra = append(extra, "inactive")
	}
	if len(it.Writes) > 0 {
		fs := make([]string, 0, len(it.Writes))
		for _, fw := range it.Writes {
			fs = append(fs, fw.Field)
		}
		extra = append(extra, "writes "+strings.Join(fs, ", "))
	}
	if it.UpdatesRecordVariable {
		extra = append(extra, "updates a record variable")
	}
	if it.Detail != "" {
		extra = append(extra, it.Detail)
	}
	if len(extra) > 0 {
		line += " · " + strings.Join(extra, " · ")
	}
	return line
}

// plain formats a history value for text output.
func plain(v any) string {
	switch x := v.(type) {
	case nil:
		return "∅"
	case string:
		if x == "" {
			return "∅"
		}
		return strings.ReplaceAll(x, "\n", " ")
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	default:
		return strings.ReplaceAll(fmt.Sprintf("%v", x), "\n", " ")
	}
}
