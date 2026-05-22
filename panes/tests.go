package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/0xguy07/sf-tui/sf"
)

type testItem struct {
	c        sf.TestClass
	selected bool
}

func (i testItem) Title() string {
	if i.selected {
		return "✔ " + i.c.Name
	}
	return "  " + i.c.Name
}
func (i testItem) Description() string  { return i.c.ID }
func (i testItem) FilterValue() string  { return i.c.Name }

type Tests struct {
	List     list.Model
	Viewport viewport.Model
	width    int
	height   int
	classes  []sf.TestClass
	selected map[string]bool
	loaded   bool

	lastResult sf.TestRunResult
	hasResult  bool
}

func NewTests(width, height int) Tests {
	l := list.New(nil, list.NewDefaultDelegate(), width/2, height)
	l.Title = "Test Classes"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	v := viewport.New(width/2, height)
	v.SetContent(lipgloss.NewStyle().Faint(true).Render("Press space to select test classes, ctrl+r to run.\nNo selection = run all local tests."))
	return Tests{
		List:     l,
		Viewport: v,
		width:    width,
		height:   height,
		selected: map[string]bool{},
	}
}

func (t *Tests) SetSize(width, height int) {
	t.width = width
	t.height = height
	listW := width / 3
	if listW < 30 {
		listW = 30
	}
	detailW := width - listW - 4
	if detailW < 20 {
		detailW = 20
	}
	t.List.SetSize(listW, height)
	t.Viewport.Width = detailW
	t.Viewport.Height = height
}

func (t *Tests) SetClasses(classes []sf.TestClass) {
	t.classes = classes
	t.loaded = true
	t.refreshItems()
}

func (t Tests) Loaded() bool { return t.loaded }

func (t *Tests) refreshItems() {
	items := make([]list.Item, 0, len(t.classes))
	for _, c := range t.classes {
		items = append(items, testItem{c: c, selected: t.selected[c.ID]})
	}
	t.List.SetItems(items)
}

// ToggleSelected flips selection for the highlighted class.
func (t *Tests) ToggleSelected() {
	it, ok := t.List.SelectedItem().(testItem)
	if !ok {
		return
	}
	t.selected[it.c.ID] = !t.selected[it.c.ID]
	t.refreshItems()
}

func (t *Tests) ClearSelection() {
	t.selected = map[string]bool{}
	t.refreshItems()
}

// SelectedNames returns class names checked for the next run.
func (t Tests) SelectedNames() []string {
	out := []string{}
	for _, c := range t.classes {
		if t.selected[c.ID] {
			out = append(out, c.Name)
		}
	}
	return out
}

func (t *Tests) SetStatus(s string) {
	t.Viewport.SetContent(lipgloss.NewStyle().Faint(true).Render(s))
}

func (t *Tests) SetResult(r sf.TestRunResult) {
	t.lastResult = r
	t.hasResult = true
	t.Viewport.SetContent(renderTestResult(r))
	t.Viewport.GotoTop()
}

// MergeCoverageDetail attaches uncovered-line info onto the last result and
// re-renders. Called when CoverageDetailLoadedMsg arrives.
func (t *Tests) MergeCoverageDetail(uncovered map[string][]int) {
	if !t.hasResult {
		return
	}
	for i := range t.lastResult.Coverage {
		if lines, ok := uncovered[t.lastResult.Coverage[i].Name]; ok {
			t.lastResult.Coverage[i].Uncovered = lines
		}
	}
	t.Viewport.SetContent(renderTestResult(t.lastResult))
}

// CoverageClassNames returns the class names currently shown in the result, so
// the caller can ask for uncovered-line details. Returns nil when no result.
func (t Tests) CoverageClassNames() []string {
	if !t.hasResult {
		return nil
	}
	out := make([]string, 0, len(t.lastResult.Coverage))
	for _, c := range t.lastResult.Coverage {
		out = append(out, c.Name)
	}
	return out
}

var (
	testHeader = lipgloss.NewStyle().Bold(true)
	testPass   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	testFail   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	testDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

func renderTestResult(r sf.TestRunResult) string {
	var b strings.Builder
	if r.Fail == 0 && r.Pass > 0 {
		fmt.Fprintln(&b, testPass.Render(fmt.Sprintf("✓ %d/%d passed", r.Pass, r.Total))+testDim.Render("  ("+r.Time+")"))
	} else if r.Fail > 0 {
		fmt.Fprintln(&b, testFail.Render(fmt.Sprintf("✗ %d failed, %d passed", r.Fail, r.Pass))+testDim.Render("  ("+r.Time+")"))
	} else {
		fmt.Fprintln(&b, testDim.Render("(no tests ran)"))
	}
	fmt.Fprintln(&b)

	if len(r.Failures) > 0 {
		fmt.Fprintln(&b, testHeader.Render("Failures"))
		for _, f := range r.Failures {
			fmt.Fprintf(&b, "%s.%s\n", testFail.Render(f.Class), f.Method)
			if f.Message != "" {
				fmt.Fprintln(&b, "  "+f.Message)
			}
			if f.Stack != "" {
				fmt.Fprintln(&b, testDim.Render("  "+truncate(f.Stack, 120)))
			}
			fmt.Fprintln(&b)
		}
	}

	if len(r.Coverage) > 0 {
		fmt.Fprintln(&b, testHeader.Render("Coverage"))
		for _, c := range r.Coverage {
			pctStr := fmt.Sprintf("%3d%%", c.Percent)
			color := testPass
			if c.Percent < 75 {
				color = testFail
			}
			fmt.Fprintf(&b, "  %-40s %s  %s\n",
				truncate(c.Name, 40),
				color.Render(pctStr),
				testDim.Render(fmt.Sprintf("%d/%d lines", c.Covered, c.Total)),
			)
			if len(c.Uncovered) > 0 {
				fmt.Fprintln(&b, testDim.Render("      uncovered: "+compactRanges(c.Uncovered)))
			}
		}
	}
	return b.String()
}

// compactRanges turns a sorted slice of ints into a compact "1-3, 7, 10-12" string.
func compactRanges(xs []int) string {
	if len(xs) == 0 {
		return ""
	}
	var parts []string
	start, prev := xs[0], xs[0]
	flush := func() {
		if start == prev {
			parts = append(parts, fmt.Sprintf("%d", start))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", start, prev))
		}
	}
	for _, x := range xs[1:] {
		if x == prev+1 {
			prev = x
			continue
		}
		flush()
		start, prev = x, x
	}
	flush()
	return strings.Join(parts, ", ")
}

func (t Tests) UpdateList(msg tea.Msg) (Tests, tea.Cmd) {
	var cmd tea.Cmd
	t.List, cmd = t.List.Update(msg)
	return t, cmd
}

func (t Tests) UpdateViewport(msg tea.Msg) (Tests, tea.Cmd) {
	if vpJumpKeys(&t.Viewport, msg) {
		return t, nil
	}
	var cmd tea.Cmd
	t.Viewport, cmd = t.Viewport.Update(msg)
	return t, cmd
}

func (t Tests) View(listFocus, detailFocus bool) string {
	left := BorderFor(listFocus).Render(t.List.View())
	right := BorderFor(detailFocus).Render(t.Viewport.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}
