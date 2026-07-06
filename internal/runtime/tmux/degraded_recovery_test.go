package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- helpers -----------------------------------------------------------------

// shrinkLatchWindow makes the latch's minimum-span condition trivially
// satisfiable so tests do not sleep 90s. Restored via t.Cleanup.
func shrinkLatchWindow(t *testing.T, d time.Duration) {
	t.Helper()
	prev := degradedLatchWindow
	degradedLatchWindow = d
	t.Cleanup(func() { degradedLatchWindow = prev })
}

// isolateTmuxTmpdir points TMUX_TMPDIR at a per-test tempdir so socket-unlink
// escalation can never touch a real tmux socket. Returns the socket path the
// provider under test would resolve for socketName.
func isolateTmuxTmpdir(t *testing.T, socketName string) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("TMUX_TMPDIR", tmp)
	return filepath.Join(tmp, fmt.Sprintf("tmux-%d", os.Getuid()), socketName)
}

// stubStartTime replaces the processStartTimeFn seam. Restored via t.Cleanup.
func stubStartTime(t *testing.T, fn func(pid string) string) {
	t.Helper()
	prev := processStartTimeFn
	processStartTimeFn = fn
	t.Cleanup(func() { processStartTimeFn = prev })
}

// stubSigkill replaces the recoverSigkillFn seam with a recorder so tests
// never signal real processes. Returns the recorded PID list pointer.
func stubSigkill(t *testing.T) *[]string {
	t.Helper()
	var killed []string
	prev := recoverSigkillFn
	recoverSigkillFn = func(pid string) error {
		killed = append(killed, pid)
		return nil
	}
	t.Cleanup(func() { recoverSigkillFn = prev })
	return &killed
}

// countKillServerCalls returns how many recorded executor calls carry the
// kill-server verb.
func countKillServerCalls(calls [][]string) int {
	n := 0
	for _, call := range calls {
		for _, a := range call {
			if a == "kill-server" {
				n++
				break
			}
		}
	}
	return n
}

// --- latch arming ------------------------------------------------------------

func TestDegradedProbesBelowCountThresholdNoRecovery(t *testing.T) {
	shrinkLatchWindow(t, 0)
	isolateTmuxTmpdir(t, "gc-degraded")

	fe := &fakeExecutor{err: errors.New("tmux has-session: connection wedged")}
	tm := &Tmux{cfg: Config{SocketName: "gc-degraded"}, exec: fe}

	for i := 0; i < degradedLatchProbes-1; i++ {
		if err := tm.probeServerAlive(); !errors.Is(err, ErrServerDegraded) {
			t.Fatalf("probe %d = %v, want ErrServerDegraded", i, err)
		}
	}
	if got := countKillServerCalls(fe.calls); got != 0 {
		t.Fatalf("kill-server issued %d times below the count threshold, want 0; calls=%v", got, fe.calls)
	}
}

func TestDegradedProbesBelowWindowThresholdNoRecovery(t *testing.T) {
	shrinkLatchWindow(t, time.Hour) // count will be met; span cannot be
	isolateTmuxTmpdir(t, "gc-degraded")

	fe := &fakeExecutor{err: errors.New("tmux has-session: connection wedged")}
	tm := &Tmux{cfg: Config{SocketName: "gc-degraded"}, exec: fe}

	for i := 0; i < degradedLatchProbes+2; i++ {
		if err := tm.probeServerAlive(); !errors.Is(err, ErrServerDegraded) {
			t.Fatalf("probe %d = %v, want ErrServerDegraded", i, err)
		}
	}
	if got := countKillServerCalls(fe.calls); got != 0 {
		t.Fatalf("kill-server issued %d times below the window threshold, want 0", got)
	}
}

func TestHealthyProbeResetsDegradedStreak(t *testing.T) {
	shrinkLatchWindow(t, 0)
	isolateTmuxTmpdir(t, "gc-degraded")

	wedge := errors.New("tmux has-session: connection wedged")
	// degraded, degraded, healthy (session-not-found), degraded, degraded:
	// no run of degradedLatchProbes consecutive degraded results.
	fe := &fakeExecutor{
		outs: []string{"", "", "", "", ""},
		errs: []error{wedge, wedge, ErrSessionNotFound, wedge, wedge},
	}
	tm := &Tmux{cfg: Config{SocketName: "gc-degraded"}, exec: fe}

	for i := 0; i < 5; i++ {
		_ = tm.probeServerAlive()
	}
	if got := countKillServerCalls(fe.calls); got != 0 {
		t.Fatalf("kill-server issued %d times despite a healthy probe breaking the streak, want 0", got)
	}
}

// --- latch trip: kill-server path ---------------------------------------------

func TestDegradedLatchTripIssuesKillServerAndNewSessionRecovers(t *testing.T) {
	shrinkLatchWindow(t, 0)
	isolateTmuxTmpdir(t, "gc-degraded")

	wedge := errors.New("tmux has-session: connection wedged")
	// 3 degraded probes, then kill-server succeeds, then the post-recovery
	// NewSession sees a healthy fresh server.
	fe := &fakeExecutor{
		outs: []string{"", "", "", "", "", ""},
		errs: []error{wedge, wedge, wedge, nil /* kill-server */, ErrSessionNotFound /* probe */, nil /* new-session */},
	}
	tm := &Tmux{cfg: Config{SocketName: "gc-degraded"}, exec: fe}

	for i := 0; i < degradedLatchProbes; i++ {
		if err := tm.probeServerAlive(); !errors.Is(err, ErrServerDegraded) {
			t.Fatalf("probe %d = %v, want ErrServerDegraded", i, err)
		}
	}

	if got := countKillServerCalls(fe.calls); got != 1 {
		t.Fatalf("kill-server issued %d times after latch trip, want exactly 1; calls=%v", got, fe.calls)
	}
	// Recovery must go through the socket-scoped run path (-L <socket>),
	// never a bare kill-server that could hit the default server.
	killCall := fe.calls[degradedLatchProbes]
	if !contains(killCall, "-L") || !contains(killCall, "gc-degraded") {
		t.Fatalf("kill-server call not socket-scoped: %v", killCall)
	}

	if err := tm.NewSession("gc-recovered", ""); err != nil {
		t.Fatalf("NewSession after recovery = %v, want nil", err)
	}
}

// --- latch trip: SIGKILL escalation + socket unlink ----------------------------

func TestDegradedRecoveryEscalatesToSigkillAndUnlinksSocket(t *testing.T) {
	shrinkLatchWindow(t, 0)
	sock := isolateTmuxTmpdir(t, "gc-degraded")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	stubStartTime(t, func(pid string) string { return "Mon Jul 6 10:00:00 2026" })
	killed := stubSigkill(t)

	// Every tmux call fails, including the kill-server escalation step.
	fe := &fakeExecutor{err: errors.New("tmux: server wedged solid")}
	tm := &Tmux{cfg: Config{SocketName: "gc-degraded"}, exec: fe}
	tm.serverPID = "424242"
	tm.serverPIDStart = "Mon Jul 6 10:00:00 2026"

	for i := 0; i < degradedLatchProbes; i++ {
		_ = tm.probeServerAlive()
	}

	if got := countKillServerCalls(fe.calls); got != 1 {
		t.Fatalf("kill-server issued %d times, want 1", got)
	}
	if len(*killed) != 1 || (*killed)[0] != "424242" {
		t.Fatalf("SIGKILL seam saw %v, want exactly [424242]", *killed)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket %s still exists after recovery (stat err=%v), want unlinked", sock, err)
	}
}

func TestDegradedRecoverySkipsSigkillOnIdentityMismatch(t *testing.T) {
	shrinkLatchWindow(t, 0)
	sock := isolateTmuxTmpdir(t, "gc-degraded")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// PID has been recycled: current start time differs from the cached one.
	stubStartTime(t, func(pid string) string { return "Tue Jul 7 09:00:00 2026" })
	killed := stubSigkill(t)

	fe := &fakeExecutor{err: errors.New("tmux: server wedged solid")}
	tm := &Tmux{cfg: Config{SocketName: "gc-degraded"}, exec: fe}
	tm.serverPID = "424242"
	tm.serverPIDStart = "Mon Jul 6 10:00:00 2026"

	for i := 0; i < degradedLatchProbes; i++ {
		_ = tm.probeServerAlive()
	}

	if len(*killed) != 0 {
		t.Fatalf("SIGKILL seam saw %v despite identity mismatch, want none", *killed)
	}
	// Socket unlink still proceeds — the next new-session must bind fresh.
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket %s still exists, want unlinked", sock)
	}
}

// --- latch reset after recovery ------------------------------------------------

func TestDegradedLatchResetsAfterRecovery(t *testing.T) {
	shrinkLatchWindow(t, 0)
	isolateTmuxTmpdir(t, "gc-degraded")

	// All calls fail (probes and kill-server); no cached PID so escalation is
	// kill-server + unlink only.
	fe := &fakeExecutor{err: errors.New("tmux: still wedged")}
	tm := &Tmux{cfg: Config{SocketName: "gc-degraded"}, exec: fe}

	// First trip.
	for i := 0; i < degradedLatchProbes; i++ {
		_ = tm.probeServerAlive()
	}
	if got := countKillServerCalls(fe.calls); got != 1 {
		t.Fatalf("kill-server issued %d times after first trip, want 1", got)
	}

	// A fresh degraded run must count from zero: below-threshold probes after
	// the trip must NOT re-fire recovery.
	for i := 0; i < degradedLatchProbes-1; i++ {
		_ = tm.probeServerAlive()
	}
	if got := countKillServerCalls(fe.calls); got != 1 {
		t.Fatalf("kill-server issued %d times before second threshold met, want still 1", got)
	}

	// Completing a second full run trips again — exactly once more.
	_ = tm.probeServerAlive()
	if got := countKillServerCalls(fe.calls); got != 2 {
		t.Fatalf("kill-server issued %d times after second trip, want 2", got)
	}
}

// --- healthy-server PID caching -------------------------------------------------

func TestConfigureServerCachesServerPIDAndIdentity(t *testing.T) {
	stubStartTime(t, func(pid string) string {
		if pid != "31337" {
			t.Fatalf("processStartTimeFn called with pid %q, want 31337", pid)
		}
		return "Mon Jul 6 10:00:00 2026"
	})

	// Call 0: set-option exit-empty; call 1: display-message → server PID.
	fe := &fakeExecutor{
		outs: []string{"", "31337"},
		errs: []error{nil, nil},
	}
	tm := &Tmux{cfg: Config{SocketName: "gc-pid"}, exec: fe}

	if err := tm.ConfigureServer(); err != nil {
		t.Fatalf("ConfigureServer: %v", err)
	}
	if tm.serverPID != "31337" {
		t.Fatalf("serverPID = %q, want 31337", tm.serverPID)
	}
	if tm.serverPIDStart != "Mon Jul 6 10:00:00 2026" {
		t.Fatalf("serverPIDStart = %q, want cached start time", tm.serverPIDStart)
	}
	foundDisplay := false
	for _, call := range fe.calls {
		if contains(call, "display-message") && contains(call, "#{pid}") {
			foundDisplay = true
		}
	}
	if !foundDisplay {
		t.Fatalf("no display-message #{pid} call recorded; calls=%v", fe.calls)
	}
}

// contains reports whether slice carries the exact element s.
func contains(slice []string, s string) bool {
	for _, a := range slice {
		if a == s {
			return true
		}
	}
	return false
}

// --- companion: state cache installs empty snapshot when server is gone ---------

// noServerProbeFetcher wraps mockFetcher with the optional no-server probe.
type noServerProbeFetcher struct {
	*mockFetcher
	noServer bool
}

func (f *noServerProbeFetcher) probeNoServer(_ context.Context) bool {
	return f.noServer
}

func TestStateCache_FetchFailureWithNoServerProbeInstallsEmptySnapshot(t *testing.T) {
	inner := &mockFetcher{sessions: map[string]bool{"gc-a": true}}
	fetcher := &noServerProbeFetcher{mockFetcher: inner, noServer: true}
	cache := NewStateCache(fetcher, 50*time.Millisecond)

	if !cache.IsRunning("gc-a") {
		t.Fatal("seed refresh should report gc-a running")
	}

	// Fetch now fails (e.g. timeout) but the follow-up probe says the server
	// is definitively gone: the empty snapshot must be installed immediately,
	// NOT held back behind the 30s staleTTL last-known-good window.
	inner.setResult(nil, errors.New("fetch: context deadline exceeded"))
	cache.Invalidate()

	if cache.IsRunning("gc-a") {
		t.Fatal("IsRunning = true after fetch failure + no-server probe, want false immediately (empty snapshot installed)")
	}
}

func TestStateCache_FetchFailureWithoutNoServerKeepsLastKnownGood(t *testing.T) {
	inner := &mockFetcher{sessions: map[string]bool{"gc-a": true}}
	fetcher := &noServerProbeFetcher{mockFetcher: inner, noServer: false}
	cache := NewStateCache(fetcher, 50*time.Millisecond)

	if !cache.IsRunning("gc-a") {
		t.Fatal("seed refresh should report gc-a running")
	}

	inner.setResult(nil, errors.New("fetch: transient failure"))
	cache.Invalidate()

	if !cache.IsRunning("gc-a") {
		t.Fatal("IsRunning = false after transient fetch failure, want last-known-good preserved")
	}
}
