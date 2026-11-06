package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/0xguy07/sf-tui/sf"
)

func restoreTimeout(t *testing.T) {
	t.Helper()
	old := sf.Timeout()
	t.Cleanup(func() { sf.SetTimeout(old) })
}

// observingDeps records sf.Timeout() at the moment the command would call sf.
func observingDeps(t *testing.T, seen *time.Duration) cliDeps {
	return cliDeps{
		trace: func(org, id string, refresh bool) (sf.Trace, error) {
			*seen = sf.Timeout()
			return fixtureTrace(t), nil
		},
		defaultOrg: func() (string, error) { return "dflt", nil },
	}
}

func TestTimeoutFlag_Applied(t *testing.T) {
	restoreTimeout(t)
	var seen time.Duration
	var out, errb bytes.Buffer
	if code := runWhy([]string{goodID, "--timeout", "100ms"}, &out, &errb, observingDeps(t, &seen)); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if seen != 100*time.Millisecond {
		t.Errorf("timeout seen by sf call = %v, want 100ms", seen)
	}
}

func TestTimeoutFlag_Invalid(t *testing.T) {
	restoreTimeout(t)
	for _, v := range []string{"banana", "0", "-5s"} {
		var seen time.Duration
		var out, errb bytes.Buffer
		code := runWhy([]string{goodID, "--timeout", v}, &out, &errb, observingDeps(t, &seen))
		if code != 2 || !strings.Contains(errb.String(), "--timeout") || seen != 0 {
			t.Errorf("%q: exit=%d stderr=%q seen=%v", v, code, errb.String(), seen)
		}
	}
}

func TestTimeoutEnv_Applied(t *testing.T) {
	restoreTimeout(t)
	t.Setenv(sf.TimeoutEnv, "90s")
	var seen time.Duration
	var out, errb bytes.Buffer
	if _, code := runCLI([]string{"why", goodID}, &out, &errb, observingDeps(t, &seen)); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if seen != 90*time.Second {
		t.Errorf("seen %v, want 90s", seen)
	}
}

func TestTimeoutEnv_FlagWins(t *testing.T) {
	restoreTimeout(t)
	t.Setenv(sf.TimeoutEnv, "90s")
	var seen time.Duration
	var out, errb bytes.Buffer
	runCLI([]string{"why", goodID, "--timeout", "1s"}, &out, &errb, observingDeps(t, &seen))
	if seen != time.Second {
		t.Errorf("seen %v, want 1s (flag over env)", seen)
	}
}

func TestTimeoutEnv_InvalidWarnsAndContinues(t *testing.T) {
	restoreTimeout(t)
	t.Setenv(sf.TimeoutEnv, "banana")
	var seen time.Duration
	var out, errb bytes.Buffer
	_, code := runCLI([]string{"why", goodID}, &out, &errb, observingDeps(t, &seen))
	if code != 0 || !strings.Contains(errb.String(), "warning: SFTUI_TIMEOUT") || seen != sf.DefaultTimeout {
		t.Errorf("exit=%d stderr=%q seen=%v", code, errb.String(), seen)
	}
	if w := applyTimeoutEnv(); !strings.Contains(w, "SFTUI_TIMEOUT") {
		t.Errorf("applyTimeoutEnv warning = %q", w)
	}
}
