package sf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const whyFixtures = "../testdata/why"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(whyFixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixtureRecords(t *testing.T, name string) []QueryRecord {
	t.Helper()
	var r struct {
		Records []QueryRecord `json:"records"`
	}
	if err := json.Unmarshal(fixture(t, name), &r); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return r.Records
}

func fixtureMetadata(t *testing.T, name string) json.RawMessage {
	t.Helper()
	return rawOf(fixtureRecords(t, name)[0]["Metadata"])
}

type route struct {
	match   string
	tooling bool
	file    string
}

type fakeAPI struct {
	t          *testing.T
	routes     []route
	gets       map[string]string
	toolingErr error
}

func (f *fakeAPI) Query(soql string, tooling bool) ([]QueryRecord, error) {
	if tooling && f.toolingErr != nil {
		return nil, f.toolingErr
	}
	for _, r := range f.routes {
		if r.tooling == tooling && strings.Contains(soql, r.match) {
			return fixtureRecords(f.t, r.file), nil
		}
	}
	f.t.Errorf("unexpected query (tooling=%v): %s", tooling, soql)
	return nil, nil
}

func (f *fakeAPI) GetJSON(path string, v any) error {
	file, ok := f.gets[path]
	if !ok {
		f.t.Errorf("unexpected GET %s", path)
		return &APIError{Status: 404, Message: "not found"}
	}
	return json.Unmarshal(fixture(f.t, file), v)
}

func accountAPI(t *testing.T) *fakeAPI {
	return &fakeAPI{
		t: t,
		gets: map[string]string{
			"/sobjects":                         "globaldescribe.json",
			"/sobjects/AccountHistory/describe": "describe_accounthistory.json",
		},
		routes: []route{
			{"FROM EntityDefinition", false, "entitydefinition_account.json"},
			{"SELECT Id FROM Account WHERE Id", false, "record_account.json"},
			{"FROM FieldDefinition", false, "tracked_account.json"},
			{"FROM AccountHistory", false, "history_account.json"},
			{"FROM User WHERE Id IN", false, "users.json"},
			{"FROM FlowDefinitionView WHERE TriggerObjectOrEventId", false, "flows_account.json"},
			{"FROM FlowDefinitionView WHERE ProcessType = 'Workflow'", false, "processes.json"},
			{"FROM ApexTrigger", false, "triggers_account.json"},
			{"FROM Flow WHERE Id = '301000000000001AAA'", true, "flow_before_save.json"},
			{"FROM Flow WHERE Id = '301000000000002AAA'", true, "flow_after_save.json"},
			{"FROM Flow WHERE Id = '301000000000003AAA'", true, "pb_update_records.json"},
			{"FROM Flow WHERE Id = '301000000000004AAA'", true, "flow_record_variable.json"},
			{"FROM Flow WHERE Id = '301000000000009AAA'", true, "pb_contact.json"},
			{"FROM Flow WHERE Id", true, "flow_empty.json"},
			{"FROM ValidationRule", true, "validationrules_account.json"},
			{"FROM WorkflowRule WHERE TableEnumOrId", true, "workflowrules_account.json"},
			{"FROM WorkflowRule WHERE Id", true, "workflowrule_meta.json"},
			{"FROM WorkflowFieldUpdate WHERE SourceTableEnumOrId", true, "workflowfieldupdates_account.json"},
			{"FROM WorkflowFieldUpdate WHERE Id", true, "workflowfieldupdate_meta.json"},
		},
	}
}

func TestHistoryObjectAndParentField(t *testing.T) {
	cases := []struct {
		obj, hist, describe, parent string
	}{
		{"Account", "AccountHistory", "describe_accounthistory.json", "AccountId"},
		{"Opportunity", "OpportunityFieldHistory", "describe_opportunityfieldhistory.json", "OpportunityId"},
		{"Foo__c", "Foo__History", "describe_foo__history.json", "ParentId"},
		{"Case", "CaseHistory", "describe_casehistory.json", "CaseId"},
	}
	for _, c := range cases {
		t.Run(c.obj, func(t *testing.T) {
			if got := HistoryObjectName(c.obj); got != c.hist {
				t.Errorf("HistoryObjectName(%s) = %s, want %s", c.obj, got, c.hist)
			}
			var d Describe
			if err := json.Unmarshal(fixture(t, c.describe), &d); err != nil {
				t.Fatal(err)
			}
			if got := HistoryParentField(d, c.obj); got != c.parent {
				t.Errorf("HistoryParentField = %s, want %s", got, c.parent)
			}
		})
	}
}

func TestGroupHistory(t *testing.T) {
	tracked := []string{"Description", "Industry", "OwnerId", "Rating"}
	saves := GroupHistory(fixtureRecords(t, "history_account.json"), tracked)

	if len(saves) != 4 {
		t.Fatalf("want 4 saves, got %d", len(saves))
	}
	type want struct {
		by     string
		fields []string
	}
	wants := []want{
		{"005000000000001AAA", []string{"Owner", "Industry", "Rating", "Description"}},
		{"005000000000002AAA", []string{"Rating"}},
		{"005000000000003AAA", []string{"Industry"}},
		{"005000000000001AAA", []string{"created", "Rating"}},
	}
	for i, w := range wants {
		s := saves[i]
		if s.CreatedByID != w.by {
			t.Errorf("save %d by %s, want %s", i, s.CreatedByID, w.by)
		}
		var got []string
		for _, c := range s.Changes {
			got = append(got, c.Field)
		}
		if !reflect.DeepEqual(got, w.fields) {
			t.Errorf("save %d fields %v, want %v", i, got, w.fields)
		}
	}
	if !saves[0].At.After(saves[1].At) {
		t.Error("saves should be newest first")
	}

	owner := saves[0].Changes[0]
	if owner.Old != "Alex Admin" || owner.New != "Sam Seller" {
		t.Errorf("lookup change should show names, got %v → %v", owner.Old, owner.New)
	}
	if owner.OldID != "005000000000001AAA" || owner.NewID != "005000000000004AAA" {
		t.Errorf("lookup change should keep Ids, got %s → %s", owner.OldID, owner.NewID)
	}
	if owner.CanonicalField != "OwnerId" {
		t.Errorf("Owner should canonicalize to OwnerId, got %s", owner.CanonicalField)
	}

	created := saves[3].Changes[0]
	if !created.Created || created.Label() != "Record created" {
		t.Errorf("created row: %+v", created)
	}
}

func TestGroupHistoryLookupEntityIDFirst(t *testing.T) {
	rows := []QueryRecord{
		{"Field": "Owner", "DataType": "EntityId", "OldValue": "005A", "NewValue": "005B", "CreatedById": "005X", "CreatedDate": "2026-09-20T14:02:11.000+0000"},
		{"Field": "Owner", "DataType": "Text", "OldValue": "Ann", "NewValue": "Bob", "CreatedById": "005X", "CreatedDate": "2026-09-20T14:02:11.000+0000"},
	}
	saves := GroupHistory(rows, nil)
	if len(saves) != 1 || len(saves[0].Changes) != 1 {
		t.Fatalf("want one save with one change, got %+v", saves)
	}
	c := saves[0].Changes[0]
	if c.Old != "Ann" || c.New != "Bob" || c.OldID != "005A" || c.NewID != "005B" {
		t.Errorf("collapse failed: %+v", c)
	}
}

func TestCanonicalField(t *testing.T) {
	tracked := []string{"ContactId", "OwnerId", "Status", "Referral__c"}
	cases := map[string]string{
		"Owner":       "OwnerId",
		"Contact":     "ContactId",
		"Status":      "Status",
		"status":      "Status",
		"Referral__c": "Referral__c",
		"Unknown":     "Unknown",
	}
	for in, want := range cases {
		if got := CanonicalField(in, tracked); got != want {
			t.Errorf("CanonicalField(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestFlowWrites(t *testing.T) {
	cases := []struct {
		name      string
		file      string
		want      []FieldWrite
		recordVar bool
	}{
		{"before-save $Record assignment", "flow_before_save.json", []FieldWrite{{Field: "Rating"}}, false},
		{"after-save update $Record", "flow_after_save.json", []FieldWrite{{Field: "Description"}}, false},
		{"process builder update records by object", "pb_update_records.json", []FieldWrite{{Field: "Rating", OtherRecords: true}}, false},
		{"record variable update", "flow_record_variable.json", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, rv, err := FlowWrites(fixtureMetadata(t, c.file), "Account")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("writes = %+v, want %+v", got, c.want)
			}
			if rv != c.recordVar {
				t.Errorf("recordVar = %v, want %v", rv, c.recordVar)
			}
		})
	}
}

func TestProcessTargets(t *testing.T) {
	if ok, trig := ProcessTargets(fixtureMetadata(t, "pb_update_records.json"), "Account"); !ok || trig != "onAllChanges" {
		t.Errorf("Account process: ok=%v trig=%s", ok, trig)
	}
	if ok, _ := ProcessTargets(fixtureMetadata(t, "pb_contact.json"), "Account"); ok {
		t.Error("Contact process should not target Account")
	}
}

func TestWorkflowFieldUpdate(t *testing.T) {
	active, events, updates, err := WorkflowRuleInfo(fixtureMetadata(t, "workflowrule_meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !active || !reflect.DeepEqual(events, []string{"C", "U"}) || !reflect.DeepEqual(updates, []string{"Set_Rating_Cold"}) {
		t.Errorf("rule: active=%v events=%v updates=%v", active, events, updates)
	}
	field, err := WorkflowFieldUpdateTarget(fixtureMetadata(t, "workflowfieldupdate_meta.json"))
	if err != nil || field != "Rating" {
		t.Errorf("field update target = %q, %v", field, err)
	}
}

func TestSectionOrderAndFlowSort(t *testing.T) {
	isolateConfig(t)
	p := LoadAutomation(accountAPI(t), "o", ObjectRef{Name: "Account", DurableID: "Account"}, false)
	if len(p.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", p.Errors)
	}
	var keys []string
	for _, s := range p.Sections {
		keys = append(keys, s.Key)
	}
	want := []string{SecBeforeFlows, SecBeforeTriggers, SecValidation, SecAfterTriggers, SecAssignment, SecWorkflow, SecProcessBuilder, SecAfterFlows}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("section order %v, want %v", keys, want)
	}
	names := func(s AutomationSection) []string {
		var out []string
		for _, it := range s.Items {
			out = append(out, it.Label+it.Name)
		}
		return out
	}
	// TriggerOrder 10 tie broken by label; unset order sorts last.
	if got := names(p.Sections[0]); !reflect.DeepEqual(got, []string{
		"Account Before: RatingAccount_Before_Rating", "Zeta BeforeAccount_Before_Zeta", "Alpha BeforeAccount_Before_Alpha"}) {
		t.Errorf("before-save order %v", got)
	}
	if got := names(p.Sections[7]); len(got) != 3 || !strings.HasPrefix(got[0], "Account After: Parent") {
		t.Errorf("after-save order %v", got)
	}
	if n := len(p.Sections[6].Items); n != 1 {
		t.Errorf("process builder should keep only the Account process, got %d", n)
	}
	if got := p.Sections[1].Items; len(got) != 2 {
		t.Errorf("before triggers: %+v", got)
	}
	if got := p.Sections[3].Items; len(got) != 2 {
		t.Errorf("after triggers: %+v", got)
	}
	if got := p.Sections[5].Items; len(got) != 1 || !reflect.DeepEqual(got[0].Writes, []FieldWrite{{Field: "Rating"}}) {
		t.Errorf("workflow: %+v", got)
	}
	if got := p.Sections[1].Items[0].Events; !reflect.DeepEqual(got, []string{"C"}) {
		t.Errorf("AccountBeforeInsert events %v", got)
	}
}

func TestClassifyUser(t *testing.T) {
	cases := []struct {
		userType, profile, license, want string
	}{
		{"AutomatedProcess", "", "", BadgeAuto},
		{"Guest", "Site Guest Profile", "Guest User License", BadgeGuest},
		{"Standard", "Analytics Cloud Integration User", "Analytics Cloud Integration User", BadgeInteg},
		{"Standard", "Minimum Access - API Only Integrations", "Salesforce Integration", BadgeInteg},
		{"Standard", "Custom: API only", "Salesforce", BadgeInteg},
		{"Standard", "System Administrator", "Salesforce", ""},
	}
	for _, c := range cases {
		if got := ClassifyUser(c.userType, c.profile, c.license); got != c.want {
			t.Errorf("ClassifyUser(%s, %s, %s) = %q, want %q", c.userType, c.profile, c.license, got, c.want)
		}
	}
}

func TestBuildTraceAttribution(t *testing.T) {
	isolateConfig(t)
	tr, err := BuildTrace(accountAPI(t), "o", "001000000000001", false)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Record.ID != "001000000000001AAA" || tr.Object.Name != "Account" || tr.Object.HistoryObject != "AccountHistory" {
		t.Fatalf("resolve: %+v %+v", tr.Record, tr.Object)
	}
	if len(tr.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", tr.Errors)
	}
	writers := map[string][]string{}
	for _, c := range tr.Saves[0].Changes {
		for _, w := range c.Writers {
			writers[c.Field] = append(writers[c.Field], w.Name+"|"+w.Note)
		}
	}
	has := func(field, want string) {
		t.Helper()
		for _, w := range writers[field] {
			if w == want {
				return
			}
		}
		t.Errorf("%s writers %v missing %q", field, writers[field], want)
	}
	has("Rating", "Account Before: Rating|")
	has("Rating", "Cold Rating Rule|")
	has("Rating", "Account Process|may target other records")
	has("Description", "Account After: Description|")
	has("Description", "AccountAfterUpdate|possible — not attributed")
	has("Owner", "Account After: Parent|updates a record variable — not attributed")
	for _, w := range writers["Industry"] {
		if strings.HasPrefix(w, "Account Before: Rating") || strings.HasPrefix(w, "AccountLegacy") || strings.HasPrefix(w, "AccountBeforeInsert") {
			t.Errorf("Industry wrongly attributed to %s", w)
		}
	}
	if u := tr.Saves[1].User; u == nil || u.Badge != BadgeAuto || tr.Saves[1].Hint == "" {
		t.Errorf("AUTO save: %+v hint=%q", u, tr.Saves[1].Hint)
	}
	if u := tr.Saves[2].User; u == nil || u.Badge != BadgeInteg || tr.Saves[2].Hint != "external system via API" {
		t.Errorf("INTEG save: %+v", u)
	}
}

func TestBuildTraceToolingDenied(t *testing.T) {
	isolateConfig(t)
	api := accountAPI(t)
	api.toolingErr = &APIError{Status: 403, Code: "INSUFFICIENT_ACCESS", Message: "Tooling API access denied"}
	tr, err := BuildTrace(api, "o", "001000000000001AAA", false)
	if err != nil {
		t.Fatalf("a Tooling failure must not fail the trace: %v", err)
	}
	if len(tr.Saves) != 4 || len(tr.Saves[0].Changes) != 4 {
		t.Fatalf("history should still render, got %d saves", len(tr.Saves))
	}
	for _, sec := range []string{SecBeforeFlows, SecAfterFlows, SecValidation, SecWorkflow, SecProcessBuilder} {
		if !strings.Contains(tr.ErrorFor(sec), "403") {
			t.Errorf("section %s should carry the 403, got %q", sec, tr.ErrorFor(sec))
		}
	}
	for _, sec := range []string{"history", "tracked", "users", SecBeforeTriggers, SecAfterTriggers} {
		if e := tr.ErrorFor(sec); e != "" {
			t.Errorf("section %s should be unaffected, got %q", sec, e)
		}
	}
	if len(tr.Automation[0].Items) == 0 {
		t.Error("flows should still be listed when their metadata is denied")
	}
	if _, ok := readAutomationCache("o", "Account"); ok {
		t.Error("an inventory with errors must not be cached")
	}
}

func TestResolveNotFoundAndNoHistory(t *testing.T) {
	isolateConfig(t)
	api := accountAPI(t)
	api.routes = append([]route{{"SELECT Id FROM Account WHERE Id", false, "empty.json"}}, api.routes...)
	if _, err := BuildTrace(api, "o", "001000000000001AAA", false); err != ErrRecordNotFound {
		t.Errorf("want ErrRecordNotFound, got %v", err)
	}

	api = accountAPI(t)
	api.gets["/sobjects"] = "globaldescribe.json"
	api.routes = append([]route{
		{"FROM EntityDefinition", false, "entitydefinition_custom.json"},
		{"SELECT Id FROM Bar__c WHERE Id", false, "record_account.json"},
	}, api.routes...)
	tr, err := ResolveRecord(api, "o", "a0Y000000000001AAA", false)
	if err != nil {
		t.Fatal(err)
	}
	p := LoadHistory(api, "o", tr, false)
	if len(p.Errors) != 1 || p.Errors[0].Message != "History tracking isn't enabled on Bar__c." {
		t.Errorf("no-history message: %+v", p.Errors)
	}
}

func TestObjKeysIncludesDurableID(t *testing.T) {
	if got := objKeys(ObjectRef{Name: "Foo__c", DurableID: "01I000000000001"}); got != "'Foo__c','01I000000000001'" {
		t.Errorf("custom keys = %s", got)
	}
	if got := objKeys(ObjectRef{Name: "Account", DurableID: "Account"}); got != "'Account'" {
		t.Errorf("standard keys = %s", got)
	}
}

func TestAutomationCacheRoundTrip(t *testing.T) {
	isolateConfig(t)
	api := accountAPI(t)
	first := LoadAutomation(api, "o", ObjectRef{Name: "Account", DurableID: "Account"}, false)
	api.routes = nil // any query now would fail the test
	second := LoadAutomation(api, "o", ObjectRef{Name: "Account", DurableID: "Account"}, false)
	if !reflect.DeepEqual(first.Sections, second.Sections) {
		t.Error("cached inventory differs from the live one")
	}
}
