package panes

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Completion struct {
	Label  string // e.g. "Account"
	Detail string // e.g. "reference→Account,Contact"
	Insert string // actual text to insert (usually same as Label)
}

type Autocomplete struct {
	Items    []Completion
	Index    int
	Visible  bool
	MaxShown int
}

func NewAutocomplete() Autocomplete {
	return Autocomplete{MaxShown: 6}
}

// DisplayHeight returns the exact number of terminal rows the popup occupies
// when rendered, including its border. Returns 0 when hidden.
func (a Autocomplete) DisplayHeight() int {
	if !a.Visible || len(a.Items) == 0 {
		return 0
	}
	n := len(a.Items)
	if n > a.MaxShown {
		n = a.MaxShown
	}
	h := n + 2 // +2 for the rounded border (top + bottom)
	if len(a.Items) > n {
		h++ // "+X more" footer row
	}
	return h
}

func (a *Autocomplete) Set(items []Completion) {
	a.Items = items
	a.Index = 0
	a.Visible = len(items) > 0
}

func (a *Autocomplete) Hide() {
	a.Visible = false
	a.Items = nil
	a.Index = 0
}

func (a *Autocomplete) Move(delta int) {
	if len(a.Items) == 0 {
		return
	}
	a.Index = (a.Index + delta + len(a.Items)) % len(a.Items)
}

func (a *Autocomplete) Selected() *Completion {
	if !a.Visible || len(a.Items) == 0 {
		return nil
	}
	return &a.Items[a.Index]
}

func (a Autocomplete) View(maxWidth int) string {
	if !a.Visible || len(a.Items) == 0 {
		return ""
	}
	n := len(a.Items)
	if n > a.MaxShown {
		n = a.MaxShown
	}
	// Keep the selected index in the visible window.
	start := 0
	if a.Index >= n {
		start = a.Index - n + 1
	}
	end := start + n
	if end > len(a.Items) {
		end = len(a.Items)
		start = end - n
	}

	row := lipgloss.NewStyle().Padding(0, 1)
	activeRow := row.Background(lipgloss.Color("205")).Foreground(lipgloss.Color("15"))
	detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	var lines []string
	for i := start; i < end; i++ {
		it := a.Items[i]
		label := it.Label
		if len(label) > maxWidth-20 && maxWidth > 25 {
			label = label[:maxWidth-23] + "…"
		}
		text := label
		if it.Detail != "" {
			text = label + "  " + detailStyle.Render(it.Detail)
		}
		if i == a.Index {
			lines = append(lines, activeRow.Render(text))
		} else {
			lines = append(lines, row.Render(text))
		}
	}

	content := strings.Join(lines, "\n")
	if len(a.Items) > n {
		content += "\n" + lipgloss.NewStyle().Faint(true).Padding(0, 1).Render(
			" +"+itoa(len(a.Items)-n)+" more")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Render(content)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// Filter returns items whose label starts with prefix (case-insensitive), then
// contains prefix. Exported helper used by the main model to build suggestions.
func Filter(prefix string, all []Completion, limit int) []Completion {
	if prefix == "" {
		if len(all) <= limit {
			return all
		}
		out := make([]Completion, limit)
		copy(out, all[:limit])
		return out
	}
	p := strings.ToLower(prefix)
	var prefixHits, containsHits []Completion
	for _, it := range all {
		l := strings.ToLower(it.Label)
		if strings.HasPrefix(l, p) {
			prefixHits = append(prefixHits, it)
		} else if strings.Contains(l, p) {
			containsHits = append(containsHits, it)
		}
	}
	sort.SliceStable(prefixHits, func(i, j int) bool { return prefixHits[i].Label < prefixHits[j].Label })
	sort.SliceStable(containsHits, func(i, j int) bool { return containsHits[i].Label < containsHits[j].Label })
	out := append(prefixHits, containsHits...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
