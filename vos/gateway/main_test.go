package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadRecoversAfterCompletedWorkerExitsWithoutReplayingMutation(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		t.Run(method, func(t *testing.T) {
			identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"sub": "alice", "enable": 1})
			}))
			defer identity.Close()
			s := &server{root: t.TempDir(), web: t.TempDir(), userinfo: identity.URL, client: &http.Client{Timeout: time.Second}, users: map[string]*worker{}, sessions: map[string]session{}}
			key := userKey("alice")
			if err := privateJSON(filepath.Join(s.root, "users", key, "license.json"), map[string]string{"revision": licenseRevision}); err != nil {
				t.Fatal(err)
			}
			old := &worker{done: make(chan struct{})}
			var replacementCalls atomic.Int32
			replacement := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				replacementCalls.Add(1)
				_, _ = w.Write([]byte("saved audio"))
			}))
			defer replacement.Close()
			u, _ := url.Parse(replacement.URL)
			replacementPort, _ := strconv.Atoi(u.Port())
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				connection, _, _ := w.(http.Hijacker).Hijack()
				close(old.done)
				s.mu.Lock()
				s.users[key] = &worker{port: replacementPort, done: make(chan struct{})}
				s.mu.Unlock()
				_ = connection.Close()
			}))
			defer backend.Close()
			u, _ = url.Parse(backend.URL)
			old.port, _ = strconv.Atoi(u.Port())
			s.users[key] = old
			path := "/v1/library/media/fixture"
			if method == "POST" {
				path = "/v1/library/import"
			}
			r := httptest.NewRequest(method, prefix+path, nil)
			if method == "POST" {
				r.Body = io.NopCloser(strings.NewReader("{}"))
			}
			r.Header.Set("Authorization", "Bearer alice")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if method == "GET" && (w.Code != 200 || w.Body.String() != "saved audio" || replacementCalls.Load() != 1) {
				t.Fatalf("completed audio was lost during worker exit: %d %q", w.Code, w.Body.String())
			}
			if method == "POST" && (w.Code != 502 || replacementCalls.Load() != 0) {
				t.Fatal("a mutation was replayed after transport failure")
			}
		})
	}
}

func TestIdentityConsentAndIsolation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		enable := 1
		if token == "disabled" {
			enable = 0
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": token, "enable": enable, "role_code": "normal_user"})
	}))
	defer upstream.Close()
	s := &server{root: t.TempDir(), web: t.TempDir(), userinfo: upstream.URL, client: &http.Client{Timeout: time.Second}, users: map[string]*worker{}, sessions: map[string]session{}}
	call := func(token, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", prefix+path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, token := range []string{"", "disabled"} {
		if got := call(token, "/vos/session", "").Code; got != 401 {
			t.Fatalf("unauthenticated/disabled: %d", got)
		}
	}
	if got := call("alice", "/setup/download", `{}`).Code; got != 428 {
		t.Fatalf("download before license: %d", got)
	}
	if got := call("alice", "/vos/license", `{"accepted":true,"revision":"stale"}`).Code; got != 400 {
		t.Fatalf("stale license: %d", got)
	}
	if got := call("alice", "/vos/license", `{"accepted":true,"revision":"`+licenseRevision+`"}`).Code; got != 200 {
		t.Fatalf("consent: %d", got)
	}
	if !s.accepted(userKey("alice")) || s.accepted(userKey("bob")) {
		t.Fatal("account consent leaked")
	}
	if got := call("alice", "/setup/download", `{}`).Code; got != 403 {
		t.Fatalf("non-admin download: %d", got)
	}
	if got := call("alice", "/engine/options", `{}`).Code; got != 403 {
		t.Fatalf("desktop command boundary: %d", got)
	}
	r := httptest.NewRequest("POST", prefix+"/vos/session", nil)
	r.Header.Set("Authorization", "Bearer alice")
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin mutation accepted")
	}
}
