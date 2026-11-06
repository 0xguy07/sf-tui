package panes

import (
	"strings"
	"testing"

	"github.com/0xguy07/sf-tui/sf"
)

func accountPerm() sf.ObjectPerm {
	return sf.ObjectPerm{
		SObject: "Account", Read: true, Create: true, Edit: true,
		Sources: []string{"Profile: Sales User", "Sales Ops"},
		Grants: map[string][]string{
			sf.PermRead: {"Profile: Sales User"}, sf.PermCreate: {"Sales Ops"}, sf.PermEdit: {"Sales Ops"},
			sf.PermDelete: {}, sf.PermViewAll: {}, sf.PermModifyAll: {},
		},
	}
}

func TestGrantSummary(t *testing.T) {
	got := grantSummary(accountPerm().Grants, objectPermOrder, objectPermLetters)
	if got != "R:Profile: Sales User  C,E:Sales Ops" {
		t.Errorf("got %q", got)
	}
	empty := map[string][]string{sf.PermRead: {}, sf.PermCreate: {}, sf.PermEdit: {}, sf.PermDelete: {}, sf.PermViewAll: {}, sf.PermModifyAll: {}}
	if got := grantSummary(empty, objectPermOrder, objectPermLetters); got != "" {
		t.Errorf("empty grants should render as \"\", got %q", got)
	}
}

func permsMsg(ov sf.Overrides) sf.PermissionsLoadedMsg {
	return sf.PermissionsLoadedMsg{
		User:        sf.UserBrief{Name: "Alex Admin", Username: "alex@acme.example", ProfileName: "System Administrator"},
		Assignments: []sf.PermSetAssignment{{PermSetID: "0PS2", PermSetLabel: "Sales Ops"}},
		Objects:     []sf.ObjectPerm{accountPerm()},
		Overrides:   ov,
	}
}

func TestRenderPermDetail_Overrides(t *testing.T) {
	with := renderPermDetail(permsMsg(sf.Overrides{ModifyAllData: []string{"Data Steward"}}), "", nil)
	if !strings.Contains(with, "System overrides") || !strings.Contains(with, "Modify All Data: Data Steward") {
		t.Errorf("overrides section missing:\n%s", with)
	}
	without := renderPermDetail(permsMsg(sf.Overrides{}), "", nil)
	if strings.Contains(without, "System overrides") {
		t.Error("overrides section should be omitted when empty")
	}
}

func TestRenderPermDetail_PerPermissionSources(t *testing.T) {
	out := renderPermDetail(permsMsg(sf.Overrides{}), "", nil)
	if !strings.Contains(out, "C,E:Sales Ops") {
		t.Errorf("per-permission sources missing:\n%s", out)
	}
}
