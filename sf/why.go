package sf

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// whyAPI is the slice of RESTClient the Why trace needs; tests swap in fixtures.
type whyAPI interface {
	Query(soql string, tooling bool) ([]QueryRecord, error)
	GetJSON(path string, v any) error
}

// Trace answers "why did this record change?" for one record. Sections load
// independently; a failed section records itself in Errors and leaves the rest intact.
type Trace struct {
	Record           RecordRef           `json:"record"`
	Object           ObjectRef           `json:"object"`
	TrackedFields    []string            `json:"trackedFields"`
	Saves            []Save              `json:"saves"`
	HistoryTruncated bool                `json:"historyTruncated,omitempty"`
	Automation       []AutomationSection `json:"automation"`
	Errors           []SectionError      `json:"errors"`
}

type RecordRef struct {
	ID string `json:"id"`
}

type ObjectRef struct {
	Name          string `json:"name"`
	Label         string `json:"label"`
	DurableID     string `json:"durableId"`
	KeyPrefix     string `json:"keyPrefix"`
	HistoryObject string `json:"historyObject,omitempty"`
}

type SectionError struct {
	Section string `json:"section"`
	Message string `json:"message"`
}

type Save struct {
	At          time.Time    `json:"at"`
	CreatedByID string       `json:"createdById"`
	User        *HistoryUser `json:"user,omitempty"`
	Hint        string       `json:"hint,omitempty"`
	Changes     []Change     `json:"changes"`
}

type Change struct {
	Field          string   `json:"field"`
	CanonicalField string   `json:"canonicalField"`
	DataType       string   `json:"dataType,omitempty"`
	Created        bool     `json:"created,omitempty"`
	Old            any      `json:"old"`
	New            any      `json:"new"`
	OldID          string   `json:"oldId,omitempty"`
	NewID          string   `json:"newId,omitempty"`
	Writers        []Writer `json:"writers,omitempty"`
}

func (c Change) Label() string {
	if c.Created {
		return "Record created"
	}
	return c.Field
}

type HistoryUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	UserType string `json:"userType"`
	IsActive bool   `json:"isActive"`
	Profile  string `json:"profile,omitempty"`
	License  string `json:"license,omitempty"`
	Badge    string `json:"badge,omitempty"`
}

type Writer struct {
	Kind    string `json:"kind"`
	ID      string `json:"id,omitempty"`
	Name    string `json:"name"`
	Section string `json:"section"`
	Note    string `json:"note,omitempty"`
}

const (
	BadgeAuto  = "AUTO"
	BadgeInteg = "INTEG"
	BadgeGuest = "GUEST"
)

func ClassifyUser(userType, profile, license string) string {
	switch userType {
	case "AutomatedProcess":
		return BadgeAuto
	case "Guest":
		return BadgeGuest
	}
	for _, s := range []string{profile, license} {
		ls := strings.ToLower(s)
		if strings.Contains(ls, "integration") || strings.Contains(ls, "api only") {
			return BadgeInteg
		}
	}
	return ""
}

func badgeHint(badge string) string {
	switch badge {
	case BadgeAuto:
		return "often an async or scheduled path, platform event, or queueable"
	case BadgeInteg:
		return "external system via API"
	}
	return ""
}

func (t *Trace) addError(section string, err error) {
	if err == nil {
		return
	}
	t.Errors = append(t.Errors, SectionError{Section: section, Message: err.Error()})
}

// ErrorFor returns the message recorded for a section, or "".
func (t Trace) ErrorFor(section string) string {
	for _, e := range t.Errors {
		if e.Section == section {
			return e.Message
		}
	}
	return ""
}

// --- resolve ---

var recordIDRe = regexp.MustCompile(`^[a-zA-Z0-9]{15}([a-zA-Z0-9]{3})?$`)

func ValidRecordID(id string) bool { return recordIDRe.MatchString(id) }

// ErrRecordNotFound is returned when the Id resolves to an object but no row.
var ErrRecordNotFound = fmt.Errorf("Deleted or not visible to this user.")

// HistoryObjectName maps an sObject to its field-history sObject.
func HistoryObjectName(obj string) string {
	switch {
	case strings.EqualFold(obj, "Opportunity"):
		return "OpportunityFieldHistory"
	case strings.HasSuffix(obj, "__c"):
		return strings.TrimSuffix(obj, "__c") + "__History"
	}
	return obj + "History"
}

// HistoryParentField picks the history object's reference field that points at obj.
func HistoryParentField(hist Describe, obj string) string {
	var fallback string
	for _, f := range hist.Fields {
		if f.Name == "CreatedById" || f.Name == "LastModifiedById" {
			continue
		}
		for _, r := range f.ReferenceTo {
			if strings.EqualFold(r, obj) {
				if f.Name == "ParentId" || strings.EqualFold(f.Name, obj+"Id") {
					return f.Name
				}
				if fallback == "" {
					fallback = f.Name
				}
			}
		}
	}
	return fallback
}

func loadGlobalDescribe(api whyAPI, org string, force bool) ([]SObjectSummary, error) {
	if !force {
		if ss, at, ok := readGlobalDescribeCache(org); ok && cacheFresh(at) {
			return ss, nil
		}
	}
	var r struct {
		SObjects []SObjectSummary `json:"sobjects"`
	}
	if err := api.GetJSON("/sobjects", &r); err != nil {
		return nil, fmt.Errorf("global describe: %w", err)
	}
	writeGlobalDescribeCache(org, r.SObjects)
	return r.SObjects, nil
}

func loadRESTDescribe(api whyAPI, org, sobject string, force bool) (Describe, error) {
	if !force {
		if d, at, ok := readDescribeCache(org, sobject); ok && cacheFresh(at) {
			return d, nil
		}
	}
	var d Describe
	if err := api.GetJSON("/sobjects/"+sobject+"/describe", &d); err != nil {
		return Describe{}, fmt.Errorf("describe %s: %w", sobject, err)
	}
	writeDescribeCache(org, d)
	return d, nil
}

// ResolveRecord finds the record's sObject and confirms the row is visible.
func ResolveRecord(api whyAPI, org, id string, force bool) (Trace, error) {
	t := Trace{Record: RecordRef{ID: id}, Saves: []Save{}, Errors: []SectionError{}}
	if !ValidRecordID(id) {
		err := fmt.Errorf("%q isn't a 15- or 18-character record Id", id)
		t.addError("record", err)
		return t, err
	}
	ss, err := loadGlobalDescribe(api, org, force)
	if err != nil {
		t.addError("record", err)
		return t, err
	}
	prefix := id[:3]
	names := map[string]bool{}
	for _, s := range ss {
		names[lc(s.Name)] = true
	}
	for _, s := range ss {
		if s.KeyPrefix == prefix && s.Queryable {
			t.Object = ObjectRef{Name: s.Name, Label: s.Label, KeyPrefix: prefix}
			break
		}
	}
	if t.Object.Name == "" {
		err := fmt.Errorf("no queryable sObject has key prefix %s", prefix)
		t.addError("record", err)
		return t, err
	}
	obj := t.Object.Name
	rows, err := api.Query("SELECT DurableId, QualifiedApiName, Label FROM EntityDefinition WHERE QualifiedApiName = '"+soqlEscape(obj)+"'", false)
	if err != nil {
		t.addError("record", fmt.Errorf("entity definition: %w", err))
		return t, err
	}
	if len(rows) > 0 {
		t.Object.DurableID = str(rows[0]["DurableId"])
		if l := str(rows[0]["Label"]); l != "" {
			t.Object.Label = l
		}
	}
	if t.Object.DurableID == "" {
		t.Object.DurableID = obj
	}
	rows, err = api.Query("SELECT Id FROM "+obj+" WHERE Id = '"+soqlEscape(id)+"'", false)
	if err != nil {
		t.addError("record", err)
		return t, err
	}
	if len(rows) == 0 {
		t.addError("record", ErrRecordNotFound)
		return t, ErrRecordNotFound
	}
	t.Record.ID = str(rows[0]["Id"])
	if h := HistoryObjectName(obj); names[lc(h)] {
		t.Object.HistoryObject = h
	}
	return t, nil
}

// --- history ---

// HistoryPart is the history half of a Trace.
type HistoryPart struct {
	TrackedFields []string
	Saves         []Save
	Truncated     bool
	Errors        []SectionError
}

const historyLimit = 200

func LoadHistory(api whyAPI, org string, t Trace, force bool) HistoryPart {
	p := HistoryPart{Saves: []Save{}}
	fail := func(section string, err error) {
		p.Errors = append(p.Errors, SectionError{Section: section, Message: err.Error()})
	}
	obj := t.Object.Name
	if t.Object.HistoryObject == "" {
		fail("history", fmt.Errorf("History tracking isn't enabled on %s.", obj))
		return p
	}

	tracked, err := api.Query("SELECT QualifiedApiName FROM FieldDefinition WHERE EntityDefinition.QualifiedApiName = '"+soqlEscape(obj)+"' AND IsFieldHistoryTracked = true", false)
	if err != nil {
		fail("tracked", err)
	}
	for _, r := range tracked {
		p.TrackedFields = append(p.TrackedFields, str(r["QualifiedApiName"]))
	}
	sort.Strings(p.TrackedFields)

	hd, err := loadRESTDescribe(api, org, t.Object.HistoryObject, force)
	if err != nil {
		fail("history", err)
		return p
	}
	parent := HistoryParentField(hd, obj)
	if parent == "" {
		fail("history", fmt.Errorf("%s has no reference field to %s", t.Object.HistoryObject, obj))
		return p
	}
	rows, err := api.Query(fmt.Sprintf(
		"SELECT Id, Field, DataType, OldValue, NewValue, CreatedById, CreatedDate FROM %s WHERE %s = '%s' ORDER BY CreatedDate DESC, Id DESC LIMIT %d",
		t.Object.HistoryObject, parent, soqlEscape(t.Record.ID), historyLimit), false)
	if err != nil {
		fail("history", err)
		return p
	}
	p.Truncated = len(rows) >= historyLimit
	p.Saves = GroupHistory(rows, p.TrackedFields)

	if ids := saveUserIDs(p.Saves); len(ids) > 0 {
		users, err := loadHistoryUsers(api, ids)
		if err != nil {
			fail("users", err)
		}
		for i := range p.Saves {
			if u, ok := users[p.Saves[i].CreatedByID]; ok {
				p.Saves[i].User = &u
				p.Saves[i].Hint = badgeHint(u.Badge)
			}
		}
	}
	return p
}

// CanonicalField maps a history Field value ("Owner") to the field API name
// ("OwnerId") using the tracked-field list. Unknown names pass through.
func CanonicalField(field string, tracked []string) string {
	for _, f := range tracked {
		if strings.EqualFold(f, field) {
			return f
		}
	}
	for _, f := range tracked {
		if strings.EqualFold(f, field+"Id") {
			return f
		}
	}
	return field
}

const historyTimeLayout = "2006-01-02T15:04:05.000-0700"

// GroupHistory collapses lookup Id/name row pairs and groups rows into saves by
// (CreatedDate, CreatedById). Rows must be newest first; saves keep that order.
func GroupHistory(rows []QueryRecord, tracked []string) []Save {
	type key struct{ at, by string }
	saves := []Save{}
	idx := map[key]int{}
	changeIdx := map[key]map[string]int{}
	for _, r := range rows {
		k := key{str(r["CreatedDate"]), str(r["CreatedById"])}
		si, ok := idx[k]
		if !ok {
			at, _ := time.Parse(historyTimeLayout, k.at)
			saves = append(saves, Save{At: at, CreatedByID: k.by})
			si = len(saves) - 1
			idx[k] = si
			changeIdx[k] = map[string]int{}
		}
		s := &saves[si]
		field := str(r["Field"])
		dt := str(r["DataType"])
		if field == "created" {
			s.Changes = append(s.Changes, Change{Field: field, CanonicalField: field, Created: true})
			continue
		}
		ci, seen := changeIdx[k][field]
		if !seen {
			s.Changes = append(s.Changes, Change{Field: field, CanonicalField: CanonicalField(field, tracked)})
			ci = len(s.Changes) - 1
			changeIdx[k][field] = ci
		}
		c := &s.Changes[ci]
		if dt == "EntityId" {
			c.OldID, c.NewID = str(r["OldValue"]), str(r["NewValue"])
			if !seen {
				c.Old, c.New = r["OldValue"], r["NewValue"]
				c.DataType = dt
			}
			continue
		}
		c.Old, c.New = r["OldValue"], r["NewValue"]
		c.DataType = dt
	}
	return saves
}

func saveUserIDs(saves []Save) []string {
	seen := map[string]bool{}
	var ids []string
	for _, s := range saves {
		if s.CreatedByID != "" && !seen[s.CreatedByID] {
			seen[s.CreatedByID] = true
			ids = append(ids, s.CreatedByID)
		}
	}
	return ids
}

func loadHistoryUsers(api whyAPI, ids []string) (map[string]HistoryUser, error) {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = "'" + soqlEscape(id) + "'"
	}
	rows, err := api.Query("SELECT Id, Name, Username, UserType, IsActive, Profile.Name, Profile.UserLicense.Name FROM User WHERE Id IN ("+strings.Join(quoted, ",")+")", false)
	if err != nil {
		return nil, err
	}
	out := map[string]HistoryUser{}
	for _, r := range rows {
		u := HistoryUser{
			ID:       str(r["Id"]),
			Name:     str(r["Name"]),
			Username: str(r["Username"]),
			UserType: str(r["UserType"]),
			Profile:  str(ResolvePath(r, "Profile.Name")),
			License:  str(ResolvePath(r, "Profile.UserLicense.Name")),
		}
		u.IsActive, _ = r["IsActive"].(bool)
		u.Badge = ClassifyUser(u.UserType, u.Profile, u.License)
		out[u.ID] = u
	}
	return out, nil
}

func (t *Trace) ApplyHistory(p HistoryPart) {
	t.TrackedFields = p.TrackedFields
	t.Saves = p.Saves
	t.HistoryTruncated = p.Truncated
	t.Errors = append(t.Errors, p.Errors...)
}

func (t *Trace) ApplyAutomation(p AutomationPart) {
	t.Automation = p.Sections
	t.Errors = append(t.Errors, p.Errors...)
}

// BuildTrace runs every section and attributes writers. The error is non-nil
// only when the record itself can't be resolved.
func BuildTrace(api whyAPI, org, id string, force bool) (Trace, error) {
	t, err := ResolveRecord(api, org, id, force)
	if err != nil {
		return t, err
	}
	var h HistoryPart
	var a AutomationPart
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); h = LoadHistory(api, org, t, force) }()
	go func() { defer wg.Done(); a = LoadAutomation(api, org, t.Object, force) }()
	wg.Wait()
	t.ApplyHistory(h)
	t.ApplyAutomation(a)
	t.Attribute()
	return t, nil
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}
