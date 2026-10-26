package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
)

func TestCalibratedInputBoundaries(t *testing.T) {
	limits := map[string]float64{"duration_seconds": 120, "steps": 32, "prompt_characters": 3, "lyrics_characters": 4}
	for _, body := range []string{`{"duration_seconds":120,"steps":32,"style":"中文歌","lyrics":"春夏秋冬"}`, `{"duration_seconds":10,"steps":8,"style":"","lyrics":""}`} {
		if err := validateInput([]byte(body), limits); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{`{"duration_seconds":121,"steps":8}`, `{"duration_seconds":10,"steps":33}`, `{"duration_seconds":10,"steps":8,"lyrics":"一二三四五"}`, `{"duration_seconds":10,"steps":8,"adapters":[{"id":"x"}]}`, `{"duration_seconds":10,"steps":8,"lm_batch_size":2}`} {
		if validateInput([]byte(body), limits) == nil {
			t.Fatalf("uncalibrated input accepted: %s", body)
		}
	}
}
func TestAdmissionAndNativeExitBeforeRelease(t *testing.T) {
	var mu sync.Mutex
	claimed, released := false, false
	var process *exec.Cmd
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		data := map[string]any{}
		switch r.URL.Path {
		case "/integrations":
			if r.Method == "GET" {
				data["items"] = []any{}
			} else {
				data["revision"] = 1
			}
		case "/permits/wait":
			var v map[string]any
			_ = json.NewDecoder(r.Body).Decode(&v)
			request, _ := v["request"].(map[string]any)
			if request["model_id"] != "yue2-q8" {
				t.Error("wrong model in admission")
			}
			data["permit_id"] = "p1"
		case "/permits/p1/verify":
			claimed = true
			data["state"] = "active"
		case "/permits/p1/release":
			var v map[string]any
			_ = json.NewDecoder(r.Body).Decode(&v)
			if v["stopped"] != true {
				t.Error("unconfirmed native stop")
			}
			if process != nil && syscall.Kill(process.Process.Pid, 0) == nil {
				t.Error("released while native process still exists")
			}
			released = true
		default:
			t.Errorf("unexpected scheduler request %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "OK", "data": data})
	}))
	defer scheduler.Close()
	s := &server{root: t.TempDir()}
	b := newBroker(s)
	b.config = binding{Settings: normalizedSettings(), URL: scheduler.URL, Token: "private-service-identity", App: appID, Models: map[string]string{"yue2-q8": "approved-executor"}, Limits: map[string]map[string]float64{"yue2-q8": {"duration_seconds": 120, "steps": 32, "prompt_characters": 1024, "lyrics_characters": 4096}}}
	l, err := b.acquire(context.Background(), "account", "/v1/music/jobs", []byte(`{"duration_seconds":10,"steps":8}`), "interactive")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := claimed
	mu.Unlock()
	if !got {
		t.Fatal("native execution was not claimed")
	}
	process = exec.Command("sleep", "60")
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = process.Start(); err != nil {
		t.Fatal(err)
	}
	w := &worker{cmd: process, done: make(chan struct{})}
	go func() { _ = process.Wait(); close(w.done) }()
	l.worker = w
	l.PID = process.Process.Pid
	b.finish(l)
	mu.Lock()
	got = released
	mu.Unlock()
	if !got || b.active != nil {
		t.Fatal("permit was not released after exit")
	}
}

func TestTerminalJobSurvivesWorkerRecycleAndStaysPrivate(t *testing.T) {
	s := &server{root: t.TempDir()}
	id := "0123456789abcdef"
	if err := privateJSON(s.terminalPath("alice", "/v1/music/jobs", id), map[string]any{"id": id, "status": "completed"}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/v1/music/jobs/"+id, nil)
	w := httptest.NewRecorder()
	if s.serveTerminal(w, r, "bob", r.URL.Path) {
		t.Fatal("terminal result leaked across accounts")
	}
	if !s.serveTerminal(w, r, "alice", r.URL.Path) || w.Code != 200 {
		t.Fatal("recycled worker lost its completed result")
	}
	r = httptest.NewRequest("DELETE", "/v1/music/jobs/"+id, nil)
	w = httptest.NewRecorder()
	if !s.serveTerminal(w, r, "alice", r.URL.Path) {
		t.Fatal("could not dismiss completed result")
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", r.URL.Path, nil)
	if s.serveTerminal(w, r, "alice", r.URL.Path) {
		t.Fatal("dismissed receipt remains visible")
	}
}

func TestSchedulerStopAcknowledgesNonceAfterNativeExit(t *testing.T) {
	var command *exec.Cmd
	ack := false
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/permits/p-stop/stopped" {
			t.Errorf("unexpected release before nonce barrier: %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["nonce"] != "approved-stop-nonce" || body["native_exit_barrier"] != true || body["model_unloaded"] != true {
			t.Error("missing stop proof")
		}
		if command != nil && syscall.Kill(command.Process.Pid, 0) == nil {
			t.Error("stop acknowledged before native exit")
		}
		ack = true
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "OK", "data": map[string]any{}})
	}))
	defer scheduler.Close()
	command = exec.Command("sleep", "60")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	worker := &worker{cmd: command, done: make(chan struct{})}
	go func() { _ = command.Wait(); close(worker.done) }()
	s := &server{root: t.TempDir()}
	b := newBroker(s)
	b.config = binding{URL: scheduler.URL}
	lease := &nativeLease{Permit: "p-stop", Execution: "exec-stop", Proof: "private-execution-proof", StopNonce: "approved-stop-nonce", worker: worker, PID: command.Process.Pid, done: make(chan struct{})}
	b.active = lease
	b.finish(lease)
	if !ack || b.active != nil {
		t.Fatal("stop nonce did not complete")
	}
}

func TestPinnedModelManifestInvalidatesCalibration(t *testing.T) {
	body, err := os.ReadFile("../models.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	if executionSettings()["model_manifest_sha256"] != hex.EncodeToString(sum[:]) {
		t.Fatal("model manifest changed without invalidating its resource calibration")
	}
}
