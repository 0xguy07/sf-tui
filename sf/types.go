package sf

type Org struct {
	Username     string `json:"username"`
	Alias        string `json:"alias"`
	InstanceURL  string `json:"instanceUrl"`
	IsDefaultOrg bool   `json:"isDefaultUsername"`
	OrgID        string `json:"orgId"`
	Connected    string `json:"connectedStatus"`
	IsScratch    bool   `json:"isScratch"`
	DevHub       string `json:"devHubUsername"`
}

// Scratch reports the CLI's scratch marking: the scratchOrgs bucket (set in
// LoadOrgs), isScratch, or a devHubUsername.
func (o Org) Scratch() bool { return o.IsScratch || o.DevHub != "" }

type OrgListResult struct {
	Status int `json:"status"`
	Result struct {
		NonScratchOrgs []Org `json:"nonScratchOrgs"`
		ScratchOrgs    []Org `json:"scratchOrgs"`
		Other          []Org `json:"other"`
	} `json:"result"`
}

type QueryRecord map[string]any

type QueryResult struct {
	Status int `json:"status"`
	Result struct {
		TotalSize int           `json:"totalSize"`
		Done      bool          `json:"done"`
		Records   []QueryRecord `json:"records"`
	} `json:"result"`
	Message string `json:"message,omitempty"`
}
