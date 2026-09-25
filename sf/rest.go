package sf

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type orgCreds struct {
	AccessToken string `json:"accessToken"`
	InstanceURL string `json:"instanceUrl"`
	APIVersion  string `json:"apiVersion"`
}

var orgDisplay = func(org string) (orgCreds, error) {
	out, err := exec.Command("sf", "org", "display", "--target-org", org, "--json").Output()
	var r struct {
		Result  orgCreds `json:"result"`
		Message string   `json:"message"`
	}
	if jerr := json.Unmarshal(out, &r); jerr != nil {
		if err != nil {
			return orgCreds{}, fmt.Errorf("sf org display: %w", err)
		}
		return orgCreds{}, fmt.Errorf("parse org display: %w", jerr)
	}
	if r.Result.AccessToken == "" || r.Result.InstanceURL == "" {
		if r.Message != "" {
			return orgCreds{}, fmt.Errorf("sf org display: %s", r.Message)
		}
		return orgCreds{}, fmt.Errorf("sf org display: no access token for %s", org)
	}
	if r.Result.APIVersion == "" {
		r.Result.APIVersion = "62.0"
	}
	return r.Result, nil
}

// APIError is a non-2xx REST response.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("%d: %s", e.Status, e.Message)
}

// RESTClient talks to the org's REST API directly, with credentials borrowed
// from `sf org display`. Only the Why tab uses it.
type RESTClient struct {
	org   string
	http  *http.Client
	mu    sync.Mutex
	creds *orgCreds
}

var restClients = struct {
	sync.Mutex
	m map[string]*RESTClient
}{m: map[string]*RESTClient{}}

func RESTFor(org string) *RESTClient {
	restClients.Lock()
	defer restClients.Unlock()
	if c, ok := restClients.m[org]; ok {
		return c
	}
	c := &RESTClient{org: org, http: &http.Client{Timeout: 60 * time.Second}}
	restClients.m[org] = c
	return c
}

func (c *RESTClient) getCreds(refresh bool) (orgCreds, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.creds != nil && !refresh {
		return *c.creds, nil
	}
	cr, err := orgDisplay(c.org)
	if err != nil {
		return orgCreds{}, err
	}
	c.creds = &cr
	return cr, nil
}

// get fetches path, which is either absolute ("/services/…", as in
// nextRecordsUrl) or relative to /services/data/vXX.X.
func (c *RESTClient) get(path string) ([]byte, error) {
	body, status, err := c.getOnce(path, false)
	if err == nil && status == http.StatusUnauthorized {
		body, status, err = c.getOnce(path, true)
	}
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, parseAPIError(status, body)
	}
	return body, nil
}

func (c *RESTClient) getOnce(path string, refresh bool) ([]byte, int, error) {
	cr, err := c.getCreds(refresh)
	if err != nil {
		return nil, 0, err
	}
	full := strings.TrimRight(cr.InstanceURL, "/")
	if strings.HasPrefix(path, "/services/") {
		full += path
	} else {
		full += "/services/data/v" + cr.APIVersion + path
	}
	req, err := http.NewRequest(http.MethodGet, full, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+cr.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return b, resp.StatusCode, err
}

func parseAPIError(status int, body []byte) error {
	var errs []struct {
		Message   string `json:"message"`
		ErrorCode string `json:"errorCode"`
	}
	if json.Unmarshal(body, &errs) == nil && len(errs) > 0 {
		return &APIError{Status: status, Code: errs[0].ErrorCode, Message: strings.TrimSpace(errs[0].Message)}
	}
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(status)
	}
	return &APIError{Status: status, Message: msg}
}

func (c *RESTClient) GetJSON(path string, v any) error {
	b, err := c.get(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Query runs SOQL against /query or /tooling/query and follows nextRecordsUrl.
func (c *RESTClient) Query(soql string, tooling bool) ([]QueryRecord, error) {
	path := "/query?q=" + url.QueryEscape(soql)
	if tooling {
		path = "/tooling/query?q=" + url.QueryEscape(soql)
	}
	var all []QueryRecord
	for path != "" {
		var page struct {
			Records        []QueryRecord `json:"records"`
			NextRecordsURL string        `json:"nextRecordsUrl"`
		}
		if err := c.GetJSON(path, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Records...)
		path = page.NextRecordsURL
	}
	return all, nil
}
