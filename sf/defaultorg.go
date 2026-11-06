package sf

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// sfConfigGet runs `sf config get target-org --json`. A variable so tests
// can substitute fixture output.
var sfConfigGet = func() ([]byte, error) {
	return exec.Command("sf", "config", "get", "target-org", "--json").Output()
}

// DefaultTargetOrg returns the sf CLI's default target org, or "" when none
// is configured.
func DefaultTargetOrg() (string, error) {
	out, err := sfConfigGet()
	if err != nil && len(out) == 0 {
		return "", fmt.Errorf("sf config get target-org: %w", err)
	}
	return parseTargetOrg(out)
}

func parseTargetOrg(out []byte) (string, error) {
	var r struct {
		Result []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return "", fmt.Errorf("parse sf config get: %w", err)
	}
	for _, e := range r.Result {
		if e.Name == "target-org" {
			return e.Value, nil
		}
	}
	return "", nil
}
