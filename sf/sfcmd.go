package sf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// DefaultTimeout bounds every non-streaming sf invocation. Deploys and full
// test runs legitimately take minutes; the point is "never forever".
const DefaultTimeout = 10 * time.Minute

// DefaultLongTimeout is the default for commands that legitimately run long:
// `sf project deploy start` (sf waits up to 33 minutes on its own) and
// `sf apex run test --wait 10`. An explicit --timeout / SFTUI_TIMEOUT
// overrides it; the user chose, so it applies to everything.
const DefaultLongTimeout = 40 * time.Minute

// TimeoutEnv is the environment variable that overrides the default, in Go
// duration syntax (e.g. "90s", "2m"). The CLI's --timeout flag overrides both.
const TimeoutEnv = "SFTUI_TIMEOUT"

var (
	timeoutMu       sync.RWMutex
	sfTimeout       = DefaultTimeout
	timeoutExplicit bool
)

// SetTimeout sets the deadline applied to subsequent sf invocations. An
// explicit value also applies to long-running commands.
func SetTimeout(d time.Duration) {
	timeoutMu.Lock()
	sfTimeout, timeoutExplicit = d, true
	timeoutMu.Unlock()
}

// resetTimeout restores the defaults (tests).
func resetTimeout() {
	timeoutMu.Lock()
	sfTimeout, timeoutExplicit = DefaultTimeout, false
	timeoutMu.Unlock()
}

func timeoutIsExplicit() bool {
	timeoutMu.RLock()
	defer timeoutMu.RUnlock()
	return timeoutExplicit
}

// Timeout returns the current sf deadline.
func Timeout() time.Duration {
	timeoutMu.RLock()
	defer timeoutMu.RUnlock()
	return sfTimeout
}

// TimeoutFromEnv parses SFTUI_TIMEOUT. ok is false when it is unset or empty.
func TimeoutFromEnv() (d time.Duration, ok bool, err error) {
	v := os.Getenv(TimeoutEnv)
	if v == "" {
		return 0, false, nil
	}
	d, err = ParseTimeout(v)
	if err != nil {
		return 0, true, fmt.Errorf("%s=%q: %w", TimeoutEnv, v, err)
	}
	return d, true, nil
}

// ParseTimeout parses a positive Go duration.
func ParseTimeout(v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("want a duration like 90s or 2m")
	}
	if d <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return d, nil
}

// TimeoutError reports an sf invocation killed at its deadline.
type TimeoutError struct {
	Label string
	After time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("%s: timed out after %s (set --timeout or %s)", e.Label, e.After, TimeoutEnv)
}

// sfRun is an sf invocation bound to the configured deadline. Callers use it
// like *exec.Cmd: non-timeout failures still surface as *exec.ExitError with
// stdout returned, so result parsing on non-zero exits keeps working.
type sfRun struct {
	cmd    *exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc
	label  string
	after  time.Duration
}

// sfCommand prepares `sf args...` under the current deadline. label names the
// command in errors, e.g. "sf data query".
func sfCommand(label string, args ...string) *sfRun {
	return newSFRun(label, Timeout(), args...)
}

// sfCommandLong is sfCommand for commands that legitimately run long: the
// deadline is longDefault unless the user set a timeout explicitly.
func sfCommandLong(label string, longDefault time.Duration, args ...string) *sfRun {
	after := Timeout()
	if !timeoutIsExplicit() {
		after = longDefault
	}
	return newSFRun(label, after, args...)
}

func newSFRun(label string, after time.Duration, args ...string) *sfRun {
	ctx, cancel := context.WithTimeout(context.Background(), after)
	cmd := exec.CommandContext(ctx, "sf", args...)
	killGroup(cmd)
	cmd.WaitDelay = 500 * time.Millisecond // give up on pipes held by anything that survived
	return &sfRun{cmd: cmd, ctx: ctx, cancel: cancel, label: label, after: after}
}

// SetDir runs the command in dir.
func (r *sfRun) SetDir(dir string) { r.cmd.Dir = dir }

func (r *sfRun) wrap(err error) error {
	if err != nil && errors.Is(r.ctx.Err(), context.DeadlineExceeded) {
		return &TimeoutError{Label: r.label, After: r.after}
	}
	return err
}

// Output runs the command and returns stdout, like (*exec.Cmd).Output.
func (r *sfRun) Output() ([]byte, error) {
	defer r.cancel()
	out, err := r.cmd.Output()
	return out, r.wrap(err)
}

// Run runs the command, like (*exec.Cmd).Run.
func (r *sfRun) Run() error {
	defer r.cancel()
	return r.wrap(r.cmd.Run())
}
