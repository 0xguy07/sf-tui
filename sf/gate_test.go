package sf

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const gateFixtures = "../testdata/gate"

func readGateFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(gateFixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseOrgListMarksScratch(t *testing.T) {
	orgs, err := parseOrgList(readGateFixture(t, "org_list.json"))
	if err != nil {
		t.Fatal(err)
	}
	scratch := map[string]bool{}
	for _, o := range orgs {
		scratch[o.Username] = scratch[o.Username] || o.Scratch()
	}
	want := map[string]bool{
		"admin@acme.com":       false,
		"admin@acme.com.uat":   false,
		"me@dev.example":       false,
		"test-abc@example.com": true,
	}
	for u, w := range want {
		if got, ok := scratch[u]; !ok || got != w {
			t.Errorf("%s scratch = %v (present %v), want %v", u, got, ok, w)
		}
	}
	if (Org{DevHub: "hub@example.com"}).Scratch() != true {
		t.Error("devHubUsername alone should mark scratch")
	}
}

func TestClassifyOrgResult(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name    string
		scratch bool
		fixture string
		sandbox *bool
		orgType string
		err     error
		want    OrgClass
	}{
		{name: "scratch flag wins", scratch: true, err: errors.New("unreachable"), want: ClassScratch},
		{name: "sandbox", fixture: "organization_sandbox.json", want: ClassSandbox},
		{name: "developer edition", fixture: "organization_de.json", want: ClassDev},
		{name: "enterprise production", fixture: "organization_prod.json", want: ClassProd},
		{name: "no rows", fixture: "organization_empty.json", want: ClassProd},
		{name: "query error", err: errors.New("INVALID_SESSION_ID"), want: ClassProd},
		{name: "missing IsSandbox", orgType: "Developer Edition", want: ClassProd},
		{name: "sandbox beats DE type", sandbox: &yes, orgType: "Developer Edition", want: ClassSandbox},
		{name: "unknown edition", sandbox: &no, orgType: "Professional Edition", want: ClassProd},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sandbox, orgType, err := c.sandbox, c.orgType, c.err
			if c.fixture != "" {
				sandbox, orgType, err = parseOrganizationClass(readGateFixture(t, c.fixture))
			}
			if got := ClassifyOrgResult(c.scratch, sandbox, orgType, err); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

var testOrgs = []Org{
	{Username: "admin@acme.com", Alias: "acme-prod"},
	{Username: "admin@acme.com.uat", Alias: "acme-uat"},
	{Username: "me@dev.example"},
}

func newTestGate(cfg GateConfig, cfgErr error, classes map[string]OrgClass) *Gate {
	g := NewGate(cfg, cfgErr)
	g.SetOrgs(testOrgs)
	for u, c := range classes {
		g.SetClass(u, c)
	}
	return g
}

func TestLockResolution(t *testing.T) {
	classes := map[string]OrgClass{
		"admin@acme.com":     ClassProd,
		"admin@acme.com.uat": ClassSandbox,
		"me@dev.example":     ClassDev,
	}
	cfg := func(user, writes string) GateConfig {
		return GateConfig{Orgs: map[string]OrgConfig{user: {Writes: writes}}}
	}
	cases := []struct {
		name         string
		cfg          GateConfig
		cfgErr       error
		session      func(g *Gate)
		org          string
		locked, prot bool
	}{
		{name: "prod locked by default", org: "acme-prod", locked: true, prot: true},
		{name: "sandbox unlocked by default", org: "acme-uat"},
		{name: "dev unlocked by default", org: "me@dev.example"},
		{name: "config unlocks prod", cfg: cfg("admin@acme.com", WritesUnlocked), org: "acme-prod", prot: true},
		{name: "config locks sandbox", cfg: cfg("admin@acme.com.uat", WritesLocked), org: "acme-uat", locked: true, prot: true},
		{name: "config keyed by username, matched case-insensitively", cfg: cfg("ADMIN@acme.com.UAT", WritesLocked), org: "acme-uat", locked: true, prot: true},
		{name: "session unlock prod", session: func(g *Gate) { g.SessionUnlock("acme-prod") }, org: "acme-prod", prot: true},
		{name: "session unlock via username", session: func(g *Gate) { g.SessionUnlock("admin@acme.com") }, org: "acme-prod", prot: true},
		{name: "session unlock beats config lock", cfg: cfg("me@dev.example", WritesLocked), session: func(g *Gate) { g.SessionUnlock("me@dev.example") }, org: "me@dev.example", prot: true},
		{name: "session lock beats config unlock", cfg: cfg("admin@acme.com", WritesUnlocked), session: func(g *Gate) { g.SessionLock("acme-prod") }, org: "acme-prod", locked: true, prot: true},
		{name: "lock writes on a sandbox", session: func(g *Gate) { g.SessionLock("acme-uat") }, org: "acme-uat", locked: true},
		{name: "re-lock after unlock", session: func(g *Gate) { g.SessionUnlock("acme-prod"); g.SessionLock("acme-prod") }, org: "acme-prod", locked: true, prot: true},
		{name: "broken config locks everything", cfgErr: errors.New("bad json"), org: "me@dev.example", locked: true, prot: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var cfg GateConfig
			if c.cfg.Orgs != nil {
				cfg = GateConfig{Orgs: map[string]OrgConfig{}}
				for u, oc := range c.cfg.Orgs {
					cfg.Orgs[lc(u)] = oc
				}
			}
			g := newTestGate(cfg, c.cfgErr, classes)
			if c.session != nil {
				c.session(g)
			}
			s := g.State(c.org)
			if s.Locked != c.locked || s.Protected != c.prot {
				t.Errorf("state = %+v, want locked=%v protected=%v", s, c.locked, c.prot)
			}
			err := g.Check(c.org, ActionRecordSave)
			if (err != nil) != c.locked {
				t.Errorf("Check err = %v, want locked=%v", err, c.locked)
			}
		})
	}
}

func TestUnclassifiedOrgFailsClosed(t *testing.T) {
	g := newTestGate(GateConfig{}, nil, nil)
	if _, need := g.NeedsClassify("acme-uat"); !need {
		t.Fatal("first NeedsClassify should launch classification")
	}
	if _, need := g.NeedsClassify("acme-uat"); need {
		t.Error("classification already in flight")
	}
	var be *BlockedError
	if err := g.Check("acme-uat", ActionDeploy); !errors.As(err, &be) || !be.Pending {
		t.Errorf("pending org should be blocked as pending, got %v", err)
	}
	g.SetClass("admin@acme.com.uat", ClassSandbox)
	if err := g.Check("acme-uat", ActionDeploy); err != nil {
		t.Errorf("classified sandbox should be writable, got %v", err)
	}
}

func TestBlockedMessage(t *testing.T) {
	g := newTestGate(GateConfig{}, nil, map[string]OrgClass{"admin@acme.com": ClassProd})
	err := g.Check("acme-prod", ActionApex)
	if err == nil || err.Error() != "Blocked: acme-prod is locked. ctrl+k → Unlock writes for this session." {
		t.Errorf("message = %v", err)
	}
}

func TestSessionUnlockNeverPersists(t *testing.T) {
	isolateConfig(t)
	path, err := gateConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadGateConfig()
	if err != nil {
		t.Fatal(err)
	}
	g := newTestGate(cfg, nil, map[string]OrgClass{"admin@acme.com": ClassProd})
	g.SessionUnlock("acme-prod")
	if g.Check("acme-prod", ActionDeploy) != nil {
		t.Fatal("session unlock should allow the write")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("session unlock wrote %s", path)
	}

	const body = `{"orgs":{"me@dev.example":{"writes":"locked"}}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ = LoadGateConfig()
	g = newTestGate(cfg, nil, map[string]OrgClass{"me@dev.example": ClassDev})
	g.SessionUnlock("me@dev.example")
	if b, _ := os.ReadFile(path); string(b) != body {
		t.Errorf("config changed by session unlock: %s", b)
	}

	cfg, _ = LoadGateConfig()
	relaunch := newTestGate(cfg, nil, map[string]OrgClass{"admin@acme.com": ClassProd, "me@dev.example": ClassDev})
	if relaunch.Check("acme-prod", ActionDeploy) == nil || relaunch.Check("me@dev.example", ActionDeploy) == nil {
		t.Error("a relaunch should be locked again")
	}
}

func TestLoadGateConfig(t *testing.T) {
	isolateConfig(t)
	path, _ := gateConfigPath()
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"orgs":{"Admin@Acme.com":{"writes":"unlocked"},"x@y.z":{}}}`)
	cfg, err := LoadGateConfig()
	if err != nil || cfg.Orgs["admin@acme.com"].Writes != WritesUnlocked {
		t.Errorf("cfg = %+v, err = %v", cfg, err)
	}
	write(`{"orgs":{"a@b.c":{"writes":"LOCKED"}}}`)
	if _, err := LoadGateConfig(); err == nil {
		t.Error("unknown writes value should be an error")
	}
	write(`{not json`)
	if _, err := LoadGateConfig(); err == nil {
		t.Error("bad JSON should be an error")
	}
}

func TestConfirmationStrength(t *testing.T) {
	g := newTestGate(GateConfig{Orgs: map[string]OrgConfig{
		"admin@acme.com":     {Writes: WritesUnlocked},
		"admin@acme.com.uat": {Writes: WritesLocked},
	}}, nil, map[string]OrgClass{
		"admin@acme.com":     ClassProd,
		"admin@acme.com.uat": ClassSandbox,
		"me@dev.example":     ClassDev,
	})
	g.SessionUnlock("acme-uat")
	cases := []struct {
		org    string
		action WriteAction
		want   Confirm
	}{
		{"me@dev.example", ActionRecordSave, ConfirmNone},
		{"me@dev.example", ActionApex, ConfirmNone},
		{"me@dev.example", ActionDeploy, ConfirmYesNo},
		{"acme-prod", ActionRecordSave, ConfirmYesNo},
		{"acme-prod", ActionApex, ConfirmYesNo},
		{"acme-prod", ActionDeploy, ConfirmTyped},
		{"acme-uat", ActionRecordSave, ConfirmYesNo},
		{"acme-uat", ActionDeploy, ConfirmTyped},
	}
	for _, c := range cases {
		if got := g.Confirmation(c.org, c.action); got != c.want {
			t.Errorf("%s %s = %v, want %v", c.org, c.action, got, c.want)
		}
	}
}

type blockAll struct{ calls []WriteAction }

func (b *blockAll) Check(org string, a WriteAction) error {
	b.calls = append(b.calls, a)
	return &BlockedError{Org: org}
}
func (b *blockAll) Confirmation(string, WriteAction) Confirm { return ConfirmNone }

func TestWriteCommandsCheckGate(t *testing.T) {
	g := &blockAll{}
	cmds := map[WriteAction]func() any{
		ActionRecordSave: func() any {
			return UpdateRecord(g, "acme-prod", "Account", "001000000000001AAA", map[string]string{"Name": "x"})()
		},
		ActionApex:   func() any { return RunApex(g, "acme-prod", "System.debug(1);")() },
		ActionDeploy: func() any { return RunDeploy(g, "acme-prod", t.TempDir())() },
	}
	for action, run := range cmds {
		msg, ok := run().(ErrMsg)
		var be *BlockedError
		if !ok || !errors.As(msg.Err, &be) {
			t.Errorf("%s: want a BlockedError, got %#v", action, msg)
		}
	}
	if len(g.calls) != 3 {
		t.Errorf("gate checked %d times, want 3", len(g.calls))
	}
	if msg, ok := UpdateRecord(nil, "o", "Account", "001", map[string]string{"a": "b"})().(ErrMsg); !ok || !errors.As(msg.Err, new(*BlockedError)) {
		t.Errorf("a nil gate must block, got %#v", msg)
	}
}
