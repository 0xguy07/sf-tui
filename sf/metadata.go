package sf

import (
	"encoding/json"
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// MetadataItem is one entry in a deploy preview — a file plus its planned action.
type MetadataItem struct {
	Type     string // "Deploy", "Delete", "Conflict", "Ignored"
	FullName string // Salesforce metadata fullName
	MetaType string // SObject, ApexClass, etc.
	Path     string // file path in the project (when known)
}

type DeployPreviewLoadedMsg struct {
	ProjectDir string
	Items      []MetadataItem
	// RawError is set when the CLI returned an error JSON we want to show inline
	// rather than as a fatal error (e.g. no project found).
	RawError string
}

// DeployRunDoneMsg is the result of either a dry-run or a real deploy. DryRun
// distinguishes the two so the UI can label the output accordingly.
type DeployRunDoneMsg struct {
	DryRun bool
	Output string
	Err    string
}

// LoadDeployPreview shells out to `sf project deploy preview --json` and
// returns the categorized list. projectDir is the working directory the CLI
// runs in (must contain a sfdx-project.json).
func LoadDeployPreview(orgAliasOrUser, projectDir string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		if projectDir == "" {
			return ErrMsg{Err: fmt.Errorf("no project directory")}
		}
		cmd := exec.Command("sf", "project", "deploy", "preview",
			"--target-org", orgAliasOrUser, "--json")
		cmd.Dir = projectDir
		out, err := cmd.Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				if msg := previewErrMessage(ee.Stderr, out); msg != "" {
					return DeployPreviewLoadedMsg{ProjectDir: projectDir, RawError: msg}
				}
			}
			return ErrMsg{Err: fmt.Errorf("sf project deploy preview: %w", err)}
		}
		items, perr := parseDeployPreview(out)
		if perr != nil {
			return ErrMsg{Err: perr}
		}
		return DeployPreviewLoadedMsg{ProjectDir: projectDir, Items: items}
	}
}

// RunDeployDryRun shells out to `sf project deploy start --dry-run` so we can
// validate without committing changes.
func RunDeployDryRun(orgAliasOrUser, projectDir string) tea.Cmd {
	return runDeploy(nil, orgAliasOrUser, projectDir, true)
}

// RunDeploy shells out to `sf project deploy start` — a real, non-dry-run
// deploy, allowed only when the write gate passes.
func RunDeploy(g WriteGate, orgAliasOrUser, projectDir string) tea.Cmd {
	return runDeploy(g, orgAliasOrUser, projectDir, false)
}

func runDeploy(g WriteGate, orgAliasOrUser, projectDir string, dryRun bool) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		if projectDir == "" {
			return ErrMsg{Err: fmt.Errorf("no project directory")}
		}
		if !dryRun {
			if err := checkGate(g, orgAliasOrUser, ActionDeploy); err != nil {
				return ErrMsg{Err: err}
			}
		}
		args := []string{"project", "deploy", "start",
			"--target-org", orgAliasOrUser, "--json"}
		if dryRun {
			args = append(args, "--dry-run")
		}
		cmd := exec.Command("sf", args...)
		cmd.Dir = projectDir
		out, err := cmd.Output()
		if err != nil {
			stderr := ""
			if ee, ok := err.(*exec.ExitError); ok {
				stderr = string(ee.Stderr)
			}
			return DeployRunDoneMsg{DryRun: dryRun, Output: string(out), Err: stderr}
		}
		return DeployRunDoneMsg{DryRun: dryRun, Output: string(out)}
	}
}

func parseDeployPreview(raw []byte) ([]MetadataItem, error) {
	var r struct {
		Result struct {
			ToDeploy  []rawDeployItem `json:"toDeploy"`
			ToDelete  []rawDeployItem `json:"toDelete"`
			Conflicts []rawDeployItem `json:"conflicts"`
			Ignored   []rawDeployItem `json:"ignored"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("parse deploy preview: %w", err)
	}
	items := make([]MetadataItem, 0,
		len(r.Result.ToDeploy)+len(r.Result.ToDelete)+len(r.Result.Conflicts)+len(r.Result.Ignored))
	push := func(action string, src []rawDeployItem) {
		for _, it := range src {
			items = append(items, MetadataItem{
				Type:     action,
				FullName: it.FullName,
				MetaType: it.Type,
				Path:     it.Path,
			})
		}
	}
	push("Deploy", r.Result.ToDeploy)
	push("Delete", r.Result.ToDelete)
	push("Conflict", r.Result.Conflicts)
	push("Ignored", r.Result.Ignored)
	return items, nil
}

type rawDeployItem struct {
	FullName string `json:"fullName"`
	Type     string `json:"type"`
	Path     string `json:"path"`
}

// previewErrMessage extracts a useful message from preview's stderr or stdout
// when the command failed. We return "" if there's nothing user-friendly.
func previewErrMessage(stderr, stdout []byte) string {
	if len(stdout) > 0 {
		var r struct {
			Message string `json:"message"`
			Name    string `json:"name"`
		}
		if err := json.Unmarshal(stdout, &r); err == nil && r.Message != "" {
			return r.Message
		}
	}
	if len(stderr) > 0 {
		return string(stderr)
	}
	return ""
}
