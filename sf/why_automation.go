package sf

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
)

const (
	SecBeforeFlows     = "before_flows"
	SecBeforeTriggers  = "before_triggers"
	SecValidation      = "validation_rules"
	SecAfterTriggers   = "after_triggers"
	SecAssignment      = "assignment_rules"
	SecWorkflow        = "workflow_rules"
	SecProcessBuilder  = "process_builder"
	SecAfterFlows      = "after_flows"
	TriggerOrderFooter = "Order among triggers on the same event isn't guaranteed."
)

// SectionOrder is Salesforce's order of execution for a save.
var SectionOrder = []struct{ Key, Title, Note string }{
	{SecBeforeFlows, "Before-save flows", ""},
	{SecBeforeTriggers, "Before triggers", ""},
	{SecValidation, "Validation rules", ""},
	{SecAfterTriggers, "After triggers", ""},
	{SecAssignment, "Assignment rules", ""},
	{SecWorkflow, "Workflow rules", "field updates re-fire before/after update triggers once"},
	{SecProcessBuilder, "Process Builder", ""},
	{SecAfterFlows, "After-save flows", "async paths run after commit"},
}

type AutomationSection struct {
	Key   string           `json:"key"`
	Title string           `json:"title"`
	Note  string           `json:"note,omitempty"`
	Items []AutomationItem `json:"items"`
}

const (
	KindFlow           = "flow"
	KindProcessBuilder = "process_builder"
	KindTrigger        = "trigger"
	KindValidation     = "validation_rule"
	KindAssignment     = "assignment_rule"
	KindWorkflow       = "workflow_rule"
)

type AutomationItem struct {
	Kind                  string       `json:"kind"`
	ID                    string       `json:"id,omitempty"`
	Name                  string       `json:"name"`
	Label                 string       `json:"label,omitempty"`
	Namespace             string       `json:"namespace,omitempty"`
	Active                bool         `json:"active"`
	Events                []string     `json:"events,omitempty"`
	TriggerOrder          *int         `json:"triggerOrder,omitempty"`
	VersionID             string       `json:"versionId,omitempty"`
	Writes                []FieldWrite `json:"writes,omitempty"`
	UpdatesRecordVariable bool         `json:"updatesRecordVariable,omitempty"`
	WritesUpdate          bool         `json:"writesUpdate,omitempty"`
	Detail                string       `json:"detail,omitempty"`
}

func (a AutomationItem) Display() string {
	name := a.Label
	if name == "" {
		name = a.Name
	}
	if a.Namespace != "" {
		name += " (" + a.Namespace + ")"
	}
	return name
}

type FieldWrite struct {
	Field        string `json:"field"`
	OtherRecords bool   `json:"otherRecords,omitempty"`
}

// AutomationPart is the automation half of a Trace.
type AutomationPart struct {
	Sections []AutomationSection
	Errors   []SectionError
}

const metadataConcurrency = 6

func objKeys(o ObjectRef) string {
	keys := []string{"'" + soqlEscape(o.Name) + "'"}
	if o.DurableID != "" && !strings.EqualFold(o.DurableID, o.Name) {
		keys = append(keys, "'"+soqlEscape(o.DurableID)+"'")
	}
	return strings.Join(keys, ",")
}

// LoadAutomation lists every automation on the object, served from a 24h disk
// cache unless force is set. Only error-free inventories are cached.
func LoadAutomation(api whyAPI, org string, o ObjectRef, force bool) AutomationPart {
	if !force {
		if p, ok := readAutomationCache(org, o.Name); ok {
			return p
		}
	}
	sem := make(chan struct{}, metadataConcurrency)
	items := map[string][]AutomationItem{}
	var errs []SectionError
	var mu sync.Mutex
	put := func(sec string, its []AutomationItem, err error, errSections ...string) {
		mu.Lock()
		defer mu.Unlock()
		items[sec] = append(items[sec], its...)
		if err != nil {
			if len(errSections) == 0 {
				errSections = []string{sec}
			}
			for _, s := range errSections {
				errs = append(errs, SectionError{Section: s, Message: err.Error()})
			}
		}
	}

	var wg sync.WaitGroup
	run := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }
	run(func() {
		before, after, err := loadRecordFlows(api, org, o, sem)
		put(SecBeforeFlows, before, nil)
		put(SecAfterFlows, after, err, SecBeforeFlows, SecAfterFlows)
	})
	run(func() {
		its, err := loadProcessBuilders(api, org, o, sem)
		put(SecProcessBuilder, its, err)
	})
	run(func() {
		before, after, err := loadTriggers(api, o)
		put(SecBeforeTriggers, before, nil)
		put(SecAfterTriggers, after, err, SecBeforeTriggers, SecAfterTriggers)
	})
	run(func() {
		its, err := loadValidationRules(api, o)
		put(SecValidation, its, err)
	})
	run(func() {
		its, err := loadAssignmentRules(api, o)
		put(SecAssignment, its, err)
	})
	run(func() {
		its, err := loadWorkflowRules(api, o, sem)
		put(SecWorkflow, its, err)
	})
	wg.Wait()

	p := AutomationPart{Sections: buildSections(items), Errors: errs}
	sort.SliceStable(p.Errors, func(i, j int) bool { return sectionRank(p.Errors[i].Section) < sectionRank(p.Errors[j].Section) })
	if len(p.Errors) == 0 {
		writeAutomationCache(org, o.Name, p)
	}
	return p
}

func sectionRank(key string) int {
	for i, s := range SectionOrder {
		if s.Key == key {
			return i
		}
	}
	return len(SectionOrder)
}

func buildSections(items map[string][]AutomationItem) []AutomationSection {
	out := make([]AutomationSection, 0, len(SectionOrder))
	for _, s := range SectionOrder {
		its := append([]AutomationItem{}, items[s.Key]...)
		switch s.Key {
		case SecBeforeFlows, SecAfterFlows:
			SortFlows(its)
		default:
			sort.SliceStable(its, func(i, j int) bool { return strings.ToLower(its[i].Display()) < strings.ToLower(its[j].Display()) })
		}
		out = append(out, AutomationSection{Key: s.Key, Title: s.Title, Note: s.Note, Items: its})
	}
	return out
}

// SortFlows orders by TriggerOrder (unset last), then label.
func SortFlows(its []AutomationItem) {
	sort.SliceStable(its, func(i, j int) bool {
		a, b := its[i].TriggerOrder, its[j].TriggerOrder
		switch {
		case a != nil && b != nil && *a != *b:
			return *a < *b
		case a != nil && b == nil:
			return true
		case a == nil && b != nil:
			return false
		}
		return strings.ToLower(its[i].Display()) < strings.ToLower(its[j].Display())
	})
}

func flowEvents(recordTriggerType string) []string {
	switch recordTriggerType {
	case "Create":
		return []string{"C"}
	case "Update":
		return []string{"U"}
	case "CreateAndUpdate":
		return []string{"C", "U"}
	case "Delete":
		return []string{"D"}
	}
	return nil
}

func workflowEvents(triggerType string) []string {
	switch triggerType {
	case "onCreateOnly":
		return []string{"C"}
	case "onCreateOrTriggeringUpdate", "onAllChanges":
		return []string{"C", "U"}
	}
	return nil
}

func intPtr(v any) *int {
	f, ok := v.(float64)
	if !ok {
		return nil
	}
	i := int(f)
	return &i
}

func boolOf(v any) bool { b, _ := v.(bool); return b }

// --- flows ---

type flowMetadata struct {
	ProcessType string `json:"processType"`
	Assignments []struct {
		AssignmentItems []struct {
			AssignToReference string `json:"assignToReference"`
		} `json:"assignmentItems"`
	} `json:"assignments"`
	RecordUpdates []struct {
		InputReference   string `json:"inputReference"`
		Object           string `json:"object"`
		InputAssignments []struct {
			Field string `json:"field"`
		} `json:"inputAssignments"`
	} `json:"recordUpdates"`
	ProcessMetadataValues []processMetadataValue `json:"processMetadataValues"`
	Variables             []struct {
		Name       string `json:"name"`
		ObjectType string `json:"objectType"`
	} `json:"variables"`
}

type processMetadataValue struct {
	Name  string `json:"name"`
	Value struct {
		StringValue *string `json:"stringValue"`
	} `json:"value"`
}

func (m flowMetadata) pmv(name string) string {
	for _, p := range m.ProcessMetadataValues {
		if p.Name == name && p.Value.StringValue != nil {
			return *p.Value.StringValue
		}
	}
	return ""
}

// FlowWrites extracts the fields a flow or process writes on obj, and whether
// it also updates a record variable it can't attribute.
func FlowWrites(raw json.RawMessage, obj string) (writes []FieldWrite, recordVar bool, err error) {
	var m flowMetadata
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false, err
	}
	idx := map[string]int{}
	add := func(field string, other bool) {
		if field == "" {
			return
		}
		k := lc(field)
		if i, ok := idx[k]; ok {
			writes[i].OtherRecords = writes[i].OtherRecords && other
			return
		}
		idx[k] = len(writes)
		writes = append(writes, FieldWrite{Field: field, OtherRecords: other})
	}
	for _, a := range m.Assignments {
		for _, it := range a.AssignmentItems {
			if f, ok := strings.CutPrefix(it.AssignToReference, "$Record."); ok && !strings.Contains(f, ".") {
				add(f, false)
			}
		}
	}
	for _, u := range m.RecordUpdates {
		switch {
		case u.InputReference == "$Record":
			for _, ia := range u.InputAssignments {
				add(ia.Field, false)
			}
		case u.InputReference == "" && strings.EqualFold(u.Object, obj):
			for _, ia := range u.InputAssignments {
				add(ia.Field, true)
			}
		case u.InputReference != "":
			recordVar = true
		}
	}
	return writes, recordVar, nil
}

// ProcessTargets reports whether a Process Builder process runs on obj, and its trigger type.
func ProcessTargets(raw json.RawMessage, obj string) (bool, string) {
	var m flowMetadata
	if json.Unmarshal(raw, &m) != nil {
		return false, ""
	}
	target := strings.EqualFold(m.pmv("ObjectType"), obj)
	if !target {
		for _, v := range m.Variables {
			if v.Name == "myVariable_current" && strings.EqualFold(v.ObjectType, obj) {
				target = true
			}
		}
	}
	return target, m.pmv("TriggerType")
}

// flowMetadataFor returns a flow version's Metadata, cached on disk forever
// because activated versions are immutable.
func flowMetadataFor(api whyAPI, org, versionID string, sem chan struct{}) (json.RawMessage, error) {
	if raw, ok := readFlowMetaCache(org, versionID); ok {
		return raw, nil
	}
	sem <- struct{}{}
	rows, err := api.Query("SELECT Id, Metadata FROM Flow WHERE Id = '"+soqlEscape(versionID)+"'", true)
	<-sem
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("flow version %s not found", versionID)
	}
	raw, err := json.Marshal(rows[0]["Metadata"])
	if err != nil {
		return nil, err
	}
	writeFlowMetaCache(org, versionID, raw)
	return raw, nil
}

// forEachBounded runs f over n indexes and returns the first error.
func forEachBounded(n int, f func(i int) error) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := f(i); err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	return first
}

func flowItem(r QueryRecord, kind string) AutomationItem {
	return AutomationItem{
		Kind:         kind,
		ID:           str(r["ActiveVersionId"]),
		Name:         str(r["ApiName"]),
		Label:        str(r["Label"]),
		Namespace:    str(r["NamespacePrefix"]),
		Active:       boolOf(r["IsActive"]),
		Events:       flowEvents(str(r["RecordTriggerType"])),
		TriggerOrder: intPtr(r["TriggerOrder"]),
		VersionID:    str(r["ActiveVersionId"]),
	}
}

func loadRecordFlows(api whyAPI, org string, o ObjectRef, sem chan struct{}) (before, after []AutomationItem, err error) {
	rows, err := api.Query("SELECT ApiName, Label, ActiveVersionId, LatestVersionId, ProcessType, TriggerType, RecordTriggerType, TriggerOrder, NamespacePrefix, IsActive FROM FlowDefinitionView WHERE TriggerObjectOrEventId IN ("+objKeys(o)+") AND TriggerType IN ('RecordBeforeSave','RecordAfterSave')", false)
	if err != nil {
		return nil, nil, fmt.Errorf("flows: %w", err)
	}
	all := make([]AutomationItem, len(rows))
	for i, r := range rows {
		all[i] = flowItem(r, KindFlow)
	}
	metaErr := forEachBounded(len(all), func(i int) error {
		it := &all[i]
		if !it.Active || it.VersionID == "" {
			return nil
		}
		raw, err := flowMetadataFor(api, org, it.VersionID, sem)
		if err != nil {
			return fmt.Errorf("flow metadata (%s): %w", it.Name, err)
		}
		it.Writes, it.UpdatesRecordVariable, err = FlowWrites(raw, o.Name)
		return err
	})
	for i, r := range rows {
		if str(r["TriggerType"]) == "RecordBeforeSave" {
			before = append(before, all[i])
		} else {
			after = append(after, all[i])
		}
	}
	return before, after, metaErr
}

func loadProcessBuilders(api whyAPI, org string, o ObjectRef, sem chan struct{}) ([]AutomationItem, error) {
	rows, err := api.Query("SELECT ApiName, Label, ActiveVersionId, LatestVersionId, ProcessType, TriggerType, RecordTriggerType, TriggerOrder, NamespacePrefix, IsActive FROM FlowDefinitionView WHERE ProcessType = 'Workflow' AND IsActive = true", false)
	if err != nil {
		return nil, fmt.Errorf("process builder: %w", err)
	}
	cands := make([]AutomationItem, len(rows))
	keep := make([]bool, len(rows))
	for i, r := range rows {
		cands[i] = flowItem(r, KindProcessBuilder)
	}
	metaErr := forEachBounded(len(cands), func(i int) error {
		it := &cands[i]
		if it.VersionID == "" {
			return nil
		}
		raw, err := flowMetadataFor(api, org, it.VersionID, sem)
		if err != nil {
			return fmt.Errorf("process metadata (%s): %w", it.Name, err)
		}
		ok, trig := ProcessTargets(raw, o.Name)
		if !ok {
			return nil
		}
		keep[i] = true
		it.Events = workflowEvents(trig)
		it.Writes, it.UpdatesRecordVariable, err = FlowWrites(raw, o.Name)
		return err
	})
	var out []AutomationItem
	for i := range cands {
		if keep[i] {
			out = append(out, cands[i])
		}
	}
	return out, metaErr
}

// --- triggers, rules ---

func loadTriggers(api whyAPI, o ObjectRef) (before, after []AutomationItem, err error) {
	rows, err := api.Query("SELECT Id, Name, Status, NamespacePrefix, UsageBeforeInsert, UsageBeforeUpdate, UsageBeforeDelete, UsageAfterInsert, UsageAfterUpdate, UsageAfterDelete, UsageAfterUndelete FROM ApexTrigger WHERE TableEnumOrId IN ("+objKeys(o)+")", false)
	if err != nil {
		return nil, nil, fmt.Errorf("triggers: %w", err)
	}
	for _, r := range rows {
		base := AutomationItem{
			Kind:         KindTrigger,
			ID:           str(r["Id"]),
			Name:         str(r["Name"]),
			Namespace:    str(r["NamespacePrefix"]),
			Active:       str(r["Status"]) == "Active",
			WritesUpdate: boolOf(r["UsageBeforeUpdate"]) || boolOf(r["UsageAfterUpdate"]),
		}
		b := events(boolOf(r["UsageBeforeInsert"]), boolOf(r["UsageBeforeUpdate"]), boolOf(r["UsageBeforeDelete"]))
		a := events(boolOf(r["UsageAfterInsert"]), boolOf(r["UsageAfterUpdate"]), boolOf(r["UsageAfterDelete"]))
		if len(b) > 0 {
			it := base
			it.Events = b
			before = append(before, it)
		}
		if len(a) > 0 || boolOf(r["UsageAfterUndelete"]) {
			it := base
			it.Events = a
			after = append(after, it)
		}
	}
	return before, after, nil
}

func events(c, u, d bool) []string {
	var out []string
	if c {
		out = append(out, "C")
	}
	if u {
		out = append(out, "U")
	}
	if d {
		out = append(out, "D")
	}
	return out
}

func loadValidationRules(api whyAPI, o ObjectRef) ([]AutomationItem, error) {
	rows, err := api.Query("SELECT Id, ValidationName, Active, ErrorMessage, NamespacePrefix FROM ValidationRule WHERE EntityDefinition.QualifiedApiName = '"+soqlEscape(o.Name)+"'", true)
	if err != nil {
		return nil, fmt.Errorf("validation rules: %w", err)
	}
	out := make([]AutomationItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, AutomationItem{
			Kind:      KindValidation,
			ID:        str(r["Id"]),
			Name:      str(r["ValidationName"]),
			Namespace: str(r["NamespacePrefix"]),
			Active:    boolOf(r["Active"]),
			Detail:    str(r["ErrorMessage"]),
		})
	}
	return out, nil
}

func loadAssignmentRules(api whyAPI, o ObjectRef) ([]AutomationItem, error) {
	if o.Name != "Lead" && o.Name != "Case" {
		return nil, nil
	}
	rows, err := api.Query("SELECT Id, Name, Active FROM AssignmentRule WHERE SobjectType = '"+o.Name+"'", false)
	if err != nil {
		return nil, fmt.Errorf("assignment rules: %w", err)
	}
	out := make([]AutomationItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, AutomationItem{Kind: KindAssignment, ID: str(r["Id"]), Name: str(r["Name"]), Active: boolOf(r["Active"])})
	}
	return out, nil
}

type workflowRuleMetadata struct {
	Active      bool   `json:"active"`
	TriggerType string `json:"triggerType"`
	Actions     []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"actions"`
}

type workflowFieldUpdateMetadata struct {
	Field string `json:"field"`
}

// WorkflowRuleInfo parses a WorkflowRule's Metadata.
func WorkflowRuleInfo(raw json.RawMessage) (active bool, events []string, fieldUpdates []string, err error) {
	var m workflowRuleMetadata
	if err := json.Unmarshal(raw, &m); err != nil {
		return false, nil, nil, err
	}
	for _, a := range m.Actions {
		if a.Type == "FieldUpdate" {
			fieldUpdates = append(fieldUpdates, a.Name)
		}
	}
	return m.Active, workflowEvents(m.TriggerType), fieldUpdates, nil
}

// WorkflowFieldUpdateTarget parses a WorkflowFieldUpdate's Metadata.field.
func WorkflowFieldUpdateTarget(raw json.RawMessage) (string, error) {
	var m workflowFieldUpdateMetadata
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	return m.Field, nil
}

func toolingMetadataByID(api whyAPI, sobject, id string, sem chan struct{}, extra string) (QueryRecord, error) {
	sem <- struct{}{}
	defer func() { <-sem }()
	rows, err := api.Query("SELECT Id, "+extra+"Metadata FROM "+sobject+" WHERE Id = '"+soqlEscape(id)+"'", true)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s %s not found", sobject, id)
	}
	return rows[0], nil
}

func rawOf(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func loadWorkflowRules(api whyAPI, o ObjectRef, sem chan struct{}) ([]AutomationItem, error) {
	rows, err := api.Query("SELECT Id, Name, NamespacePrefix FROM WorkflowRule WHERE TableEnumOrId IN ("+objKeys(o)+")", true)
	if err != nil {
		return nil, fmt.Errorf("workflow rules: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make([]AutomationItem, len(rows))
	updates := make([][]string, len(rows))
	for i, r := range rows {
		out[i] = AutomationItem{Kind: KindWorkflow, ID: str(r["Id"]), Name: str(r["Name"]), Namespace: str(r["NamespacePrefix"])}
	}
	firstErr := forEachBounded(len(out), func(i int) error {
		rec, err := toolingMetadataByID(api, "WorkflowRule", out[i].ID, sem, "")
		if err != nil {
			return fmt.Errorf("workflow rule metadata (%s): %w", out[i].Name, err)
		}
		out[i].Active, out[i].Events, updates[i], err = WorkflowRuleInfo(rawOf(rec["Metadata"]))
		return err
	})

	need := false
	for _, u := range updates {
		need = need || len(u) > 0
	}
	if !need {
		return out, firstErr
	}
	fuRows, err := api.Query("SELECT Id, Name FROM WorkflowFieldUpdate WHERE SourceTableEnumOrId IN ("+objKeys(o)+")", true)
	if err != nil {
		return out, fmt.Errorf("workflow field updates: %w", err)
	}
	targets := map[string]string{}
	var tmu sync.Mutex
	fuErr := forEachBounded(len(fuRows), func(i int) error {
		id := str(fuRows[i]["Id"])
		rec, err := toolingMetadataByID(api, "WorkflowFieldUpdate", id, sem, "FullName, ")
		if err != nil {
			return fmt.Errorf("workflow field update metadata: %w", err)
		}
		field, err := WorkflowFieldUpdateTarget(rawOf(rec["Metadata"]))
		if err != nil {
			return err
		}
		name := str(rec["FullName"])
		if _, after, ok := strings.Cut(name, "."); ok {
			name = after
		}
		if name == "" {
			name = str(fuRows[i]["Name"])
		}
		tmu.Lock()
		targets[lc(name)] = field
		tmu.Unlock()
		return nil
	})
	for i := range out {
		for _, u := range updates[i] {
			if f, ok := targets[lc(u)]; ok {
				out[i].Writes = append(out[i].Writes, FieldWrite{Field: f})
			}
		}
	}
	if firstErr == nil {
		firstErr = fuErr
	}
	return out, firstErr
}

// --- attribution ---

// Attribute fills Change.Writers from active automation.
func (t *Trace) Attribute() {
	for si := range t.Saves {
		for ci := range t.Saves[si].Changes {
			c := &t.Saves[si].Changes[ci]
			if c.Created {
				continue
			}
			c.Writers = WritersFor(t.Automation, c.CanonicalField)
		}
	}
}

func WritersFor(sections []AutomationSection, field string) []Writer {
	var out []Writer
	seenTrigger := map[string]bool{}
	for _, s := range sections {
		for _, it := range s.Items {
			if !it.Active {
				continue
			}
			w := Writer{Kind: it.Kind, ID: it.ID, Name: it.Display(), Section: s.Title}
			switch it.Kind {
			case KindTrigger:
				if it.WritesUpdate && !seenTrigger[it.ID] {
					seenTrigger[it.ID] = true
					w.Note = "possible — not attributed"
					out = append(out, w)
				}
				continue
			}
			matched := false
			for _, fw := range it.Writes {
				if strings.EqualFold(fw.Field, field) {
					if fw.OtherRecords {
						w.Note = "may target other records"
					}
					out = append(out, w)
					matched = true
					break
				}
			}
			if !matched && it.UpdatesRecordVariable {
				w.Note = "updates a record variable — not attributed"
				out = append(out, w)
			}
		}
	}
	return out
}

// AutomationPath is the in-org path that opens an automation item.
func AutomationPath(it AutomationItem, obj string) string {
	switch it.Kind {
	case KindFlow:
		if it.VersionID != "" {
			return "/builder_platform_interaction/flowBuilder.app?flowId=" + it.VersionID
		}
		return "/lightning/setup/Flows/home"
	case KindProcessBuilder:
		return "/lightning/setup/ProcessAutomation/home"
	case KindTrigger:
		return "/lightning/setup/ApexTriggers/page?address=" + url.QueryEscape("/"+it.ID)
	case KindValidation:
		return "/lightning/setup/ObjectManager/" + obj + "/ValidationRules/" + it.ID + "/view"
	case KindAssignment:
		if obj == "Case" {
			return "/lightning/setup/CaseRules/page?address=" + url.QueryEscape("/"+it.ID)
		}
		return "/lightning/setup/LeadRules/page?address=" + url.QueryEscape("/"+it.ID)
	case KindWorkflow:
		return "/lightning/setup/WorkflowRules/page?address=" + url.QueryEscape("/"+it.ID)
	}
	return "/lightning/setup/SetupOneHome/home"
}
