package sf

import (
	"fmt"
	"sort"
	"strings"
)

// Pure builders that turn decoded SOQL records into report pieces. No I/O.

// sourceLabel is the label a permission grant is attributed to: the
// permission set's label, or "Profile: <name>" for a profile-owned set.
func sourceLabel(a PermSetAssignment) string {
	if a.IsProfile && a.ProfileName != "" {
		return "Profile: " + a.ProfileName
	}
	return a.PermSetLabel
}

// buildAssignments decodes PermissionSetAssignment records (with the
// PermissionSet and PermissionSetGroup relationships selected) and collects
// the system-wide overrides granted by any of them.
func buildAssignments(recs []map[string]any) ([]PermSetAssignment, Overrides) {
	out := make([]PermSetAssignment, 0, len(recs))
	ov := Overrides{ModifyAllData: []string{}, ViewAllData: []string{}}
	for _, rec := range recs {
		as := PermSetAssignment{PermSetID: stringField(rec, "PermissionSetId")}
		if as.PermSetID == "" {
			continue
		}
		var mad, vad bool
		if ps, ok := rec["PermissionSet"].(map[string]any); ok {
			as.PermSetLabel = stringField(ps, "Label")
			as.IsProfile = boolField(ps, "IsOwnedByProfile")
			if prof, ok := ps["Profile"].(map[string]any); ok {
				as.ProfileName = stringField(prof, "Name")
			}
			mad = boolField(ps, "PermissionsModifyAllData")
			vad = boolField(ps, "PermissionsViewAllData")
		}
		if g, ok := rec["PermissionSetGroup"].(map[string]any); ok {
			as.GroupLabel = stringField(g, "MasterLabel")
		}
		label := sourceLabel(as)
		if mad && !contains(ov.ModifyAllData, label) {
			ov.ModifyAllData = append(ov.ModifyAllData, label)
		}
		if vad && !contains(ov.ViewAllData, label) {
			ov.ViewAllData = append(ov.ViewAllData, label)
		}
		out = append(out, as)
	}
	return out, ov
}

// labelsOf maps PermSetID → source label for grant attribution.
func labelsOf(as []PermSetAssignment) map[string]string {
	m := make(map[string]string, len(as))
	for _, a := range as {
		m[a.PermSetID] = sourceLabel(a)
	}
	return m
}

// Permission keys used in ObjectPerm.Grants and FieldPerm.Grants.
const (
	PermRead      = "read"
	PermCreate    = "create"
	PermEdit      = "edit"
	PermDelete    = "delete"
	PermViewAll   = "viewAll"
	PermModifyAll = "modifyAll"
)

var objectPermKeys = []string{PermRead, PermCreate, PermEdit, PermDelete, PermViewAll, PermModifyAll}
var fieldPermKeys = []string{PermRead, PermEdit}

func emptyGrants(keys []string) map[string][]string {
	m := make(map[string][]string, len(keys))
	for _, k := range keys {
		m[k] = []string{}
	}
	return m
}

// grant records that label grants perm, deduped, and marks any-grant in sources.
func grant(grants map[string][]string, sources *[]string, perm, label string) {
	if label == "" {
		return
	}
	if !contains(grants[perm], label) {
		grants[perm] = append(grants[perm], label)
	}
	if !contains(*sources, label) {
		*sources = append(*sources, label)
	}
}

// buildObjectPerms folds ObjectPermissions records into one ObjectPerm per
// sobject with per-permission sources, sorted by sobject name.
func buildObjectPerms(recs []map[string]any, labels map[string]string) []ObjectPerm {
	byObject := map[string]*ObjectPerm{}
	for _, rec := range recs {
		name := stringField(rec, "SobjectType")
		if name == "" {
			continue
		}
		op, ok := byObject[name]
		if !ok {
			op = &ObjectPerm{SObject: name, Sources: []string{}, Grants: emptyGrants(objectPermKeys)}
			byObject[name] = op
		}
		label := labels[stringField(rec, "ParentId")]
		for _, f := range []struct {
			col  string
			perm string
			flag *bool
		}{
			{"PermissionsRead", PermRead, &op.Read},
			{"PermissionsCreate", PermCreate, &op.Create},
			{"PermissionsEdit", PermEdit, &op.Edit},
			{"PermissionsDelete", PermDelete, &op.Delete},
			{"PermissionsViewAllRecords", PermViewAll, &op.ViewAll},
			{"PermissionsModifyAllRecords", PermModifyAll, &op.ModifyAll},
		} {
			if boolField(rec, f.col) {
				*f.flag = true
				grant(op.Grants, &op.Sources, f.perm, label)
			}
		}
	}
	out := make([]ObjectPerm, 0, len(byObject))
	for _, v := range byObject {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SObject < out[j].SObject })
	return out
}

// buildFieldPerms folds FieldPermissions records ("Obj.Field") into one
// FieldPerm per field with per-permission sources, sorted by field name.
func buildFieldPerms(recs []map[string]any, labels map[string]string) []FieldPerm {
	byField := map[string]*FieldPerm{}
	for _, rec := range recs {
		name := stringField(rec, "Field")
		if i := strings.IndexByte(name, '.'); i >= 0 {
			name = name[i+1:]
		}
		if name == "" {
			continue
		}
		fp, ok := byField[name]
		if !ok {
			fp = &FieldPerm{Field: name, Sources: []string{}, Grants: emptyGrants(fieldPermKeys)}
			byField[name] = fp
		}
		label := labels[stringField(rec, "ParentId")]
		if boolField(rec, "PermissionsRead") {
			fp.Read = true
			grant(fp.Grants, &fp.Sources, PermRead, label)
		}
		if boolField(rec, "PermissionsEdit") {
			fp.Edit = true
			grant(fp.Grants, &fp.Sources, PermEdit, label)
		}
	}
	out := make([]FieldPerm, 0, len(byField))
	for _, v := range byField {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

const (
	assignmentsSOQL = "SELECT PermissionSetId, PermissionSet.Label, PermissionSet.IsOwnedByProfile, PermissionSet.Profile.Name, " +
		"PermissionSet.PermissionsModifyAllData, PermissionSet.PermissionsViewAllData, " +
		"PermissionSetGroupId, PermissionSetGroup.MasterLabel " +
		"FROM PermissionSetAssignment WHERE AssigneeId = '%s'"
	assignmentsSOQLNoGroup = "SELECT PermissionSetId, PermissionSet.Label, PermissionSet.IsOwnedByProfile, PermissionSet.Profile.Name, " +
		"PermissionSet.PermissionsModifyAllData, PermissionSet.PermissionsViewAllData " +
		"FROM PermissionSetAssignment WHERE AssigneeId = '%s'"
	objectPermsSOQL = "SELECT SobjectType, PermissionsRead, PermissionsCreate, PermissionsEdit, PermissionsDelete, " +
		"PermissionsViewAllRecords, PermissionsModifyAllRecords, ParentId " +
		"FROM ObjectPermissions WHERE ParentId IN (%s)"
	fieldPermsSOQL = "SELECT Field, PermissionsRead, PermissionsEdit, ParentId " +
		"FROM FieldPermissions WHERE SobjectType = '%s' AND ParentId IN (%s)"
)

func queryRecords(q SOQLRunner, org, soql string) ([]map[string]any, error) {
	out, err := q.SOQL(org, soql, false)
	if err != nil {
		return nil, err
	}
	return decodeRecords(out)
}

// loadAssignments runs the assignments query, retrying without the
// permission-set-group columns when the org's API rejects them.
func loadAssignments(q SOQLRunner, org string, user UserBrief) ([]map[string]any, error) {
	recs, err := queryRecords(q, org, fmt.Sprintf(assignmentsSOQL, soqlEscape(user.ID)))
	if err != nil && strings.Contains(err.Error(), "PermissionSetGroup") {
		recs, err = queryRecords(q, org, fmt.Sprintf(assignmentsSOQLNoGroup, soqlEscape(user.ID)))
	}
	return recs, err
}

// BuildPermissionsReport assembles the report for user. The error is non-nil
// only when the assignments cannot be loaded; object and field permission
// failures are recorded in Errors and the rest of the report is returned.
func BuildPermissionsReport(q SOQLRunner, org string, user UserBrief, object string) (PermissionsReport, error) {
	r := PermissionsReport{
		User:        user,
		Assignments: []PermSetAssignment{},
		Overrides:   Overrides{ModifyAllData: []string{}, ViewAllData: []string{}},
		Objects:     []ObjectPerm{},
		Object:      object,
		Fields:      []FieldPerm{},
		Errors:      []SectionError{},
	}
	recs, err := loadAssignments(q, org, user)
	if err != nil {
		return r, fmt.Errorf("assignments: %w", err)
	}
	r.Assignments, r.Overrides = buildAssignments(recs)
	if len(r.Assignments) == 0 {
		return r, nil
	}
	labels := labelsOf(r.Assignments)
	ids := make([]string, 0, len(r.Assignments))
	for _, a := range r.Assignments {
		ids = append(ids, "'"+soqlEscape(a.PermSetID)+"'")
	}
	in := strings.Join(ids, ",")

	if recs, err := queryRecords(q, org, fmt.Sprintf(objectPermsSOQL, in)); err != nil {
		r.Errors = append(r.Errors, SectionError{Section: "objects", Message: err.Error()})
	} else {
		r.Objects = buildObjectPerms(recs, labels)
	}
	if object != "" {
		if recs, err := queryRecords(q, org, fmt.Sprintf(fieldPermsSOQL, soqlEscape(object), in)); err != nil {
			r.Errors = append(r.Errors, SectionError{Section: "fields", Message: err.Error()})
		} else {
			r.Fields = buildFieldPerms(recs, labels)
		}
	}
	return r, nil
}
