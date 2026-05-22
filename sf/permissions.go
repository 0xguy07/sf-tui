package sf

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type UserBrief struct {
	ID          string `json:"Id"`
	Name        string `json:"Name"`
	Username    string `json:"Username"`
	IsActive    bool   `json:"IsActive"`
	ProfileName string // flattened from Profile.Name
}

type UsersLoadedMsg struct {
	Users []UserBrief
}

type PermSetAssignment struct {
	PermSetID    string
	PermSetLabel string
	IsProfile    bool
	ProfileName  string // when IsProfile
}

type ObjectPerm struct {
	SObject       string
	Read          bool
	Create        bool
	Edit          bool
	Delete        bool
	ViewAll       bool
	ModifyAll     bool
	Sources       []string // perm set / profile labels granting any of these
}

type PermissionsLoadedMsg struct {
	User        UserBrief
	Assignments []PermSetAssignment
	Objects     []ObjectPerm // sorted, deduped by SObject
}

type FieldPerm struct {
	Field   string // API name without sobject prefix (e.g. "Industry")
	Read    bool
	Edit    bool
	Sources []string
}

type FieldPermissionsLoadedMsg struct {
	User       UserBrief
	SObject    string
	Fields     []FieldPerm
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

// LoadPermissions fetches assignments + object perms for one user.
func LoadPermissions(orgAliasOrUser string, user UserBrief) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}

		// 1. Assignments.
		soql := fmt.Sprintf(
			"SELECT PermissionSetId, PermissionSet.Label, PermissionSet.IsOwnedByProfile, PermissionSet.Profile.Name "+
				"FROM PermissionSetAssignment WHERE AssigneeId = '%s'",
			soqlEscape(user.ID),
		)
		out, err := runSOQLRaw(orgAliasOrUser, soql, false)
		if err != nil {
			return ErrMsg{Err: err}
		}
		var a struct {
			Result struct {
				Records []map[string]any `json:"records"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &a); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse assignments: %w", err)}
		}

		assignments := make([]PermSetAssignment, 0, len(a.Result.Records))
		permSetIDs := make([]string, 0, len(a.Result.Records))
		for _, rec := range a.Result.Records {
			as := PermSetAssignment{PermSetID: stringField(rec, "PermissionSetId")}
			if ps, ok := rec["PermissionSet"].(map[string]any); ok {
				as.PermSetLabel = stringField(ps, "Label")
				as.IsProfile = boolField(ps, "IsOwnedByProfile")
				if prof, ok := ps["Profile"].(map[string]any); ok {
					as.ProfileName = stringField(prof, "Name")
				}
			}
			assignments = append(assignments, as)
			if as.PermSetID != "" {
				permSetIDs = append(permSetIDs, as.PermSetID)
			}
		}

		// 2. Object perms across all assigned permission sets.
		objectPerms := []ObjectPerm{}
		if len(permSetIDs) > 0 {
			ids := "'" + strings.Join(permSetIDs, "','") + "'"
			soql := fmt.Sprintf(
				"SELECT SobjectType, PermissionsRead, PermissionsCreate, PermissionsEdit, PermissionsDelete, "+
					"PermissionsViewAllRecords, PermissionsModifyAllRecords, ParentId "+
					"FROM ObjectPermissions WHERE ParentId IN (%s)", ids,
			)
			out, err := runSOQLRaw(orgAliasOrUser, soql, false)
			if err != nil {
				return ErrMsg{Err: err}
			}
			var op struct {
				Result struct {
					Records []map[string]any `json:"records"`
				} `json:"result"`
			}
			if err := json.Unmarshal(out, &op); err != nil {
				return ErrMsg{Err: fmt.Errorf("parse object perms: %w", err)}
			}

			parentLabels := map[string]string{}
			for _, as := range assignments {
				label := as.PermSetLabel
				if as.IsProfile && as.ProfileName != "" {
					label = "Profile: " + as.ProfileName
				}
				parentLabels[as.PermSetID] = label
			}

			byObject := map[string]*ObjectPerm{}
			for _, rec := range op.Result.Records {
				name := stringField(rec, "SobjectType")
				if name == "" {
					continue
				}
				existing, ok := byObject[name]
				if !ok {
					existing = &ObjectPerm{SObject: name}
					byObject[name] = existing
				}
				granted := false
				if boolField(rec, "PermissionsRead") {
					existing.Read = true
					granted = true
				}
				if boolField(rec, "PermissionsCreate") {
					existing.Create = true
					granted = true
				}
				if boolField(rec, "PermissionsEdit") {
					existing.Edit = true
					granted = true
				}
				if boolField(rec, "PermissionsDelete") {
					existing.Delete = true
					granted = true
				}
				if boolField(rec, "PermissionsViewAllRecords") {
					existing.ViewAll = true
					granted = true
				}
				if boolField(rec, "PermissionsModifyAllRecords") {
					existing.ModifyAll = true
					granted = true
				}
				if granted {
					if label := parentLabels[stringField(rec, "ParentId")]; label != "" && !contains(existing.Sources, label) {
						existing.Sources = append(existing.Sources, label)
					}
				}
			}

			objectPerms = make([]ObjectPerm, 0, len(byObject))
			for _, v := range byObject {
				objectPerms = append(objectPerms, *v)
			}
			sort.Slice(objectPerms, func(i, j int) bool {
				return objectPerms[i].SObject < objectPerms[j].SObject
			})
		}

		return PermissionsLoadedMsg{
			User:        user,
			Assignments: assignments,
			Objects:     objectPerms,
		}
	}
}

// LoadFieldPermissions fetches field-level read/edit perms for one user on one
// sobject, with the perm set / profile labels that grant them. Requires the
// user to already have at least one perm set assignment loaded — we re-query
// PermissionSetAssignment to keep this self-contained.
func LoadFieldPermissions(orgAliasOrUser string, user UserBrief, sobject string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		if sobject == "" {
			return ErrMsg{Err: fmt.Errorf("no sobject specified")}
		}

		// 1. Re-fetch assignments to get parent IDs and labels.
		soql := fmt.Sprintf(
			"SELECT PermissionSetId, PermissionSet.Label, PermissionSet.IsOwnedByProfile, PermissionSet.Profile.Name "+
				"FROM PermissionSetAssignment WHERE AssigneeId = '%s'",
			soqlEscape(user.ID),
		)
		out, err := runSOQLRaw(orgAliasOrUser, soql, false)
		if err != nil {
			return ErrMsg{Err: err}
		}
		var a struct {
			Result struct {
				Records []map[string]any `json:"records"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &a); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse assignments: %w", err)}
		}
		parentLabels := map[string]string{}
		permSetIDs := []string{}
		for _, rec := range a.Result.Records {
			id := stringField(rec, "PermissionSetId")
			if id == "" {
				continue
			}
			permSetIDs = append(permSetIDs, id)
			label := ""
			if ps, ok := rec["PermissionSet"].(map[string]any); ok {
				label = stringField(ps, "Label")
				if boolField(ps, "IsOwnedByProfile") {
					if prof, ok := ps["Profile"].(map[string]any); ok {
						if name := stringField(prof, "Name"); name != "" {
							label = "Profile: " + name
						}
					}
				}
			}
			parentLabels[id] = label
		}
		if len(permSetIDs) == 0 {
			return FieldPermissionsLoadedMsg{User: user, SObject: sobject}
		}

		// 2. FieldPermissions records. Field is stored as "SobjectType.FieldName".
		ids := "'" + strings.Join(permSetIDs, "','") + "'"
		soql = fmt.Sprintf(
			"SELECT Field, PermissionsRead, PermissionsEdit, ParentId "+
				"FROM FieldPermissions WHERE SobjectType = '%s' AND ParentId IN (%s)",
			soqlEscape(sobject), ids,
		)
		out, err = runSOQLRaw(orgAliasOrUser, soql, false)
		if err != nil {
			return ErrMsg{Err: err}
		}
		var fp struct {
			Result struct {
				Records []map[string]any `json:"records"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &fp); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse field perms: %w", err)}
		}

		byField := map[string]*FieldPerm{}
		for _, rec := range fp.Result.Records {
			full := stringField(rec, "Field") // e.g. "Account.Industry"
			name := full
			if i := strings.IndexByte(full, '.'); i >= 0 {
				name = full[i+1:]
			}
			if name == "" {
				continue
			}
			existing, ok := byField[name]
			if !ok {
				existing = &FieldPerm{Field: name}
				byField[name] = existing
			}
			granted := false
			if boolField(rec, "PermissionsRead") {
				existing.Read = true
				granted = true
			}
			if boolField(rec, "PermissionsEdit") {
				existing.Edit = true
				granted = true
			}
			if granted {
				if label := parentLabels[stringField(rec, "ParentId")]; label != "" && !contains(existing.Sources, label) {
					existing.Sources = append(existing.Sources, label)
				}
			}
		}
		fields := make([]FieldPerm, 0, len(byField))
		for _, v := range byField {
			fields = append(fields, *v)
		}
		sort.Slice(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })

		return FieldPermissionsLoadedMsg{User: user, SObject: sobject, Fields: fields}
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
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%s", string(ee.Stderr))
		}
		return nil, fmt.Errorf("sf data query: %w", err)
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
