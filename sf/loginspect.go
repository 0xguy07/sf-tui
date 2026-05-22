package sf

import (
	"strings"
)

// ApexEvent is one parsed entry from a debug log.
type ApexEvent struct {
	Time      string // raw HH:MM:SS.mmm
	Kind      string // event name from log (CODE_UNIT_STARTED, METHOD_ENTRY, USER_DEBUG, …)
	Detail    string // human-friendly summary, derived per kind
	Depth     int    // tree depth assigned by ParseApexEvents
	IsEnter   bool   // started/entry — pushes
	IsExit    bool   // finished/exit — pops
	IsHighlight bool // exception, USER_DEBUG, fatal
}

// ParseApexEvents reads raw log lines and returns a flat, depth-annotated list.
// Heuristic — it matches on event name suffixes (`_STARTED`, `_ENTRY`, `_FINISHED`, `_EXIT`)
// to decide nesting. Good enough for a structured demo view.
func ParseApexEvents(lines []string) []ApexEvent {
	events := make([]ApexEvent, 0, len(lines))
	depth := 0
	for _, line := range lines {
		// Lines look like:
		// 15:47:02.30 (30843789)|USER_DEBUG|[1]|DEBUG|hello world
		// 15:47:02.30 (30225153)|EXECUTION_STARTED
		t, rest, ok := splitTime(line)
		if !ok {
			continue
		}
		parts := strings.Split(rest, "|")
		if len(parts) == 0 {
			continue
		}
		kind := parts[0]
		detail := ""
		if len(parts) > 1 {
			detail = strings.Join(parts[1:], " · ")
		}

		ev := ApexEvent{Time: t, Kind: kind, Detail: detail}

		// Some kinds are exits — render at the parent depth, then decrement.
		switch {
		case strings.HasSuffix(kind, "_FINISHED"), strings.HasSuffix(kind, "_EXIT"):
			ev.IsExit = true
			if depth > 0 {
				depth--
			}
			ev.Depth = depth
		case strings.HasSuffix(kind, "_STARTED"), strings.HasSuffix(kind, "_ENTRY"):
			ev.IsEnter = true
			ev.Depth = depth
			depth++
		default:
			ev.Depth = depth
		}

		switch kind {
		case "USER_DEBUG", "EXCEPTION_THROWN", "FATAL_ERROR", "SYSTEM_MODE_ENTER":
			ev.IsHighlight = true
		}

		events = append(events, ev)
	}
	return events
}

// splitTime returns ("15:47:02.30", "USER_DEBUG|...|hello", true) for a log line, or
// ("", "", false) if it's a non-event line (separator, blank, summary block).
func splitTime(line string) (string, string, bool) {
	// Expect "HH:MM:SS.mmm (nanos)|<rest>"
	if len(line) < 13 || line[2] != ':' || line[5] != ':' {
		return "", "", false
	}
	bar := strings.IndexByte(line, '|')
	if bar < 0 {
		return "", "", false
	}
	left := line[:bar]
	// Strip "(...)" if present.
	if idx := strings.IndexByte(left, ' '); idx > 0 {
		left = left[:idx]
	}
	return left, line[bar+1:], true
}
