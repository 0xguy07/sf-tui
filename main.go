package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/0xguy07/sf-tui/panes"
	"github.com/0xguy07/sf-tui/sf"
)

type tab int

const (
	tabQuery tab = iota
	tabObjects
	tabLogs
	tabApex
	tabLimits
	tabPerms
	tabTests
	tabMeta
	tabCompare
)

type focus int

const (
	focusOrgs focus = iota
	focusMainA
	focusMainB
)

type model struct {
	orgs    panes.OrgList
	query   panes.Query
	results panes.Results
	objects panes.Objects
	logs    panes.Logs
	apex    panes.Apex
	limits  panes.Limits
	perms   panes.Permissions
	tests   panes.Tests
	meta    panes.Metadata
	compare panes.Compare
	picker  panes.Picker
	palette panes.Palette
	ac      panes.Autocomplete
	record  panes.Record

	saveInput   textinput.Model
	saving      bool
	exportInput textinput.Model
	exporting   bool
	flsInput    textinput.Model
	flsPrompt   bool
	flsForUser  *sf.UserBrief
	deployConfirm bool
	pickerOn    bool
	paletteOn   bool
	helpOn      bool

	store *sf.Store
	cache *sf.Cache
	tail  *sf.LogTail

	spinner spinner.Model
	loading bool
	status  string
	err     string

	tab    tab
	focus  focus
	width  int
	height int
	ready  bool

	objectsLoadedFor string
	limitsLoadedFor  string
	usersLoadedFor   string
	testsLoadedFor   string
	metaLoadedFor    string
	compareTargetOrg string
	useTooling       bool
	lastCtxSObject   string // most recent FROM target we auto-loaded a describe for
	lastQuerySObject string // FROM target of the last successful query (for record editor)
	pendingEditFor   string // sobject we're waiting on a describe for, to open the editor
}

var (
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	tabStyle     = lipgloss.NewStyle().Padding(0, 2)
	activeTab    = lipgloss.NewStyle().Padding(0, 2).Background(lipgloss.Color("205")).Foreground(lipgloss.Color("15")).Bold(true)
	overlayStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Padding(0, 1)
	orgBadge     = lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("63")).Foreground(lipgloss.Color("15")).Bold(true)
	orgBadgeNone = lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("240")).Foreground(lipgloss.Color("15"))
)

func initialModel() model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Placeholder = "name this query…"
	ti.CharLimit = 80
	ti.Width = 40

	ei := textinput.New()
	ei.Placeholder = "filename (defaults to timestamp)…"
	ei.CharLimit = 80
	ei.Width = 40

	fi := textinput.New()
	fi.Placeholder = "sobject API name (e.g. Account)…"
	fi.CharLimit = 80
	fi.Width = 40

	store, _ := sf.LoadStore()
	if store == nil {
		store = &sf.Store{}
	}

	return model{
		orgs:        panes.NewOrgList(30, 10),
		query:       panes.NewQuery(60, 6),
		results:     panes.NewResults(60, 10),
		objects:     panes.NewObjects(80, 20),
		logs:        panes.NewLogs(80, 20),
		apex:        panes.NewApex(80, 10),
		limits:      panes.NewLimits(80, 20),
		perms:       panes.NewPermissions(80, 20),
		tests:       panes.NewTests(80, 20),
		meta:        panes.NewMetadata(80, 20),
		compare:     panes.NewCompare(80, 20),
		picker:      panes.NewPicker(80, 20),
		palette:     panes.NewPalette(80, 20),
		ac:          panes.NewAutocomplete(),
		record:      panes.NewRecord(80, 20),
		saveInput:   ti,
		exportInput: ei,
		flsInput:    fi,
		store:       store,
		cache:       sf.NewCache(),
		spinner:     sp,
		focus:       focusOrgs,
		status:      "loading orgs…",
		loading:     true,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(sf.LoadOrgs(), m.spinner.Tick)
}

func (m *model) selectedOrg() string {
	o := m.orgs.Selected()
	if o == nil {
		return ""
	}
	if o.Alias != "" {
		return o.Alias
	}
	return o.Username
}

func (m *model) setFocus(f focus) {
	m.focus = f
	m.query.Blur()
	m.results.Blur()
	m.apex.Blur()
	m.objects.FocusList()
	if f == focusMainB && m.tab == tabObjects {
		m.objects.FocusDetail()
	}
	if f == focusMainA && m.tab == tabQuery {
		_ = m.query.Focus()
	}
	if f == focusMainB && m.tab == tabQuery {
		m.results.Focus()
	}
	if f == focusMainA && m.tab == tabApex {
		_ = m.apex.Focus()
	}
	if f != focusMainA || m.tab != tabQuery {
		m.ac.Hide()
	}
}

func (m *model) cycleFocus(forward bool) {
	order := []focus{focusOrgs, focusMainA, focusMainB}
	idx := 0
	for i, f := range order {
		if f == m.focus {
			idx = i
			break
		}
	}
	if forward {
		idx = (idx + 1) % len(order)
	} else {
		idx = (idx + len(order) - 1) % len(order)
	}
	m.setFocus(order[idx])
}

func (m *model) switchTab(t tab) {
	m.tab = t
	m.err = ""
	m.setFocus(focusMainA)
}

// refreshCompletions re-parses the query and refreshes the autocomplete list.
// May return a tea.Cmd to lazy-load sobjects or a describe.
func (m *model) refreshCompletions() tea.Cmd {
	if m.tab != tabQuery || m.focus != focusMainA {
		m.ac.Hide()
		return nil
	}
	text := strings.TrimSpace(m.query.Value())
	if text == "" {
		m.ac.Hide()
		return nil
	}
	ctx := sf.Analyze(m.query.Value(), len(m.query.Value()))

	var cmds []tea.Cmd
	org := m.selectedOrg()

	switch ctx.Kind {
	case sf.KindSObject:
		if org != "" && !m.cache.HasSObjects(org) {
			cmds = append(cmds, sf.LoadSObjects(org))
		}
		items := buildSObjectCompletions(m.cache.SObjects(org), ctx.Prefix)
		m.ac.Set(items)
	case sf.KindField:
		if ctx.SObject == "" {
			m.ac.Hide()
			break
		}
		// Walk reference chain: start from ctx.SObject, for each step find the
		// reference field with that name and hop to its ReferenceTo[0].
		target := ctx.SObject
		for _, step := range ctx.RefChain {
			d, ok := m.cache.Describe(org, target)
			if !ok {
				if _, _, cmd := m.cache.EnsureDescribe(org, target); cmd != nil {
					cmds = append(cmds, cmd)
				}
				target = ""
				break
			}
			target = resolveRefTarget(d, step)
			if target == "" {
				break
			}
		}
		if target == "" {
			m.ac.Hide()
			break
		}
		d, ok := m.cache.Describe(org, target)
		if !ok {
			if _, _, cmd := m.cache.EnsureDescribe(org, target); cmd != nil {
				cmds = append(cmds, cmd)
				m.lastCtxSObject = target
			}
			m.ac.Hide()
			break
		}
		items := buildFieldCompletions(d, ctx.Prefix)
		m.ac.Set(items)
	default:
		m.ac.Hide()
	}
	return tea.Batch(cmds...)
}

func resolveRefTarget(d sf.Describe, fieldName string) string {
	want := strings.ToLower(fieldName)
	// Relationship names often differ from field names (Account vs AccountId),
	// but we don't have relationshipName in our Field struct yet. Match on field
	// name or "<rel>Id" → "<rel>".
	for _, f := range d.Fields {
		if strings.ToLower(f.Name) == want && len(f.ReferenceTo) > 0 {
			return f.ReferenceTo[0]
		}
	}
	// Try matching by dropping trailing "Id" from field: user types "Owner" → field is "OwnerId".
	for _, f := range d.Fields {
		if len(f.ReferenceTo) == 0 {
			continue
		}
		base := strings.ToLower(f.Name)
		if strings.HasSuffix(base, "id") && strings.TrimSuffix(base, "id") == want {
			return f.ReferenceTo[0]
		}
	}
	return ""
}

func buildSObjectCompletions(ss []sf.SObjectSummary, prefix string) []panes.Completion {
	all := make([]panes.Completion, 0, len(ss))
	for _, s := range ss {
		detail := "std"
		if s.Custom {
			detail = "custom"
		}
		all = append(all, panes.Completion{Label: s.Name, Detail: detail, Insert: s.Name})
	}
	return panes.Filter(prefix, all, 50)
}

func buildFieldCompletions(d sf.Describe, prefix string) []panes.Completion {
	all := make([]panes.Completion, 0, len(d.Fields))
	for _, f := range d.Fields {
		detail := f.Type
		if f.Type == "reference" && len(f.ReferenceTo) > 0 {
			detail = "→" + f.ReferenceTo[0]
		}
		all = append(all, panes.Completion{Label: f.Name, Detail: detail, Insert: f.Name})
	}
	return panes.Filter(prefix, all, 50)
}

func (m *model) acceptCompletion() {
	sel := m.ac.Selected()
	if sel == nil {
		return
	}
	m.query.ReplaceTailToken(sel.Insert)
	m.ac.Hide()
	m.status = "inserted " + sel.Label
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Record editor modal — captures all keystrokes while open.
	if m.record.Visible {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				if m.record.ValueFocused() {
					m.record.FocusList()
					return m, nil
				}
				if m.record.Dirty() {
					m.err = "discard changes? press ctrl+w to confirm, esc to keep editing"
					return m, nil
				}
				m.record.Close()
				return m, nil
			case "ctrl+w":
				m.record.Close()
				m.err = ""
				m.status = "edit cancelled"
				return m, nil
			case "tab":
				if m.record.ValueFocused() {
					m.record.FocusList()
				} else {
					m.record.FocusValue()
				}
				return m, nil
			case "enter":
				if !m.record.ValueFocused() {
					m.record.FocusValue()
					return m, nil
				}
			case "ctrl+s":
				if !m.record.Dirty() {
					m.status = "no changes to save"
					return m, nil
				}
				m.loading = true
				m.status = "saving record…"
				return m, tea.Batch(
					sf.UpdateRecord(m.selectedOrg(), m.record.SObject, m.record.RecordID, m.record.Updates()),
					m.spinner.Tick,
				)
			}
		}
		var cmd tea.Cmd
		m.record, cmd = m.record.Update(msg)
		return m, cmd
	}

	// Save-query input mode swallows keystrokes.
	if m.saving {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				m.saving = false
				m.saveInput.Blur()
				m.saveInput.SetValue("")
				return m, nil
			case "enter":
				name := strings.TrimSpace(m.saveInput.Value())
				if name != "" {
					m.store.SaveNamed(name, m.selectedOrg(), m.query.Value())
					if err := m.store.Save(); err != nil {
						m.err = "save failed: " + err.Error()
					} else {
						m.status = "saved as " + name
					}
				}
				m.saving = false
				m.saveInput.Blur()
				m.saveInput.SetValue("")
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.saveInput, cmd = m.saveInput.Update(msg)
		return m, cmd
	}

	// Export input mode.
	if m.exporting {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				m.exporting = false
				m.exportInput.Blur()
				m.exportInput.SetValue("")
				return m, nil
			case "enter":
				path, err := sf.ExportSOQL(strings.TrimSpace(m.exportInput.Value()), m.query.Value())
				if err != nil {
					m.err = err.Error()
				} else {
					m.status = "exported → " + path
				}
				m.exporting = false
				m.exportInput.Blur()
				m.exportInput.SetValue("")
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.exportInput, cmd = m.exportInput.Update(msg)
		return m, cmd
	}

	// FLS prompt input mode.
	if m.flsPrompt {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				m.flsPrompt = false
				m.flsInput.Blur()
				m.flsInput.SetValue("")
				m.flsForUser = nil
				return m, nil
			case "enter":
				name := strings.TrimSpace(m.flsInput.Value())
				m.flsPrompt = false
				m.flsInput.Blur()
				m.flsInput.SetValue("")
				if name == "" || m.flsForUser == nil {
					m.flsForUser = nil
					return m, nil
				}
				user := *m.flsForUser
				m.flsForUser = nil
				m.loading = true
				m.status = "loading FLS for " + name + "…"
				return m, tea.Batch(sf.LoadFieldPermissions(m.selectedOrg(), user, name), m.spinner.Tick)
			}
		}
		var cmd tea.Cmd
		m.flsInput, cmd = m.flsInput.Update(msg)
		return m, cmd
	}

	// Deploy confirmation modal — y/n only.
	if m.deployConfirm {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "y", "Y":
				m.deployConfirm = false
				org := m.selectedOrg()
				if org == "" || !m.meta.Loaded() {
					m.err = "load a deploy preview first"
					return m, nil
				}
				m.loading = true
				m.status = "deploying to " + org + "…"
				m.meta.SetStatus("deploying to " + org + "… (real deploy, not dry-run)")
				return m, tea.Batch(sf.RunDeploy(org, m.meta.ProjectDir()), m.spinner.Tick)
			case "n", "N", "esc":
				m.deployConfirm = false
				m.status = "deploy cancelled"
				return m, nil
			}
		}
		return m, nil
	}

	// Help overlay.
	if m.helpOn {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc", "?", "q":
				m.helpOn = false
				return m, nil
			}
		}
		return m, nil
	}

	// Palette overlay mode.
	if m.paletteOn {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc", "ctrl+k":
				m.paletteOn = false
				return m, nil
			case "enter":
				if sel := m.palette.Selected(); sel != nil {
					m.paletteOn = false
					return m.dispatchCommand(sel.ID)
				}
				m.paletteOn = false
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.palette, cmd = m.palette.Update(msg)
		return m, cmd
	}

	// Picker overlay mode.
	if m.pickerOn {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc", "ctrl+p":
				m.pickerOn = false
				return m, nil
			case "enter":
				if sel := m.picker.Selected(); sel != nil {
					m.query.SetValue(sel.SOQL)
					m.status = "loaded query"
					m.tab = tabQuery
					m.setFocus(focusMainA)
					cmd := m.refreshCompletions()
					return m, cmd
				}
				m.pickerOn = false
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		m.ready = true

	case sf.OrgsLoadedMsg:
		m.orgs.SetOrgs(msg.Orgs)
		m.loading = false
		m.status = fmt.Sprintf("%d orgs loaded — alt+1/2/3 switch tabs, tab switches panes, q quits", len(msg.Orgs))

	case sf.QueryDoneMsg:
		m.results.SetRows(msg.Columns, msg.Rows, msg.Total)
		m.lastQuerySObject = msg.SObject
		m.loading = false
		m.status = fmt.Sprintf("%d rows", msg.Total)
		m.store.AddHistory(m.selectedOrg(), m.query.Value())
		_ = m.store.Save()
		m.setFocus(focusMainB)

	case sf.SObjectsLoadedMsg:
		org := m.selectedOrg()
		m.cache.SetSObjects(org, msg.SObjects)
		m.objects.SetSObjects(msg.SObjects)
		m.objectsLoadedFor = org
		m.loading = false
		m.status = fmt.Sprintf("%d sobjects cached", len(msg.SObjects))
		if cmd := m.refreshCompletions(); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case sf.DescribeLoadedMsg:
		org := m.selectedOrg()
		m.cache.SetDescribe(org, msg.Describe)
		m.cache.ReleaseDescribe(org, msg.Describe.Name)
		m.objects.SetDescribe(msg.Describe)
		m.loading = false
		m.status = fmt.Sprintf("%s: %d fields", msg.Describe.Name, len(msg.Describe.Fields))
		if m.pendingEditFor != "" && strings.EqualFold(m.pendingEditFor, msg.Describe.Name) {
			if rec := m.results.SelectedRecord(); rec != nil {
				m.record.Open(msg.Describe.Name, msg.Describe, rec)
				m.status = "editing record"
			}
			m.pendingEditFor = ""
		}
		if cmd := m.refreshCompletions(); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case sf.LogLineMsg:
		m.logs.Append(msg.Line)
		if m.tail != nil {
			return m, m.tail.Next()
		}

	case sf.LogEndedMsg:
		m.logs.Append("── log tail ended ──")
		m.logs.Running = false
		m.tail = nil
		if msg.Err != nil {
			m.err = "log tail: " + msg.Err.Error()
		}

	case sf.ApexDoneMsg:
		m.loading = false
		m.apex.Running = false
		m.apex.SetOutput(panes.FormatApexResult(
			msg.Success, msg.Compiled, msg.CompileProblem,
			msg.ExceptionMsg, msg.ExceptionStack, msg.Logs,
		))
		switch {
		case !msg.Compiled:
			m.status = "compile failed"
		case !msg.Success:
			m.status = "runtime error"
		default:
			m.status = "apex executed"
		}

	case sf.LimitsLoadedMsg:
		m.limits.SetLimits(msg.Limits)
		m.loading = false
		m.status = fmt.Sprintf("%d limits loaded", len(msg.Limits))

	case sf.UsersLoadedMsg:
		m.perms.SetUsers(msg.Users)
		m.loading = false
		m.status = fmt.Sprintf("%d users — pick one and press enter", len(msg.Users))

	case sf.PermissionsLoadedMsg:
		m.perms.SetDetail(msg)
		m.loading = false
		m.status = fmt.Sprintf("%s: %d perm sets, %d objects", msg.User.Name, len(msg.Assignments), len(msg.Objects))

	case sf.FieldPermissionsLoadedMsg:
		m.perms.SetFieldPerms(msg)
		m.loading = false
		m.status = fmt.Sprintf("%s on %s: %d field perms", msg.User.Name, msg.SObject, len(msg.Fields))

	case sf.TestClassesLoadedMsg:
		m.tests.SetClasses(msg.Classes)
		m.loading = false
		m.status = fmt.Sprintf("%d test classes — space selects, ctrl+r runs", len(msg.Classes))

	case sf.TestRunDoneMsg:
		m.tests.SetResult(msg.Result)
		m.loading = false
		if names := m.tests.CoverageClassNames(); len(names) > 0 {
			cmds = append(cmds, sf.LoadCoverageDetail(m.selectedOrg(), names))
		}
		if msg.Result.Fail > 0 {
			m.status = fmt.Sprintf("✗ %d failed, %d passed", msg.Result.Fail, msg.Result.Pass)
		} else {
			m.status = fmt.Sprintf("✓ %d/%d passed", msg.Result.Pass, msg.Result.Total)
		}

	case sf.DeployPreviewLoadedMsg:
		m.meta.SetPreview(msg)
		m.metaLoadedFor = m.selectedOrg()
		m.loading = false
		if msg.RawError != "" {
			m.status = "deploy preview failed (see detail pane)"
		} else {
			m.status = fmt.Sprintf("%d items in deploy plan", len(msg.Items))
		}

	case sf.DeployRunDoneMsg:
		m.meta.SetDryRunResult(msg)
		m.loading = false
		label := "deploy"
		if msg.DryRun {
			label = "dry-run"
		}
		if msg.Err != "" {
			m.status = label + " reported errors (see output)"
		} else {
			m.status = label + " complete"
		}

	case sf.SchemaLoadedMsg:
		m.compare.SetSchema(msg.Side, msg.Org, msg.SObjects)
		m.loading = false
		m.status = fmt.Sprintf("loaded %d sobjects from %s", len(msg.SObjects), msg.Org)

	case sf.FieldDiffLoadedMsg:
		m.compare.SetFieldDiff(msg)
		m.loading = false
		switch {
		case msg.AErr != "" && msg.BErr != "":
			m.status = "field diff failed on both sides"
		case msg.AErr != "":
			m.status = "field diff: A side failed (" + msg.AErr + ")"
		case msg.BErr != "":
			m.status = "field diff: B side failed (" + msg.BErr + ")"
		default:
			m.status = "field diff: " + msg.SObject
		}

	case sf.CoverageDetailLoadedMsg:
		m.tests.MergeCoverageDetail(msg.Uncovered)
		// Status untouched — this is a quiet enrichment after the headline result.

	case sf.OpenedMsg:
		m.loading = false
		m.status = "opened " + msg.What

	case sf.RecordUpdatedMsg:
		m.loading = false
		m.record.Apply()
		// Patch in-memory record + re-render row so the user sees the new values.
		if rec := m.results.SelectedRecord(); rec != nil {
			for k, v := range msg.Updates {
				rec[k] = v
			}
			m.results.SetRows(m.results.Cols, m.results.Records, m.results.Total)
		}
		m.record.Close()
		m.status = fmt.Sprintf("saved %s", msg.ID)

	case sf.ErrMsg:
		m.loading = false
		m.err = msg.Error()

	case tea.KeyMsg:
		// Autocomplete keystroke interception (Query tab, focused, popup visible).
		if m.tab == tabQuery && m.focus == focusMainA && m.ac.Visible {
			switch msg.String() {
			case "esc":
				m.ac.Hide()
				return m, nil
			case "up", "ctrl+p":
				m.ac.Move(-1)
				return m, nil
			case "down", "ctrl+n":
				m.ac.Move(1)
				return m, nil
			case "tab", "enter":
				m.acceptCompletion()
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+c":
			m.stopTail()
			return m, tea.Quit
		case "q":
			if m.focus != focusMainA || m.tab != tabQuery {
				m.stopTail()
				return m, tea.Quit
			}
		case "alt+1":
			m.switchTab(tabQuery)
			if cmd := m.refreshCompletions(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		case "alt+2":
			m.switchTab(tabObjects)
			org := m.selectedOrg()
			if org != "" && m.objectsLoadedFor != org {
				m.loading = true
				m.status = "loading sobjects…"
				return m, tea.Batch(sf.LoadSObjects(org), m.spinner.Tick)
			}
			return m, nil
		case "alt+3":
			m.switchTab(tabLogs)
			return m, nil
		case "alt+4":
			m.switchTab(tabApex)
			return m, nil
		case "alt+5":
			m.switchTab(tabLimits)
			org := m.selectedOrg()
			if org != "" && m.limitsLoadedFor != org {
				m.loading = true
				m.status = "loading limits…"
				m.limitsLoadedFor = org
				return m, tea.Batch(sf.LoadLimits(org), m.spinner.Tick)
			}
			return m, nil
		case "alt+6":
			m.switchTab(tabPerms)
			org := m.selectedOrg()
			if org != "" && m.usersLoadedFor != org {
				m.loading = true
				m.status = "loading users…"
				m.usersLoadedFor = org
				return m, tea.Batch(sf.LoadUsers(org), m.spinner.Tick)
			}
			return m, nil
		case "alt+7":
			m.switchTab(tabTests)
			org := m.selectedOrg()
			if org != "" && m.testsLoadedFor != org {
				m.loading = true
				m.status = "loading test classes (Tooling API)…"
				m.testsLoadedFor = org
				return m, tea.Batch(sf.LoadTestClasses(org), m.spinner.Tick)
			}
			return m, nil
		case "alt+8":
			m.switchTab(tabMeta)
			return m, nil
		case "alt+9":
			m.switchTab(tabCompare)
			return m, nil
		case "tab":
			m.cycleFocus(true)
			return m, nil
		case "shift+tab":
			m.cycleFocus(false)
			return m, nil
		case "ctrl+p":
			// Only the "open picker" when autocomplete isn't visible, which was
			// handled above. Here popup is hidden.
			m.picker.SetEntries(m.store.Entries())
			m.pickerOn = true
			return m, nil
		case "ctrl+k":
			m.palette.SetCommands(m.paletteCommands())
			m.paletteOn = true
			return m, nil
		case "?":
			// Block when typing into a textarea (Query / Apex editor) — `?` is a
			// valid character there. Allowed everywhere else, including filter
			// inputs (you can press esc first).
			if !((m.tab == tabQuery && m.focus == focusMainA) ||
				(m.tab == tabApex && m.focus == focusMainA)) {
				m.helpOn = true
				return m, nil
			}
		case "ctrl+t":
			if m.tab == tabQuery {
				m.useTooling = !m.useTooling
				if m.useTooling {
					m.status = "Tooling API: ON (next query)"
				} else {
					m.status = "Tooling API: off"
				}
				return m, nil
			}
		case "ctrl+r":
			if m.tab == tabQuery {
				return m.runQuery()
			}
			if m.tab == tabApex {
				return m.runApex()
			}
			if m.tab == tabLimits {
				org := m.selectedOrg()
				if org == "" {
					m.err = "select an org first"
					return m, nil
				}
				m.loading = true
				m.status = "refreshing limits…"
				m.limitsLoadedFor = org
				return m, tea.Batch(sf.LoadLimits(org), m.spinner.Tick)
			}
			if m.tab == tabTests {
				return m.runTests()
			}
			if m.tab == tabMeta {
				return m.runMeta()
			}
			if m.tab == tabCompare {
				return m.runCompare()
			}
		case "ctrl+s":
			if m.tab == tabQuery && strings.TrimSpace(m.query.Value()) != "" {
				m.saving = true
				_ = m.saveInput.Focus()
				return m, nil
			}
		case "ctrl+e":
			if m.tab == tabQuery && strings.TrimSpace(m.query.Value()) != "" {
				m.exporting = true
				_ = m.exportInput.Focus()
				return m, nil
			}
		case "ctrl+y":
			if m.tab == tabQuery && len(m.results.Records) > 0 {
				tsv := sf.FormatTSV(m.results.Cols, m.results.Records)
				if err := clipboard.WriteAll(tsv); err != nil {
					m.err = "clipboard: " + err.Error()
				} else {
					m.status = fmt.Sprintf("copied %d rows as TSV", len(m.results.Records))
				}
				return m, nil
			}
		case "ctrl+x":
			if m.tab == tabQuery && len(m.results.Records) > 0 {
				path, err := sf.ExportCSV("", m.results.Cols, m.results.Records)
				if err != nil {
					m.err = err.Error()
				} else {
					m.status = "wrote CSV → " + path
				}
				return m, nil
			}
		case "ctrl+space":
			if m.tab == tabQuery && m.focus == focusMainA {
				if cmd := m.refreshCompletions(); cmd != nil {
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
		case "ctrl+l":
			if m.tab == tabLogs {
				return m.toggleLogTail()
			}
		case " ":
			if m.tab == tabTests && m.focus == focusMainA && m.tests.List.FilterState() != list.Filtering {
				m.tests.ToggleSelected()
				return m, nil
			}
		case "f":
			if m.tab == tabPerms && m.perms.List.FilterState() != list.Filtering {
				u := m.perms.SelectedDetailUser()
				if u == nil {
					m.err = "load a user first (enter on the user list)"
					return m, nil
				}
				m.flsForUser = u
				m.flsPrompt = true
				_ = m.flsInput.Focus()
				return m, nil
			}
		case "c":
			if m.tab == tabCompare && m.compare.List.FilterState() != list.Filtering {
				m.compare.Clear()
				m.status = "compare cleared — press ctrl+r to load A side"
				return m, nil
			}
		case "esc":
			if m.tab == tabCompare && m.compare.InFieldView() &&
				m.compare.List.FilterState() != list.Filtering {
				m.compare.ShowSummary()
				m.status = "back to schema summary"
				return m, nil
			}
		case "ctrl+d":
			if m.tab == tabMeta {
				return m.confirmDeploy()
			}
		case "ctrl+i":
			if m.tab == tabLogs {
				m.logs.ToggleInspector()
				if m.logs.Inspector {
					m.status = "log inspector: ON"
				} else {
					m.status = "log inspector: off (raw)"
				}
				return m, nil
			}
		case "ctrl+o":
			return m.openInOrg()
		case "enter":
			if m.tab == tabObjects && m.focus == focusMainA {
				name := m.objects.SelectedName()
				if name != "" {
					org := m.selectedOrg()
					if d, ok := m.cache.Describe(org, name); ok {
						m.objects.SetDescribe(d)
						m.status = fmt.Sprintf("%s: %d fields", d.Name, len(d.Fields))
						return m, nil
					}
					m.loading = true
					m.status = "describing " + name + "…"
					return m, tea.Batch(sf.LoadDescribe(org, name), m.spinner.Tick)
				}
			}
			if m.tab == tabQuery && m.focus == focusMainB {
				return m.openRecordEditor()
			}
			if m.tab == tabPerms && m.focus == focusMainA {
				if u := m.perms.Selected(); u != nil {
					m.loading = true
					m.status = "loading permissions for " + u.Name + "…"
					m.perms.SetStatus("loading permissions for " + u.Name + "…")
					return m, tea.Batch(sf.LoadPermissions(m.selectedOrg(), *u), m.spinner.Tick)
				}
			}
			if m.tab == tabCompare && m.focus == focusMainA {
				return m.openFieldDiff()
			}
		}

		// org change invalidates cached-for-current-tab markers
		if m.focus == focusOrgs {
			prev := m.selectedOrg()
			var cmd tea.Cmd
			m.orgs, cmd = m.orgs.Update(msg)
			cmds = append(cmds, cmd)
			if newOrg := m.selectedOrg(); newOrg != prev {
				m.objectsLoadedFor = ""
				m.limitsLoadedFor = ""
				m.usersLoadedFor = ""
				m.testsLoadedFor = ""
				m.metaLoadedFor = ""
				m.limits.Clear()
				m.ac.Hide()
			}
			if m.loading {
				var sc tea.Cmd
				m.spinner, sc = m.spinner.Update(msg)
				cmds = append(cmds, sc)
			}
			return m, tea.Batch(cmds...)
		}
	}

	var cmd tea.Cmd
	if m.loading {
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	switch m.focus {
	case focusOrgs:
		m.orgs, cmd = m.orgs.Update(msg)
	case focusMainA, focusMainB:
		switch m.tab {
		case tabQuery:
			if m.focus == focusMainA {
				prevText := m.query.Value()
				m.query, cmd = m.query.Update(msg)
				if m.query.Value() != prevText {
					// Text changed; refresh completions.
					if acCmd := m.refreshCompletions(); acCmd != nil {
						cmds = append(cmds, acCmd)
					}
				}
			} else {
				m.results, cmd = m.results.Update(msg)
			}
		case tabObjects:
			if m.focus == focusMainA {
				m.objects.FocusList()
			} else {
				m.objects.FocusDetail()
			}
			m.objects, cmd = m.objects.Update(msg)
		case tabLogs:
			m.logs, cmd = m.logs.Update(msg)
		case tabApex:
			if m.focus == focusMainA {
				m.apex, cmd = m.apex.UpdateEditor(msg)
			} else {
				m.apex, cmd = m.apex.UpdateViewport(msg)
			}
		case tabLimits:
			m.limits, cmd = m.limits.Update(msg)
		case tabPerms:
			if m.focus == focusMainA {
				m.perms, cmd = m.perms.UpdateList(msg)
			} else {
				m.perms, cmd = m.perms.UpdateViewport(msg)
			}
		case tabTests:
			if m.focus == focusMainA {
				m.tests, cmd = m.tests.UpdateList(msg)
			} else {
				m.tests, cmd = m.tests.UpdateViewport(msg)
			}
		case tabMeta:
			if m.focus == focusMainA {
				m.meta, cmd = m.meta.UpdateList(msg)
			} else {
				m.meta, cmd = m.meta.UpdateViewport(msg)
			}
		case tabCompare:
			if m.focus == focusMainA {
				m.compare, cmd = m.compare.UpdateList(msg)
			} else {
				m.compare, cmd = m.compare.UpdateViewport(msg)
			}
		}
	}
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) openRecordEditor() (tea.Model, tea.Cmd) {
	if m.lastQuerySObject == "" {
		m.err = "can't determine sobject from query — try `SELECT … FROM <Object>`"
		return m, nil
	}
	rec := m.results.SelectedRecord()
	if rec == nil {
		m.err = "no row selected"
		return m, nil
	}
	if id := stringFromAny(sf.ResolvePath(rec, "Id")); id == "" {
		m.err = "row has no Id field — include Id in your SELECT to edit"
		return m, nil
	}
	org := m.selectedOrg()
	if d, ok := m.cache.Describe(org, m.lastQuerySObject); ok {
		m.record.Open(m.lastQuerySObject, d, rec)
		m.status = "editing record"
		return m, nil
	}
	// Need to fetch describe first; remember to open when it arrives.
	m.pendingEditFor = m.lastQuerySObject
	m.loading = true
	m.status = "fetching " + m.lastQuerySObject + " describe…"
	return m, tea.Batch(sf.LoadDescribe(org, m.lastQuerySObject), m.spinner.Tick)
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func (m model) paletteCommands() []panes.Command {
	return []panes.Command{
		{ID: "tab.query", Label: "Go to Query", Hint: "alt+1", Help: "Write SOQL and run it against the selected org"},
		{ID: "tab.objects", Label: "Go to Objects", Hint: "alt+2", Help: "Browse SObjects and inspect fields"},
		{ID: "tab.logs", Label: "Go to Logs", Hint: "alt+3", Help: "Live-tail Apex debug logs"},
		{ID: "tab.apex", Label: "Go to Apex", Hint: "alt+4", Help: "Anonymous Apex scratchpad"},
		{ID: "tab.limits", Label: "Go to Limits", Hint: "alt+5", Help: "View API limits with usage bars"},
		{ID: "tab.perms", Label: "Go to Permissions", Hint: "alt+6", Help: "Permission Explorer — effective object perms by user"},
		{ID: "tab.tests", Label: "Go to Tests", Hint: "alt+7", Help: "Run Apex tests with coverage"},
		{ID: "tab.meta", Label: "Go to Metadata", Hint: "alt+8", Help: "Deploy preview + dry-run for the current project"},
		{ID: "tab.compare", Label: "Go to Compare", Hint: "alt+9", Help: "Schema diff between two orgs"},
		{ID: "action.run", Label: "Run / Execute (current tab)", Hint: "ctrl+r", Help: "Run query / execute Apex / refresh limits"},
		{ID: "action.deploy", Label: "Deploy (real, Meta tab)", Hint: "ctrl+d", Help: "Run a real (non-dry-run) project deploy after a y/n confirmation"},
		{ID: "action.open", Label: "Open in Org", Hint: "ctrl+o", Help: "Open selected record or sobject in the browser"},
		{ID: "action.history", Label: "Saved queries + history", Hint: "ctrl+p", Help: "Pick a previous or saved query"},
		{ID: "action.tail", Label: "Toggle log tail", Hint: "ctrl+l", Help: "Start or stop tailing Apex logs"},
		{ID: "action.inspector", Label: "Toggle log inspector view", Hint: "ctrl+i", Help: "Switch between raw and structured Apex log view"},
		{ID: "action.copy", Label: "Copy results as TSV", Hint: "ctrl+y", Help: "Copy query results to the clipboard"},
		{ID: "action.csv", Label: "Export results as CSV", Hint: "ctrl+x", Help: "Write query results to ~/sf-tui-queries/*.csv"},
		{ID: "action.savequery", Label: "Save current query", Hint: "ctrl+s", Help: "Name and save the current SOQL"},
		{ID: "action.help", Label: "Show keybindings", Hint: "?", Help: "Open the help overlay listing every keybinding"},
	}
}

func (m model) dispatchCommand(id string) (tea.Model, tea.Cmd) {
	switch id {
	case "tab.query":
		m.switchTab(tabQuery)
		return m, nil
	case "tab.objects":
		m.switchTab(tabObjects)
		org := m.selectedOrg()
		if org != "" && m.objectsLoadedFor != org {
			m.loading = true
			m.status = "loading sobjects…"
			return m, tea.Batch(sf.LoadSObjects(org), m.spinner.Tick)
		}
		return m, nil
	case "tab.logs":
		m.switchTab(tabLogs)
		return m, nil
	case "tab.apex":
		m.switchTab(tabApex)
		return m, nil
	case "tab.limits":
		m.switchTab(tabLimits)
		org := m.selectedOrg()
		if org != "" && m.limitsLoadedFor != org {
			m.loading = true
			m.status = "loading limits…"
			m.limitsLoadedFor = org
			return m, tea.Batch(sf.LoadLimits(org), m.spinner.Tick)
		}
		return m, nil
	case "tab.perms":
		m.switchTab(tabPerms)
		org := m.selectedOrg()
		if org != "" && m.usersLoadedFor != org {
			m.loading = true
			m.status = "loading users…"
			m.usersLoadedFor = org
			return m, tea.Batch(sf.LoadUsers(org), m.spinner.Tick)
		}
		return m, nil
	case "tab.tests":
		m.switchTab(tabTests)
		org := m.selectedOrg()
		if org != "" && m.testsLoadedFor != org {
			m.loading = true
			m.status = "loading test classes…"
			m.testsLoadedFor = org
			return m, tea.Batch(sf.LoadTestClasses(org), m.spinner.Tick)
		}
		return m, nil
	case "tab.meta":
		m.switchTab(tabMeta)
		return m, nil
	case "tab.compare":
		m.switchTab(tabCompare)
		return m, nil
	case "action.run":
		switch m.tab {
		case tabQuery:
			return m.runQuery()
		case tabApex:
			return m.runApex()
		case tabTests:
			return m.runTests()
		case tabMeta:
			return m.runMeta()
		case tabCompare:
			return m.runCompare()
		case tabLimits:
			org := m.selectedOrg()
			if org == "" {
				m.err = "select an org first"
				return m, nil
			}
			m.loading = true
			m.status = "refreshing limits…"
			m.limitsLoadedFor = org
			return m, tea.Batch(sf.LoadLimits(org), m.spinner.Tick)
		}
		return m, nil
	case "action.deploy":
		m.tab = tabMeta
		m.setFocus(focusMainA)
		return m.confirmDeploy()
	case "action.open":
		return m.openInOrg()
	case "action.history":
		m.picker.SetEntries(m.store.Entries())
		m.pickerOn = true
		return m, nil
	case "action.tail":
		return m.toggleLogTail()
	case "action.inspector":
		m.tab = tabLogs
		m.logs.ToggleInspector()
		if m.logs.Inspector {
			m.status = "log inspector: ON"
		} else {
			m.status = "log inspector: off (raw)"
		}
		return m, nil
	case "action.copy":
		if len(m.results.Records) == 0 {
			m.err = "no results to copy"
			return m, nil
		}
		tsv := sf.FormatTSV(m.results.Cols, m.results.Records)
		if err := clipboard.WriteAll(tsv); err != nil {
			m.err = "clipboard: " + err.Error()
		} else {
			m.status = fmt.Sprintf("copied %d rows as TSV", len(m.results.Records))
		}
		return m, nil
	case "action.csv":
		if len(m.results.Records) == 0 {
			m.err = "no results to export"
			return m, nil
		}
		path, err := sf.ExportCSV("", m.results.Cols, m.results.Records)
		if err != nil {
			m.err = err.Error()
		} else {
			m.status = "wrote CSV → " + path
		}
		return m, nil
	case "action.help":
		m.helpOn = true
		return m, nil
	case "action.savequery":
		if strings.TrimSpace(m.query.Value()) == "" {
			m.err = "query is empty"
			return m, nil
		}
		m.tab = tabQuery
		m.setFocus(focusMainA)
		m.saving = true
		_ = m.saveInput.Focus()
		return m, nil
	}
	return m, nil
}

func (m model) openInOrg() (tea.Model, tea.Cmd) {
	org := m.selectedOrg()
	if org == "" {
		m.err = "select an org first"
		return m, nil
	}

	var path, what string
	switch m.tab {
	case tabQuery:
		if rec := m.results.SelectedRecord(); rec != nil {
			if id := stringFromAny(sf.ResolvePath(rec, "Id")); id != "" {
				path = "/" + id
				what = id
			}
		}
	case tabObjects:
		if name := m.objects.SelectedName(); name != "" {
			path = "/lightning/setup/ObjectManager/" + name + "/Details/view"
			what = name + " in Object Manager"
		}
	}
	if path == "" {
		path = "/lightning/setup/SetupOneHome/home"
		what = "Setup home"
	}

	m.loading = true
	m.status = "opening " + what + "…"
	return m, tea.Batch(sf.OpenInOrg(org, path, what), m.spinner.Tick)
}

func (m model) runMeta() (tea.Model, tea.Cmd) {
	org := m.selectedOrg()
	if org == "" {
		m.err = "select an org first"
		return m, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		m.err = "could not resolve cwd: " + err.Error()
		return m, nil
	}
	// First ctrl+r in the tab → load preview. Subsequent → dry-run deploy.
	if !m.meta.Loaded() || m.metaLoadedFor != org {
		m.loading = true
		m.status = "loading deploy preview from " + dir + "…"
		m.meta.SetStatus("loading deploy preview…")
		return m, tea.Batch(sf.LoadDeployPreview(org, dir), m.spinner.Tick)
	}
	m.loading = true
	m.status = "running deploy dry-run…"
	m.meta.SetStatus("running deploy dry-run…")
	return m, tea.Batch(sf.RunDeployDryRun(org, m.meta.ProjectDir()), m.spinner.Tick)
}

func (m model) confirmDeploy() (tea.Model, tea.Cmd) {
	org := m.selectedOrg()
	if org == "" {
		m.err = "select an org first"
		return m, nil
	}
	if !m.meta.Loaded() || m.metaLoadedFor != org {
		m.err = "load a deploy preview first (ctrl+r)"
		return m, nil
	}
	m.deployConfirm = true
	m.err = ""
	return m, nil
}

func (m model) openFieldDiff() (tea.Model, tea.Cmd) {
	if !m.compare.BothLoaded() {
		m.err = "load both sides first (ctrl+r twice)"
		return m, nil
	}
	name := m.compare.SelectedSObject()
	if name == "" {
		m.err = "no sobject selected"
		return m, nil
	}
	inA, inB := m.compare.SelectedSides()
	aOrg := ""
	bOrg := ""
	if inA {
		aOrg = m.compare.AOrg()
	}
	if inB {
		bOrg = m.compare.BOrg()
	}
	m.loading = true
	m.status = "describing " + name + " on both sides…"
	m.compare.SetStatus("describing " + name + " on both sides…")
	return m, tea.Batch(sf.LoadFieldDiff(aOrg, bOrg, name), m.spinner.Tick)
}

func (m model) runCompare() (tea.Model, tea.Cmd) {
	org := m.selectedOrg()
	if org == "" {
		m.err = "select an org first"
		return m, nil
	}
	side := m.compare.NextSide()
	if side == "" {
		m.err = "both sides loaded — press 'c' to clear and re-pick"
		return m, nil
	}
	m.loading = true
	m.status = fmt.Sprintf("loading schema (%s) from %s…", side, org)
	m.compare.SetStatus(fmt.Sprintf("loading %s side from %s…", side, org))
	return m, tea.Batch(sf.LoadOrgSchema(org, side), m.spinner.Tick)
}

func (m model) runTests() (tea.Model, tea.Cmd) {
	org := m.selectedOrg()
	if org == "" {
		m.err = "select an org first"
		return m, nil
	}
	names := m.tests.SelectedNames()
	m.loading = true
	m.err = ""
	if len(names) == 0 {
		m.status = "running all local tests…"
		m.tests.SetStatus("running all local tests… this may take a while")
	} else {
		m.status = fmt.Sprintf("running %d test class(es)…", len(names))
		m.tests.SetStatus(fmt.Sprintf("running %d test class(es)…", len(names)))
	}
	return m, tea.Batch(sf.RunTests(org, names), m.spinner.Tick)
}

func (m model) runApex() (tea.Model, tea.Cmd) {
	org := m.selectedOrg()
	if org == "" {
		m.err = "select an org first"
		return m, nil
	}
	code := strings.TrimSpace(m.apex.Value())
	if code == "" {
		m.err = "apex editor is empty"
		return m, nil
	}
	m.loading = true
	m.err = ""
	m.apex.Running = true
	m.status = "executing apex…"
	return m, tea.Batch(sf.RunApex(org, m.apex.Value()), m.spinner.Tick)
}

func (m model) runQuery() (tea.Model, tea.Cmd) {
	org := m.selectedOrg()
	if org == "" {
		m.err = "select an org first"
		return m, nil
	}
	soql := strings.TrimSpace(m.query.Value())
	if soql == "" {
		m.err = "query is empty"
		return m, nil
	}
	m.loading = true
	m.err = ""
	if m.useTooling {
		m.status = "running query (Tooling API)…"
	} else {
		m.status = "running query…"
	}
	m.ac.Hide()
	runner := sf.RunQuery
	if m.useTooling {
		runner = sf.RunQueryTooling
	}
	return m, tea.Batch(runner(org, soql), m.spinner.Tick)
}

func (m model) toggleLogTail() (tea.Model, tea.Cmd) {
	if m.logs.Running {
		m.stopTail()
		m.logs.Running = false
		m.status = "log tail stopped"
		return m, nil
	}
	org := m.selectedOrg()
	if org == "" {
		m.err = "select an org first"
		return m, nil
	}
	tail, cmd, err := sf.StartLogTail(org)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.tail = tail
	m.logs.Clear()
	m.logs.Running = true
	m.status = "tailing logs for " + org
	return m, cmd
}

func (m *model) stopTail() {
	if m.tail != nil {
		m.tail.Stop()
		m.tail = nil
	}
}

func (m *model) layout() {
	sidebarW := 28
	mainW := m.width - sidebarW - 4
	if mainW < 30 {
		mainW = 30
	}
	bodyH := m.height - 5
	if bodyH < 10 {
		bodyH = 10
	}

	m.orgs.List.SetSize(sidebarW, bodyH)

	queryH := 6
	resultsH := bodyH - queryH - 2
	if resultsH < 3 {
		resultsH = 3
	}
	m.query.Area.SetWidth(mainW)
	m.query.Area.SetHeight(queryH)
	m.results.SetSize(mainW, resultsH)

	m.objects.SetSize(mainW, bodyH)
	m.logs.SetSize(mainW, bodyH)

	apexEditorH := bodyH / 3
	if apexEditorH < 4 {
		apexEditorH = 4
	}
	apexOutputH := bodyH - apexEditorH - 2
	if apexOutputH < 3 {
		apexOutputH = 3
	}
	m.apex.SetSize(mainW, apexEditorH, apexOutputH)
	m.limits.SetSize(mainW, bodyH)
	m.perms.SetSize(mainW, bodyH)
	m.tests.SetSize(mainW, bodyH)
	m.meta.SetSize(mainW, bodyH)
	m.compare.SetSize(mainW, bodyH)
	m.picker.SetSize(m.width-6, m.height-6)
	m.palette.SetSize(m.width-6, m.height-6)
	m.record.SetSize(m.width-6, m.height-4)
}

func (m model) View() string {
	if !m.ready {
		return "initializing…"
	}

	tabs := m.renderHeader()

	orgsView := panes.BorderFor(m.focus == focusOrgs).Render(m.orgs.View())

	var mainView string
	switch m.tab {
	case tabQuery:
		// Reserve space for the autocomplete popup by shrinking the results
		// pane when it's visible, so total height stays within the terminal.
		acH := m.ac.DisplayHeight()
		bodyH := m.height - 5 // matches layout(): tabs(1)+footer(1)+help(1)+margins
		if bodyH < 10 {
			bodyH = 10
		}
		queryH := 6
		resultsH := bodyH - queryH - 2 - acH
		if resultsH < 3 {
			resultsH = 3
		}
		m.results.SetSize(m.results.Width, resultsH)

		queryView := panes.BorderFor(m.focus == focusMainA).Render(m.query.View())
		resultsView := panes.BorderFor(m.focus == focusMainB).Render(m.results.View())
		if acH > 0 && m.focus == focusMainA {
			mainW := m.width - 32
			acView := m.ac.View(mainW)
			mainView = lipgloss.JoinVertical(lipgloss.Left, queryView, acView, resultsView)
		} else {
			mainView = lipgloss.JoinVertical(lipgloss.Left, queryView, resultsView)
		}
	case tabObjects:
		mainView = m.objects.View(m.focus == focusMainA, m.focus == focusMainB)
	case tabLogs:
		mainView = panes.BorderFor(true).Render(m.logs.View())
	case tabApex:
		editorView := panes.BorderFor(m.focus == focusMainA).Render(m.apex.EditorView())
		outputView := panes.BorderFor(m.focus == focusMainB).Render(m.apex.OutputView())
		mainView = lipgloss.JoinVertical(lipgloss.Left, editorView, outputView)
	case tabLimits:
		mainView = panes.BorderFor(true).Render(m.limits.View())
	case tabPerms:
		mainView = m.perms.View(m.focus == focusMainA, m.focus == focusMainB)
	case tabTests:
		mainView = m.tests.View(m.focus == focusMainA, m.focus == focusMainB)
	case tabMeta:
		mainView = m.meta.View(m.focus == focusMainA, m.focus == focusMainB)
	case tabCompare:
		mainView = m.compare.View(m.focus == focusMainA, m.focus == focusMainB)
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, orgsView, mainView)

	var footer string
	switch {
	case m.saving:
		footer = statusStyle.Render("save as: ") + m.saveInput.View() + helpStyle.Render("  (enter to save, esc to cancel)")
	case m.exporting:
		footer = statusStyle.Render("export as: ") + m.exportInput.View() + helpStyle.Render("  (enter to write, esc to cancel)")
	case m.flsPrompt:
		footer = statusStyle.Render("field perms on: ") + m.flsInput.View() + helpStyle.Render("  (enter to load, esc to cancel)")
	case m.deployConfirm:
		warn := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).
			Render("REAL DEPLOY to " + m.selectedOrg() + " — proceed? y/n")
		footer = warn
	case m.err != "":
		footer = errStyle.Render("error: " + m.err)
	case m.loading:
		footer = statusStyle.Render(m.spinner.View() + " " + m.status)
	default:
		footer = statusStyle.Render(m.status)
	}

	help := helpStyle.Render(m.helpLine())

	view := lipgloss.JoinVertical(lipgloss.Left, tabs, body, footer, help)

	if m.pickerOn {
		overlay := overlayStyle.Render(m.picker.List.View())
		return overlay + "\n" + helpStyle.Render("enter: load · esc: close")
	}

	if m.paletteOn {
		overlay := overlayStyle.Render(m.palette.List.View())
		return overlay + "\n" + helpStyle.Render("type to filter · enter: run · esc: close")
	}

	if m.helpOn {
		overlay := overlayStyle.Render(renderHelpModal(m.width-8, m.height-6))
		return overlay + "\n" + helpStyle.Render("?/esc/q: close")
	}

	if m.record.Visible {
		help := helpStyle.Render("tab: switch focus · enter: edit value · ctrl+s: save · esc: cancel · ctrl+w: discard")
		return overlayStyle.Render(m.record.View()) + "\n" + help
	}

	return view
}

func renderHelpModal(width, height int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	row := func(k, desc string) string {
		return fmt.Sprintf("  %-22s %s", keyStyle.Render(k), desc)
	}

	var b strings.Builder
	fmt.Fprintln(&b, titleStyle.Render("sf-tui — keybindings"))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, sectionStyle.Render("Global"))
	fmt.Fprintln(&b, row("alt+1 … alt+9", "switch tab (Query/Objects/Logs/Apex/Limits/Perms/Tests/Meta/Compare)"))
	fmt.Fprintln(&b, row("tab / shift+tab", "cycle pane focus within current tab"))
	fmt.Fprintln(&b, row("ctrl+k", "command palette"))
	fmt.Fprintln(&b, row("ctrl+o", "open in org (record / sobject / Setup home)"))
	fmt.Fprintln(&b, row("?", "this help"))
	fmt.Fprintln(&b, row("ctrl+c / q", "quit"))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, sectionStyle.Render("Lists"))
	fmt.Fprintln(&b, row("↑↓ / j k", "move cursor"))
	fmt.Fprintln(&b, row("← → / h l", "page up/down"))
	fmt.Fprintln(&b, row("g / home", "jump to top"))
	fmt.Fprintln(&b, row("G / end", "jump to bottom"))
	fmt.Fprintln(&b, row("/", "filter"))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, sectionStyle.Render("Viewports (output panes)"))
	fmt.Fprintln(&b, row("↑↓", "scroll line"))
	fmt.Fprintln(&b, row("pgup / pgdn / b / f", "scroll page"))
	fmt.Fprintln(&b, row("u / d", "scroll half page"))
	fmt.Fprintln(&b, row("g / G / home / end", "jump to top/bottom"))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, sectionStyle.Render("Query tab"))
	fmt.Fprintln(&b, row("ctrl+r", "run query"))
	fmt.Fprintln(&b, row("ctrl+t", "toggle Tooling API"))
	fmt.Fprintln(&b, row("ctrl+space", "force autocomplete"))
	fmt.Fprintln(&b, row("ctrl+s / ctrl+e", "save query / export query"))
	fmt.Fprintln(&b, row("ctrl+p", "saved queries + history"))
	fmt.Fprintln(&b, row("ctrl+y / ctrl+x", "copy results TSV / write CSV"))
	fmt.Fprintln(&b, row("enter (results)", "edit selected record"))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, sectionStyle.Render("Logs / Apex / Tests / Perms"))
	fmt.Fprintln(&b, row("ctrl+l", "start/stop tail (Logs)"))
	fmt.Fprintln(&b, row("ctrl+i", "toggle log inspector view (Logs)"))
	fmt.Fprintln(&b, row("ctrl+r", "execute Apex / run selected tests / refresh"))
	fmt.Fprintln(&b, row("space", "toggle test class selection (Tests)"))
	fmt.Fprintln(&b, row("f", "load field-level perms for an sobject (Perms)"))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, sectionStyle.Render("Meta / Compare"))
	fmt.Fprintln(&b, row("ctrl+r", "preview + dry-run (Meta) / load A then B (Compare)"))
	fmt.Fprintln(&b, row("ctrl+d", "real deploy (Meta) — confirms with y/n"))
	fmt.Fprintln(&b, row("enter", "field-level diff for selected sobject (Compare)"))
	fmt.Fprintln(&b, row("esc", "back to schema summary (Compare field view)"))
	fmt.Fprintln(&b, row("c", "clear loaded sides (Compare)"))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, dimStyle.Render("Press ? again, esc, or q to close."))

	body := b.String()
	if width > 0 {
		body = lipgloss.NewStyle().Width(width).Render(body)
	}
	if height > 0 {
		body = lipgloss.NewStyle().Height(height).Render(body)
	}
	return body
}

func renderTabs(active tab) string {
	names := []string{"Query", "Objects", "Logs", "Apex", "Limits", "Perms", "Tests", "Meta", "Compare"}
	parts := make([]string, 0, len(names))
	for i, n := range names {
		label := fmt.Sprintf("%d %s", i+1, n)
		if tab(i) == active {
			parts = append(parts, activeTab.Render(label))
		} else {
			parts = append(parts, tabStyle.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m model) renderHeader() string {
	tabs := renderTabs(m.tab)
	var badges []string
	if m.useTooling && m.tab == tabQuery {
		toolingBadge := lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("220")).
			Foreground(lipgloss.Color("16")).
			Bold(true).
			Render("TOOLING")
		badges = append(badges, toolingBadge)
	}
	if org := m.selectedOrg(); org != "" {
		badges = append(badges, orgBadge.Render("⏵ "+org))
	} else {
		badges = append(badges, orgBadgeNone.Render("⏵ no org"))
	}
	right := lipgloss.JoinHorizontal(lipgloss.Top, badges...)
	gap := m.width - lipgloss.Width(tabs) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs, strings.Repeat(" ", gap), right)
}

func (m model) helpLine() string {
	switch m.tab {
	case tabQuery:
		if m.ac.Visible {
			return "↑↓: move · tab/enter: accept · esc: close · (typing updates suggestions)"
		}
		if m.focus == focusMainB {
			return "↑↓: rows · enter: edit · ctrl+o: open record · ctrl+y: TSV · ctrl+x: CSV · ctrl+k: palette"
		}
		return "ctrl+space: suggest · ctrl+r: run · ctrl+s: save · ctrl+e: export · ctrl+p: history · ctrl+k: palette"
	case tabObjects:
		return "tab: pane · enter: describe · /: filter · ctrl+o: open setup · ctrl+k: palette · q: quit"
	case tabLogs:
		return "ctrl+l: tail · ctrl+i: inspector view · ↑↓: scroll · ctrl+k: palette · q: quit"
	case tabApex:
		if m.focus == focusMainA {
			return "tab: pane · ctrl+r: execute · ctrl+o: open org · ctrl+k: palette · q: quit"
		}
		return "↑↓: scroll · tab: pane · ctrl+r: re-run · ctrl+k: palette · q: quit"
	case tabLimits:
		return "ctrl+r: refresh · ctrl+o: open org · ↑↓: scroll · ctrl+k: palette · q: quit"
	case tabPerms:
		if m.focus == focusMainA {
			return "↑↓: pick user · enter: load · f: field perms · /: filter · tab: detail · ctrl+k: palette"
		}
		return "↑↓: scroll · f: field perms · tab: user list · ctrl+k: palette · q: quit"
	case tabTests:
		if m.focus == focusMainA {
			return "↑↓: pick · space: select · /: filter · ctrl+r: run · tab: detail · ctrl+k: palette"
		}
		return "↑↓: scroll · tab: list · ctrl+r: re-run · ctrl+k: palette · q: quit"
	case tabMeta:
		return "ctrl+r: preview / dry-run · ctrl+d: real deploy · /: filter · tab: pane · ctrl+k: palette"
	case tabCompare:
		if m.compare.InFieldView() {
			return "esc: back to summary · ↑↓: scroll fields · tab: pane · ctrl+k: palette"
		}
		return "ctrl+r: load A then B · enter: field diff · c: clear · /: filter · ctrl+k: palette"
	}
	return ""
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
