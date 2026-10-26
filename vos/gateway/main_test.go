package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
