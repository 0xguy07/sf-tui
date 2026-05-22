package sf

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type TestClass struct {
	ID   string
	Name string
}

type TestClassesLoadedMsg struct {
	Classes []TestClass
}

// LoadTestClasses queries Tooling API for ApexClass with @IsTest. We can't filter
// on annotations directly, so use the SymbolTable shortcut: pull all ApexClass
// where Body LIKE '%@IsTest%' as a simple heuristic for v1.
func LoadTestClasses(orgAliasOrUser string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		// Body queries are slow on huge orgs; cap with LIMIT for v1.
		soql := "SELECT Id, Name FROM ApexClass WHERE Body LIKE '%@isTest%' OR Body LIKE '%@IsTest%' OR Body LIKE '%@ISTEST%' ORDER BY Name LIMIT 500"
		out, err := runSOQLRaw(orgAliasOrUser, soql, true)
		if err != nil {
			return ErrMsg{Err: err}
		}
		var r struct {
			Result struct {
				Records []map[string]any `json:"records"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &r); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse test classes: %w", err)}
		}
		classes := make([]TestClass, 0, len(r.Result.Records))
		for _, rec := range r.Result.Records {
			classes = append(classes, TestClass{
				ID:   stringField(rec, "Id"),
				Name: stringField(rec, "Name"),
			})
		}
		sort.Slice(classes, func(i, j int) bool { return classes[i].Name < classes[j].Name })
		return TestClassesLoadedMsg{Classes: classes}
	}
}

type TestRunResult struct {
	Pass   int
	Fail   int
	Skip   int
	Total  int
	Time   string
	Failures []TestFailure
	Coverage []ClassCoverage
}

type TestFailure struct {
	Class   string
	Method  string
	Message string
	Stack   string
}

type ClassCoverage struct {
	Name      string
	Percent   int
	Covered   int
	Total     int
	Uncovered []int // line numbers without test coverage; filled by LoadCoverageDetail
}

type TestRunDoneMsg struct {
	Result TestRunResult
}

type CoverageDetailLoadedMsg struct {
	Uncovered map[string][]int // class name → uncovered line numbers
}

func RunTests(orgAliasOrUser string, classNames []string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		args := []string{"apex", "run", "test",
			"--target-org", orgAliasOrUser,
			"--code-coverage",
			"--result-format", "json",
			"--synchronous",
			"--wait", "10",
		}
		if len(classNames) > 0 {
			args = append(args, "--class-names", strings.Join(classNames, ","))
		} else {
			args = append(args, "--test-level", "RunLocalTests")
		}
		out, err := exec.Command("sf", args...).Output()
		if err != nil {
			// Test failures cause non-zero exit but still produce JSON. Try parsing.
			var r apexTestRaw
			if jerr := json.Unmarshal(out, &r); jerr == nil && (r.Result.Summary.TestsRan > 0 || len(r.Result.Tests) > 0) {
				return TestRunDoneMsg{Result: convertTestResult(r)}
			}
			if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
				return ErrMsg{Err: fmt.Errorf("%s", string(ee.Stderr))}
			}
			return ErrMsg{Err: fmt.Errorf("sf apex run test: %w", err)}
		}
		var r apexTestRaw
		if err := json.Unmarshal(out, &r); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse test result: %w", err)}
		}
		return TestRunDoneMsg{Result: convertTestResult(r)}
	}
}

type apexTestRaw struct {
	Result struct {
		Summary struct {
			Outcome      string  `json:"outcome"`
			TestsRan     int     `json:"testsRan"`
			Passing      int     `json:"passing"`
			Failing      int     `json:"failing"`
			Skipped      int     `json:"skipped"`
			TestExecutionTime string `json:"testExecutionTime"`
		} `json:"summary"`
		Tests []struct {
			Outcome      string `json:"Outcome"`
			ApexClass    struct {
				Name string `json:"Name"`
			} `json:"ApexClass"`
			MethodName string `json:"MethodName"`
			Message    string `json:"Message"`
			StackTrace string `json:"StackTrace"`
		} `json:"tests"`
		CodeCoverage []struct {
			Name             string `json:"name"`
			NumLinesCovered  int    `json:"numLinesCovered"`
			NumLinesUncovered int   `json:"numLinesUncovered"`
		} `json:"codecoverage"`
	} `json:"result"`
}

// LoadCoverageDetail queries Tooling API for uncovered line numbers per class
// for the given class names. Used to drill into a class after a test run.
func LoadCoverageDetail(orgAliasOrUser string, classNames []string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		if len(classNames) == 0 {
			return CoverageDetailLoadedMsg{Uncovered: map[string][]int{}}
		}
		quoted := make([]string, 0, len(classNames))
		for _, n := range classNames {
			quoted = append(quoted, "'"+strings.ReplaceAll(n, "'", "\\'")+"'")
		}
		soql := fmt.Sprintf(
			"SELECT ApexClassOrTrigger.Name, Coverage FROM ApexCodeCoverageAggregate "+
				"WHERE ApexClassOrTrigger.Name IN (%s)",
			strings.Join(quoted, ","),
		)
		out, err := runSOQLRaw(orgAliasOrUser, soql, true)
		if err != nil {
			return ErrMsg{Err: err}
		}
		var r struct {
			Result struct {
				Records []struct {
					ApexClassOrTrigger struct {
						Name string `json:"Name"`
					} `json:"ApexClassOrTrigger"`
					Coverage struct {
						Uncovered []int `json:"uncoveredLines"`
					} `json:"Coverage"`
				} `json:"records"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &r); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse coverage detail: %w", err)}
		}
		out2 := make(map[string][]int, len(r.Result.Records))
		for _, rec := range r.Result.Records {
			lines := append([]int(nil), rec.Coverage.Uncovered...)
			sort.Ints(lines)
			out2[rec.ApexClassOrTrigger.Name] = lines
		}
		return CoverageDetailLoadedMsg{Uncovered: out2}
	}
}

func convertTestResult(r apexTestRaw) TestRunResult {
	out := TestRunResult{
		Pass:  r.Result.Summary.Passing,
		Fail:  r.Result.Summary.Failing,
		Skip:  r.Result.Summary.Skipped,
		Total: r.Result.Summary.TestsRan,
		Time:  r.Result.Summary.TestExecutionTime,
	}
	for _, t := range r.Result.Tests {
		if strings.EqualFold(t.Outcome, "Fail") {
			out.Failures = append(out.Failures, TestFailure{
				Class:   t.ApexClass.Name,
				Method:  t.MethodName,
				Message: t.Message,
				Stack:   t.StackTrace,
			})
		}
	}
	for _, cc := range r.Result.CodeCoverage {
		total := cc.NumLinesCovered + cc.NumLinesUncovered
		pct := 0
		if total > 0 {
			pct = (cc.NumLinesCovered * 100) / total
		}
		out.Coverage = append(out.Coverage, ClassCoverage{
			Name: cc.Name, Percent: pct, Covered: cc.NumLinesCovered, Total: total,
		})
	}
	sort.Slice(out.Coverage, func(i, j int) bool {
		return out.Coverage[i].Percent < out.Coverage[j].Percent
	})
	return out
}
