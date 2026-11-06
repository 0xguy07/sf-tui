package sf

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeSF installs a shell script named sf first on PATH and isolates the
// config dir. body is the script after the shebang.
func fakeSF(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "sf")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir+"/.config")
}

func withTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := Timeout()
	SetTimeout(d)
	t.Cleanup(func() { SetTimeout(old) })
}

func TestSfCommand_Timeout(t *testing.T) {
	fakeSF(t, "sleep 5")
	withTimeout(t, 100*time.Millisecond)
	start := time.Now()
	_, err := sfCommand("sf org list", "org", "list", "--json").Output()
	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("want *TimeoutError, got %v", err)
	}
	if te.Label != "sf org list" || te.After != 100*time.Millisecond {
		t.Errorf("got %+v", te)
	}
	if !strings.Contains(err.Error(), "timed out after 100ms") || !strings.Contains(err.Error(), "SFTUI_TIMEOUT") {
		t.Errorf("message = %q", err.Error())
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("took %v; the process was not killed at the deadline", time.Since(start))
	}
}

func TestSfCommand_ExitErrorPreserved(t *testing.T) {
	fakeSF(t, `echo '{"status":1,"message":"nope"}'; echo "warn" 1>&2; exit 1`)
	withTimeout(t, 5*time.Second)
	out, err := sfCommand("sf x", "x").Output()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || string(ee.Stderr) != "warn\n" || !strings.Contains(string(out), `"nope"`) {
		t.Errorf("out=%q err=%v", out, err)
	}
}

func TestSfCommand_SetDir(t *testing.T) {
	fakeSF(t, "pwd")
	withTimeout(t, 5*time.Second)
	dir := t.TempDir()
	r := sfCommand("sf x", "x")
	r.SetDir(dir)
	out, err := r.Output()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := filepath.EvalSymlinks(filepath.Clean(string(out[:len(out)-1])))
	want, _ := filepath.EvalSymlinks(dir)
	if got != want {
		t.Errorf("ran in %q, want %q", got, want)
	}
}

func TestTimeoutFromEnv(t *testing.T) {
	t.Setenv("SFTUI_TIMEOUT", "")
	if d, ok, err := TimeoutFromEnv(); d != 0 || ok || err != nil {
		t.Errorf("unset: %v %v %v", d, ok, err)
	}
	t.Setenv("SFTUI_TIMEOUT", "90s")
	if d, ok, err := TimeoutFromEnv(); d != 90*time.Second || !ok || err != nil {
		t.Errorf("90s: %v %v %v", d, ok, err)
	}
	t.Setenv("SFTUI_TIMEOUT", "banana")
	if _, _, err := TimeoutFromEnv(); err == nil {
		t.Error("banana should error")
	}
	t.Setenv("SFTUI_TIMEOUT", "0")
	if _, _, err := TimeoutFromEnv(); err == nil {
		t.Error("zero should error")
	}
}

func TestSFCommandError_TimeoutPassthrough(t *testing.T) {
	te := &TimeoutError{Label: "sf data query", After: time.Second}
	if got := sfCommandError("sf data query", nil, te); got != te {
		t.Errorf("TimeoutError must pass through unchanged, got %v", got)
	}
}

// sleepingSF hangs on every call except `sf sobject list`, which answers.
const sleepingSF = `case "$1 $2" in
  "sobject list") echo '{"status":0,"result":{"sobjects":[{"name":"Account","label":"Account","custom":false,"queryable":true,"keyPrefix":"001"}]}}' ;;
  *) sleep 5 ;;
esac`

func wantTimeout(t *testing.T, err error) {
	t.Helper()
	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Errorf("want *TimeoutError, got %v", err)
	}
}

func TestRunQuery_Timeout(t *testing.T) {
	fakeSF(t, sleepingSF)
	withTimeout(t, 100*time.Millisecond)
	msg := RunQuery("x", "SELECT Id FROM Account")()
	em, ok := msg.(ErrMsg)
	if !ok {
		t.Fatalf("got %#v", msg)
	}
	wantTimeout(t, em.Err)
}

func TestFetchDescribe_Timeout(t *testing.T) {
	fakeSF(t, sleepingSF)
	withTimeout(t, 100*time.Millisecond)
	_, _, _, err := fetchDescribe("x", "Account", true)
	wantTimeout(t, err)
}

func TestLoadLimits_Timeout(t *testing.T) {
	fakeSF(t, sleepingSF)
	withTimeout(t, 100*time.Millisecond)
	em, ok := LoadLimits("x")().(ErrMsg)
	if !ok {
		t.Fatal("want ErrMsg")
	}
	wantTimeout(t, em.Err)
}

func TestResolveUser_Timeout(t *testing.T) {
	fakeSF(t, sleepingSF)
	withTimeout(t, 100*time.Millisecond)
	_, err := ResolveUser(DefaultSOQLRunner(), "x", "alex@acme.example")
	wantTimeout(t, err)
}

func TestBuildSchemaDiff_DescribeTimeoutIsSectionError(t *testing.T) {
	fakeSF(t, sleepingSF)
	withTimeout(t, 100*time.Millisecond)
	d, err := BuildSchemaDiff(DefaultSchemaLoader(), "a", "b", CompareOptions{Object: "Account", Refresh: true})
	if err != nil {
		t.Fatalf("sobject lists answer, so no hard error expected: %v", err)
	}
	if len(d.Errors) != 2 || !strings.Contains(d.Errors[0].Message, "timed out") {
		t.Errorf("errors = %+v", d.Errors)
	}
}
