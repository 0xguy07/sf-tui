package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/0xguy07/sf-tui/sf"
)

type stubGate struct {
	err     error
	confirm sf.Confirm
	checks  []sf.WriteAction
}

func (s *stubGate) Check(_ string, a sf.WriteAction) error {
	s.checks = append(s.checks, a)
	return s.err
}

func (s *stubGate) Confirmation(string, sf.WriteAction) sf.Confirm { return s.confirm }

func isolateHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdg"))
	return tmp
}

func testModel(t *testing.T) model {
	t.Helper()
	isolateHome(t)
	var mm tea.Model = initialModel()
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	mm, _ = mm.Update(sf.OrgsLoadedMsg{Orgs: []sf.Org{{Alias: "acme-prod", Username: "admin@acme.com"}}})
	return mm.(model)
}

func keys(t *testing.T, m model, ks ...string) (model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	var mm tea.Model = m
	for _, k := range ks {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "ctrl+s":
			msg = tea.KeyMsg{Type: tea.KeyCtrlS}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		mm, cmd = mm.Update(msg)
	}
	return mm.(model), cmd
}

// writePaths starts each gated write the way a user would.
var writePaths = []struct {
	action sf.WriteAction
	start  func(t *testing.T, m model) (model, tea.Cmd)
}{
	{sf.ActionRecordSave, func(t *testing.T, m model) (model, tea.Cmd) {
		m.record.Open("Account", sf.Describe{Name: "Account", Fields: []sf.Field{{Name: "Name", Updateable: true}}},
			sf.QueryRecord{"Id": "001000000000001AAA", "Name": "Acme"})
		m, _ = keys(t, m, "enter", "!")
		return keys(t, m, "ctrl+s")
	}},
	{sf.ActionApex, func(t *testing.T, m model) (model, tea.Cmd) {
		m.apex.Area.SetValue("System.debug(1);")
		mm, cmd := m.runApex()
		return mm.(model), cmd
	}},
	{sf.ActionDeploy, func(t *testing.T, m model) (model, tea.Cmd) {
		m.meta.SetPreview(sf.DeployPreviewLoadedMsg{ProjectDir: t.TempDir()})
		m.metaLoadedFor = "acme-prod"
		mm, cmd := m.confirmDeploy()
		return mm.(model), cmd
	}},
}

func TestWritePathsBlockedByGate(t *testing.T) {
	for _, wp := range writePaths {
		t.Run(string(wp.action), func(t *testing.T) {
			m := testModel(t)
			g := &stubGate{err: &sf.BlockedError{Org: "acme-prod"}}
			m.gate = g
			m, cmd := wp.start(t, m)
			if cmd != nil || m.pending != nil || m.loading {
				t.Errorf("blocked write started: cmd=%v pending=%v loading=%v", cmd != nil, m.pending, m.loading)
			}
			if m.err != "Blocked: acme-prod is locked. ctrl+k → Unlock writes for this session." {
				t.Errorf("err = %q", m.err)
			}
			if len(g.checks) != 1 || g.checks[0] != wp.action {
				t.Errorf("gate checks = %v, want [%s]", g.checks, wp.action)
			}
		})
	}
}

func TestWritePathsConfirmation(t *testing.T) {
	for _, wp := range writePaths {
		t.Run(string(wp.action)+"/none", func(t *testing.T) {
			m := testModel(t)
			m.gate = &stubGate{confirm: sf.ConfirmNone}
			m, cmd := wp.start(t, m)
			if cmd == nil || m.pending != nil {
				t.Errorf("unprotected write should run immediately: cmd=%v pending=%+v", cmd != nil, m.pending)
			}
		})
		t.Run(string(wp.action)+"/yes-no", func(t *testing.T) {
			m := testModel(t)
			m.gate = &stubGate{confirm: sf.ConfirmYesNo}
			m, cmd := wp.start(t, m)
			if cmd != nil || m.pending == nil || m.pending.typed {
				t.Fatalf("want a y/n prompt, got cmd=%v pending=%+v", cmd != nil, m.pending)
			}
			if p := m.pendingPrompt(); !strings.Contains(p, "acme-prod") {
				t.Errorf("prompt should name the org: %q", p)
			}
			if n, c := keys(t, m, "n"); c != nil || n.pending != nil {
				t.Error("n should cancel")
			}
			if m, cmd = keys(t, m, "y"); cmd == nil || m.pending != nil {
				t.Error("y should run the write")
			}
		})
		t.Run(string(wp.action)+"/typed", func(t *testing.T) {
			m := testModel(t)
			m.gate = &stubGate{confirm: sf.ConfirmTyped}
			m, _ = wp.start(t, m)
			if m.pending == nil || !m.pending.typed {
				t.Fatalf("want a typed prompt, got %+v", m.pending)
			}
			if w, c := keys(t, m, "y", "enter"); c != nil || w.pending != nil || !strings.Contains(w.err, "didn't match") {
				t.Errorf("y is not the org name: cmd=%v err=%q", c != nil, w.err)
			}
			typed := append(strings.Split("acme-prod", ""), "enter")
			if m, cmd := keys(t, m, typed...); cmd == nil || m.pending != nil {
				t.Errorf("typing the org name should run the write: err=%q", m.err)
			}
		})
	}
}

func TestConfirmRechecksGate(t *testing.T) {
	m := testModel(t)
	g := &stubGate{confirm: sf.ConfirmYesNo}
	m.gate = g
	m, _ = writePaths[1].start(t, m)
	g.err = &sf.BlockedError{Org: "acme-prod"}
	m, cmd := keys(t, m, "y")
	if cmd != nil || !strings.HasPrefix(m.err, "Blocked:") {
		t.Errorf("a lock that lands during the prompt must still block: cmd=%v err=%q", cmd != nil, m.err)
	}
}

func TestSessionUnlockFromPalette(t *testing.T) {
	m := testModel(t)
	m.gates.SetClass("admin@acme.com", sf.ClassProd)
	if m.gate.Check("acme-prod", sf.ActionApex) == nil {
		t.Fatal("prod should start locked")
	}
	mm, _ := m.dispatchCommand("action.unlock")
	m, _ = keys(t, mm.(model), append(strings.Split("acme-pro", ""), "enter")...)
	if m.gate.Check("acme-prod", sf.ActionApex) == nil {
		t.Error("a wrong name must not unlock")
	}
	mm, _ = m.dispatchCommand("action.unlock")
	m, _ = keys(t, mm.(model), append(strings.Split("acme-prod", ""), "enter")...)
	if err := m.gate.Check("acme-prod", sf.ActionApex); err != nil {
		t.Errorf("typed name should unlock: %v", err)
	}
	if got := orgStateText(m.gates.State("acme-prod")); got != "PROD · UNLOCKED" {
		t.Errorf("badge = %q", got)
	}
	if c := m.gate.Confirmation("acme-prod", sf.ActionDeploy); c != sf.ConfirmTyped {
		t.Errorf("unlocked prod deploy confirmation = %v, want typed", c)
	}
	mm, _ = m.dispatchCommand("action.lock")
	m = mm.(model)
	if m.gate.Check("acme-prod", sf.ActionApex) == nil {
		t.Error("Lock writes should re-lock")
	}
	path, _ := os.UserConfigDir()
	if _, err := os.Stat(filepath.Join(path, "sf-tui", "config.json")); !os.IsNotExist(err) {
		t.Error("session unlock/lock must not write config")
	}
}

func TestBadgeText(t *testing.T) {
	cases := []struct {
		st   sf.OrgState
		want string
	}{
		{sf.OrgState{Class: sf.ClassProd, Locked: true, Protected: true}, "PROD · LOCKED"},
		{sf.OrgState{Class: sf.ClassProd, Protected: true}, "PROD · UNLOCKED"},
		{sf.OrgState{Class: sf.ClassSandbox}, "SANDBOX"},
		{sf.OrgState{Class: sf.ClassSandbox, Locked: true, Protected: true}, "SANDBOX · LOCKED"},
		{sf.OrgState{Class: sf.ClassScratch}, "SCRATCH"},
		{sf.OrgState{Class: sf.ClassDev}, "DEV"},
		{sf.OrgState{Class: sf.ClassDev, Locked: true, Protected: true}, "DEV · LOCKED"},
		{sf.OrgState{Class: sf.ClassUnknown, Locked: true, Protected: true}, "CLASSIFYING…"},
	}
	for _, c := range cases {
		if got := orgStateText(c.st); got != c.want {
			t.Errorf("%+v → %q, want %q", c.st, got, c.want)
		}
	}
}
