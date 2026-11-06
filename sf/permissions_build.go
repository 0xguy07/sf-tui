package sf

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
