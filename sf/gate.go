package sf

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type OrgClass string

const (
	ClassUnknown OrgClass = ""
	ClassScratch OrgClass = "scratch"
	ClassSandbox OrgClass = "sandbox"
	ClassDev     OrgClass = "dev"
	ClassProd    OrgClass = "prod"
)

type WriteAction string

const (
	ActionRecordSave WriteAction = "record save"
	ActionApex       WriteAction = "anonymous Apex"
	ActionDeploy     WriteAction = "deploy"
)

type Confirm int

const (
	ConfirmNone Confirm = iota
	ConfirmYesNo
	ConfirmTyped
)

// WriteGate is the single check every write to an org goes through. New write
// features must call Check before running and honor Confirmation.
type WriteGate interface {
	Check(org string, action WriteAction) error
	Confirmation(org string, action WriteAction) Confirm
}

type BlockedError struct {
	Org     string
	Pending bool
}

func (e *BlockedError) Error() string {
	if e.Pending {
		return "Blocked: " + e.Org + " is still being classified — try again in a moment."
	}
	return "Blocked: " + e.Org + " is locked. ctrl+k → Unlock writes for this session."
}

// checkGate runs inside every write command, so a write can't execute without a gate.
func checkGate(g WriteGate, org string, action WriteAction) error {
	if g == nil {
		return &BlockedError{Org: org}
	}
	return g.Check(org, action)
}

// ClassifyOrgResult maps the scratch flag and the Organization query result to
// a class. Anything unrecognized, including a failed query, is prod.
func ClassifyOrgResult(scratch bool, isSandbox *bool, orgType string, queryErr error) OrgClass {
	switch {
	case scratch:
		return ClassScratch
	case queryErr != nil || isSandbox == nil:
		return ClassProd
	case *isSandbox:
		return ClassSandbox
	case orgType == "Developer Edition":
		return ClassDev
	}
	return ClassProd
}

func parseOrganizationClass(out []byte) (*bool, string, error) {
	var r struct {
		Result struct {
			Records []struct {
				IsSandbox        *bool  `json:"IsSandbox"`
				OrganizationType string `json:"OrganizationType"`
			} `json:"records"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, "", fmt.Errorf("parse organization: %w", err)
	}
	if len(r.Result.Records) == 0 {
		return nil, "", errors.New("organization query returned no rows")
	}
	rec := r.Result.Records[0]
	return rec.IsSandbox, rec.OrganizationType, nil
}

type OrgClassifiedMsg struct {
	Username string
	Class    OrgClass
	Err      error
}

func ClassifyOrg(o Org) tea.Cmd {
	return func() tea.Msg {
		if o.Scratch() {
			return OrgClassifiedMsg{Username: o.Username, Class: ClassScratch}
		}
		target := o.Alias
		if target == "" {
			target = o.Username
		}
		out, err := runSOQLRaw(target, "SELECT IsSandbox, OrganizationType FROM Organization", false)
		var sandbox *bool
		var orgType string
		if err == nil {
			sandbox, orgType, err = parseOrganizationClass(out)
		}
		return OrgClassifiedMsg{Username: o.Username, Class: ClassifyOrgResult(false, sandbox, orgType, err), Err: err}
	}
}

// --- config ---

const (
	WritesLocked   = "locked"
	WritesUnlocked = "unlocked"
)

type OrgConfig struct {
	Writes string `json:"writes,omitempty"`
}

// GateConfig is config.json in the sf-tui config dir, keyed by username.
type GateConfig struct {
	Orgs map[string]OrgConfig `json:"orgs"`
}

func gateConfigPath() (string, error) {
	d, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// LoadGateConfig reads config.json. A missing file is an empty config; an
// unreadable one or an unknown writes value is an error, and the gate then
// locks every org.
func LoadGateConfig() (GateConfig, error) {
	cfg := GateConfig{Orgs: map[string]OrgConfig{}}
	p, err := gateConfigPath()
	if err != nil {
		return cfg, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	var raw GateConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return cfg, fmt.Errorf("%s: %w", p, err)
	}
	for user, oc := range raw.Orgs {
		if oc.Writes != "" && oc.Writes != WritesLocked && oc.Writes != WritesUnlocked {
			return cfg, fmt.Errorf("%s: %s: writes must be %q or %q", p, user, WritesLocked, WritesUnlocked)
		}
		cfg.Orgs[strings.ToLower(user)] = oc
	}
	return cfg, nil
}

// --- gate ---

// Gate classifies orgs and resolves their lock state for the session.
// Resolution order: session lock/unlock, then config, then class (prod locked).
type Gate struct {
	mu        sync.RWMutex
	byName    map[string]Org // alias and username (lowercased) -> org
	classes   map[string]OrgClass
	pending   map[string]bool
	session   map[string]string
	cfg       GateConfig
	cfgBroken bool
}

func NewGate(cfg GateConfig, cfgErr error) *Gate {
	if cfg.Orgs == nil {
		cfg.Orgs = map[string]OrgConfig{}
	}
	return &Gate{
		byName:    map[string]Org{},
		classes:   map[string]OrgClass{},
		pending:   map[string]bool{},
		session:   map[string]string{},
		cfg:       cfg,
		cfgBroken: cfgErr != nil,
	}
}

func (g *Gate) SetOrgs(orgs []Org) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, o := range orgs {
		if o.Username == "" {
			continue
		}
		g.byName[strings.ToLower(o.Username)] = o
		if o.Alias != "" {
			g.byName[strings.ToLower(o.Alias)] = o
		}
	}
}

// username resolves an alias or username to the lowercased username key.
func (g *Gate) username(org string) string {
	if o, ok := g.byName[strings.ToLower(org)]; ok {
		return strings.ToLower(o.Username)
	}
	return strings.ToLower(org)
}

// NeedsClassify marks org as in-flight and reports whether the caller should classify it.
func (g *Gate) NeedsClassify(org string) (Org, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	o, ok := g.byName[strings.ToLower(org)]
	if !ok {
		return Org{}, false
	}
	u := strings.ToLower(o.Username)
	if _, done := g.classes[u]; done || g.pending[u] {
		return o, false
	}
	g.pending[u] = true
	return o, true
}

func (g *Gate) SetClass(username string, c OrgClass) {
	g.mu.Lock()
	defer g.mu.Unlock()
	u := strings.ToLower(username)
	delete(g.pending, u)
	if c == ClassUnknown {
		c = ClassProd
	}
	g.classes[u] = c
}

func (g *Gate) SessionUnlock(org string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.session[g.username(org)] = WritesUnlocked
}

func (g *Gate) SessionLock(org string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.session[g.username(org)] = WritesLocked
}

// OrgState is what the header badge renders.
type OrgState struct {
	Class     OrgClass // ClassUnknown while classification is in flight
	Locked    bool
	Protected bool
}

func (g *Gate) State(org string) OrgState {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.state(g.username(org))
}

func (g *Gate) state(u string) OrgState {
	class := g.classes[u]
	cfgWrites := g.cfg.Orgs[u].Writes
	if g.cfgBroken {
		cfgWrites = WritesLocked
	}
	s := OrgState{Class: class}
	s.Protected = class == ClassProd || class == ClassUnknown || cfgWrites == WritesLocked
	switch {
	case g.session[u] != "":
		s.Locked = g.session[u] == WritesLocked
	case cfgWrites != "":
		s.Locked = cfgWrites == WritesLocked
	default:
		s.Locked = class == ClassProd || class == ClassUnknown
	}
	return s
}

func (g *Gate) Check(org string, _ WriteAction) error {
	if org == "" {
		return errors.New("no org selected")
	}
	s := g.State(org)
	if !s.Locked {
		return nil
	}
	return &BlockedError{Org: org, Pending: s.Class == ClassUnknown}
}

// Confirmation is how hard to confirm a write the gate allows. Unprotected
// orgs keep the existing behavior: deploy asks y/n, the rest don't ask.
func (g *Gate) Confirmation(org string, action WriteAction) Confirm {
	protected := g.State(org).Protected
	switch {
	case protected && action == ActionDeploy:
		return ConfirmTyped
	case protected, action == ActionDeploy:
		return ConfirmYesNo
	}
	return ConfirmNone
}
