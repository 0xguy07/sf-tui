package sf

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type UserBrief struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Username    string `json:"username"`
	IsActive    bool   `json:"isActive"`
	ProfileName string `json:"profile,omitempty"` // flattened from Profile.Name
}

// PermissionsReport is the full answer to "what can this user do, and which
// permission set or profile grants it?" for one user. Sections that fail to
// load are recorded in Errors; the rest of the report stands.
type PermissionsReport struct {
	User        UserBrief           `json:"user"`
	Assignments []PermSetAssignment `json:"assignments"`
	Overrides   Overrides           `json:"overrides"`
	Objects     []ObjectPerm        `json:"objects"`
	Object      string              `json:"object,omitempty"` // the --object value, "" otherwise
	Fields      []FieldPerm         `json:"fields"`           // populated only when Object != ""; never null
	Errors      []SectionError      `json:"errors"`
}

type UsersLoadedMsg struct {
	Users []UserBrief
}

type PermSetAssignment struct {
	PermSetID    string `json:"permSetId"`
	PermSetLabel string `json:"label"`
	IsProfile    bool   `json:"isProfile"`
	ProfileName  string `json:"profileName,omitempty"` // when IsProfile
	GroupLabel   string `json:"groupLabel,omitempty"`  // PermissionSetGroup.MasterLabel when assigned via a group
}

// Overrides are system permissions on an assigned permission set that
// override object permissions. They are reported, not applied to Objects.
type Overrides struct {
	ModifyAllData []string `json:"modifyAllData"`
	ViewAllData   []string `json:"viewAllData"`
}

type ObjectPerm struct {
	SObject   string   `json:"sobject"`
	Read      bool     `json:"read"`
	Create    bool     `json:"create"`
	Edit      bool     `json:"edit"`
	Delete    bool     `json:"delete"`
	ViewAll   bool     `json:"viewAll"`
	ModifyAll bool     `json:"modifyAll"`
	Sources   []string `json:"sources"` // perm set / profile labels granting any of these
	// Grants lists, per permission (PermRead … PermModifyAll), the sources
	// that grant it. Every key is present; an empty slice means not granted.
	Grants map[string][]string `json:"grants"`
}

type PermissionsLoadedMsg struct {
	User        UserBrief
	Assignments []PermSetAssignment
	Objects     []ObjectPerm // sorted, deduped by SObject
	Overrides   Overrides    // Modify/View All Data granted by any assigned set
}

type FieldPerm struct {
	Field   string   `json:"field"` // API name without sobject prefix (e.g. "Industry")
	Read    bool     `json:"read"`
	Edit    bool     `json:"edit"`
	Sources []string `json:"sources"`
	// Grants: PermRead and PermEdit → sources. Every key present.
	Grants map[string][]string `json:"grants"`
}

type FieldPermissionsLoadedMsg struct {
	User    UserBrief
	SObject string
	Fields  []FieldPerm
}

func soqlEscape(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}

// LoadUsers grabs the active users in the org for the user picker.
func LoadUsers(orgAliasOrUser string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		soql := "SELECT Id, Name, Username, IsActive, Profile.Name FROM User WHERE IsActive = true ORDER BY Name LIMIT 1000"
		out, err := runSOQLRaw(orgAliasOrUser, soql, false)
		if err != nil {
			return ErrMsg{Err: err}
		}
		var r struct {
			Result struct {
				Records []map[string]any `json:"records"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &r); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse users: %w", err)}
		}
		users := make([]UserBrief, 0, len(r.Result.Records))
		for _, rec := range r.Result.Records {
			u := UserBrief{
				ID:       stringField(rec, "Id"),
				Name:     stringField(rec, "Name"),
				Username: stringField(rec, "Username"),
				IsActive: boolField(rec, "IsActive"),
			}
			if p, ok := rec["Profile"].(map[string]any); ok {
				u.ProfileName = stringField(p, "Name")
			}
			users = append(users, u)
		}
		return UsersLoadedMsg{Users: users}
	}
}

// LoadPermissions fetches assignments, overrides and object perms for one user.
func LoadPermissions(orgAliasOrUser string, user UserBrief) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		r, err := BuildPermissionsReport(DefaultSOQLRunner(), orgAliasOrUser, user, "")
		if err != nil {
			return ErrMsg{Err: err}
		}
		if len(r.Errors) > 0 {
			return ErrMsg{Err: fmt.Errorf("%s: %s", r.Errors[0].Section, r.Errors[0].Message)}
		}
		return PermissionsLoadedMsg{User: r.User, Assignments: r.Assignments, Objects: r.Objects, Overrides: r.Overrides}
	}
}

// LoadFieldPermissions fetches field-level read/edit perms for one user on one
// sobject, with the perm set / profile labels that grant them.
func LoadFieldPermissions(orgAliasOrUser string, user UserBrief, sobject string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		if sobject == "" {
			return ErrMsg{Err: fmt.Errorf("no sobject specified")}
		}
		r, err := BuildPermissionsReport(DefaultSOQLRunner(), orgAliasOrUser, user, sobject)
		if err != nil {
			return ErrMsg{Err: err}
		}
		for _, e := range r.Errors {
			if e.Section == "fields" {
				return ErrMsg{Err: fmt.Errorf("fields: %s", e.Message)}
			}
		}
		return FieldPermissionsLoadedMsg{User: user, SObject: sobject, Fields: r.Fields}
	}
}

func runSOQLRaw(orgAliasOrUser, soql string, tooling bool) ([]byte, error) {
	args := []string{"data", "query",
		"--target-org", orgAliasOrUser,
		"--query", soql,
		"--json",
	}
	if tooling {
		args = append(args, "--use-tooling-api")
	}
	cmd := exec.Command("sf", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, sfCommandError(out, err)
	}
	return out, nil
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func boolField(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func contains(haystack []string, s string) bool {
	for _, h := range haystack {
		if h == s {
			return true
		}
	}
	return false
}
