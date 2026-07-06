package main

import (
	"strings"
	"testing"
)

// TestSupervisorLaunchdTemplate_KeepAliveAlwaysRestarts pins launchd/systemd
// restart parity. systemd gets Restart=always: the supervisor comes back after
// ANY exit, including a clean idle-exit. The old launchd shape —
// KeepAlive={Crashed:true, SuccessfulExit:false} — restarted only on crashes,
// so on macOS an idle supervisor exit left the city dark until a human ran
// gc start (the E1 zero-touch tail, age-gc-adoption-u0he.13). launchd's
// equivalent of Restart=always is KeepAlive=true; ThrottleInterval matches
// systemd's RestartSec backoff so a fast-failing supervisor cannot spin-loop.
func TestSupervisorLaunchdTemplate_KeepAliveAlwaysRestarts(t *testing.T) {
	content, err := renderSupervisorTemplate(supervisorLaunchdTemplate, &supervisorServiceData{
		GCPath:       "/usr/local/bin/gc",
		LogPath:      "/tmp/supervisor.log",
		GCHome:       "/tmp/gc-home",
		LaunchdLabel: "com.gascity.supervisor.test",
		Path:         "/usr/bin:/bin",
	})
	if err != nil {
		t.Fatalf("rendering launchd template: %v", err)
	}

	keepAliveIdx := strings.Index(content, "<key>KeepAlive</key>")
	if keepAliveIdx == -1 {
		t.Fatal("launchd plist must declare KeepAlive")
	}
	rest := content[keepAliveIdx:]
	valueEnd := strings.Index(rest, "<key>StandardOutPath</key>")
	if valueEnd == -1 {
		valueEnd = len(rest)
	}
	keepAliveValue := rest[:valueEnd]

	if !strings.Contains(keepAliveValue, "<true/>") || strings.Contains(keepAliveValue, "<dict>") {
		t.Errorf("KeepAlive must be the unconditional <true/> (launchd's Restart=always); got: %q", keepAliveValue)
	}
	if strings.Contains(content, "<key>SuccessfulExit</key>") {
		t.Error("SuccessfulExit=false restart condition must be gone — it is what left an idle-exited supervisor dark")
	}
	if !strings.Contains(content, "<key>ThrottleInterval</key>") {
		t.Error("ThrottleInterval must bound the relaunch rate (systemd RestartSec parity)")
	}
	if !strings.Contains(content, "<key>RunAtLoad</key>") {
		t.Error("RunAtLoad must survive the KeepAlive change")
	}
	if !strings.Contains(content, "GC_SUPERVISOR_PRESERVE_SESSIONS_ON_SIGNAL") {
		t.Error("preserve-sessions env must survive the KeepAlive change")
	}
}
