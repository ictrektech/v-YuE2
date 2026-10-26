package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"
)

const appID = "com.ictrek.v-yue2"

// Linker override is used only by isolated calibration builds; production is fixed.
var computeOrigin = "http://v-yue2-backend:8791"

const controllerURL = "http://v-scheduler:8080/scheduler/api/v1"

type binding struct {
	Protocol      int                           `json:"protocol_version"`
	Revision      int                           `json:"binding_revision"`
	App           string                        `json:"app_id"`
	URL           string                        `json:"url"`
	Token         string                        `json:"token"`
	ExecutorToken string                        `json:"executor_token"`
	Upstream      string                        `json:"upstream_url"`
	Models        map[string]string             `json:"models"`
	Limits        map[string]map[string]float64 `json:"model_limits"`
	Settings      map[string]any                `json:"execution_settings"`
}
type nativeLease struct {
	StopNonce    string          `json:"stop_nonce,omitempty"`
	Account      string          `json:"account"`
	Kind         string          `json:"kind"`
	JobID        string          `json:"job_id"`
	Job          json.RawMessage `json:"job"`
	Permit       string          `json:"permit_id"`
	Execution    string          `json:"execution_id"`
	Proof        string          `json:"execution_token"`
	Executor     string          `json:"executor_id"`
	Model        string          `json:"model_id"`
	Stopped      bool            `json:"stopped"`
	PID          int             `json:"pid"`
	Start        string          `json:"process_birth"`
	Started      bool            `json:"started"`
	WaitDeadline int64           `json:"wait_deadline"`
	Priority     string          `json:"priority"`
	live         bool
	worker       *worker
	done         chan struct{}
	once         sync.Once
}
type broker struct {
	s        *server
	mu       sync.Mutex
	io       sync.Mutex
	config   binding
	revision int
	active   *nativeLease
	owner    string
}

func executionSettings() map[string]any {
	return map[string]any{"runtime": "yue2-cpp-v1", "native_parent_guard": "dedicated-spawn-thread-v1", "engine_commit": "c8516dbf44d660cca40d3f420c67063e37ec8258", "engine_sha256": engineHash(), "model_manifest_sha256": "c67d200cbd57f7e5626242f151eee535568fd7ac1cf5f756b7542c09c409438c", "cpu_threads": 8, "max_batch": 1, "max_seq": 8192, "vae_core": 512, "vae_halo": 16, "keep_loaded": false, "clamp_fp16": true, "flash_attention": true, "concurrency": 1, "engine_admission": "permit-before-native-start-v1"}
}
func randomID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func privateJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func readPrivate(path string, out any) error {
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 256<<10 {
		return errors.New("private bounded scheduler state required")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, out)
}
func newBroker(s *server) *broker {
	b := &broker{s: s}
	_ = readPrivate(filepath.Join(s.root, "scheduler.json"), &b.config)
	var old nativeLease
	if readPrivate(filepath.Join(s.root, "native-lease.json"), &old) == nil {
		old.done = make(chan struct{})
		b.active = &old
	}
	return b
}
func (b *broker) call(ctx context.Context, c binding, method, path string, body any, key string) (map[string]any, error) {
	var data []byte
	var e error
	if body != nil {
		data, e = json.Marshal(body)
		if e != nil {
			return nil, e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, c.URL+path, bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	timeout := 10 * time.Second
	if path == "/permits/wait" {
		timeout = 135 * time.Second
	}
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := client.Do(req)
	if e != nil {
		return nil, errors.New("V-Scheduler unavailable")
	}
	defer res.Body.Close()
	var value struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if e != nil {
		return nil, e
	}
	if res.StatusCode != 200 {
		var detail struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(raw, &detail)
		return nil, fmt.Errorf("V-Scheduler rejected admission (%d): %s", res.StatusCode, detail.Code)
	}
	if json.Unmarshal(raw, &value) != nil || value.Code != "OK" {
		return nil, errors.New("invalid scheduler receipt")
	}
	return value.Data, nil
}
func (b *broker) handle(w http.ResponseWriter, r *http.Request, path string) bool {
	if path != "/api/v1/scheduler/discovery" && path != "/api/v1/scheduler/binding" {
		return false
	}
	if path == "/api/v1/scheduler/discovery" && r.Method == "GET" {
		ready := engineHash() != "unavailable"
		for _, name := range []string{"YuE2-3B-Q8_0.gguf", "YuE2-Vae-F32.gguf", "nar_lora_joint_v9.safetensors"} {
			if st, e := os.Stat(filepath.Join(env("YUE_MODELHUB_ROOT", "/modelhub"), "export/ms/vivibit/YuE2-Studio-GGUF/current", name)); e != nil || st.Size() == 0 {
				ready = false
			}
		}
		models := []map[string]any{}
		if ready {
			models = append(models, map[string]any{"id": "yue2-q8", "task_type": "model.infer"})
		}
		reply(w, 200, map[string]any{"scheduler": map[string]any{"protocol_version": 1, "app_id": appID, "executor_base_url": computeOrigin, "upstream_url": computeOrigin, "binding_endpoint": "/api/v1/scheduler/binding", "execution_settings": executionSettings(), "sync_models": models, "preemption_modes": []string{"restart"}}})
		return true
	}
	if r.Method == "GET" {
		b.mu.Lock()
		n, rev := len(b.config.Models), b.config.Revision
		b.mu.Unlock()
		reply(w, 200, map[string]any{"configured": n > 0, "binding_revision": rev, "approved_model_count": n})
		return true
	}
	if r.Method != "POST" {
		fail(w, 405, "Method not allowed")
		return true
	}
	if !b.s.schedulerIdentity(r) {
		fail(w, 403, "V-Scheduler service identity required")
		return true
	}
	var c binding
	if json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(&c) != nil || c.Protocol != 1 || c.Revision < 1 || c.App != appID || c.URL != controllerURL || c.Upstream != computeOrigin || len(c.Token) < 32 || !reflect.DeepEqual(c.Settings, normalizedSettings()) || len(c.Models) > 16 {
		fail(w, 422, "Invalid trusted scheduler binding")
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active != nil || c.Revision < b.config.Revision {
		fail(w, 409, "Active execution or stale binding")
		return true
	}
	if e := b.preflight(r.Context(), c); e != nil {
		fail(w, 503, "Scheduler executor verification failed")
		return true
	}
	if e := privateJSON(filepath.Join(b.s.root, "scheduler.json"), c); e != nil {
		fail(w, 500, "Cannot persist binding")
		return true
	}
	b.config = c
	b.io.Lock()
	b.revision = 0
	b.io.Unlock()
	reply(w, 200, map[string]any{"configured": len(c.Models) > 0, "binding_revision": c.Revision})
	return true
}
func normalizedSettings() map[string]any {
	data, _ := json.Marshal(executionSettings())
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}
func (b *broker) preflight(ctx context.Context, c binding) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.URL+"/executors", nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, e := b.s.client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	var v struct {
		Code string `json:"code"`
		Data []struct {
			ID   string   `json:"id"`
			App  string   `json:"app_id"`
			Task string   `json:"task_type"`
			URLs []string `json:"upstream_urls"`
		} `json:"data"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&v) != nil || v.Code != "OK" {
		return errors.New("executor directory unavailable")
	}
	for model, executor := range c.Models {
		if model != "yue2-q8" || len(c.Limits[model]) < 4 {
			return errors.New("uncalibrated model")
		}
		found := false
		for _, x := range v.Data {
			if x.ID == executor && x.App == c.App && x.Task == "model.infer" {
				for _, u := range x.URLs {
					found = found || u == computeOrigin
				}
			}
		}
		if !found {
			return errors.New("executor does not match application")
		}
	}
	return nil
}
func (b *broker) ensure(ctx context.Context, c binding) error {
	b.io.Lock()
	defer b.io.Unlock()
	if b.revision > 0 {
		return nil
	}
	data, e := b.call(ctx, c, "GET", "/integrations", nil, "")
	if e != nil {
		return e
	}
	revision := 0.
	if rows, ok := data["items"].([]any); ok {
		for _, v := range rows {
			r, _ := v.(map[string]any)
			if r["app_id"] == c.App {
				revision, _ = r["revision"].(float64)
			}
		}
	}
	ids := []string{}
	for _, id := range c.Models {
		ids = append(ids, id)
	}
	data, e = b.call(ctx, c, "POST", "/integrations", map[string]any{"protocol_version": 1, "revision": revision, "executors": ids, "stop_protocol": "permit-native-exit-v1"}, "")
	if e != nil {
		return e
	}
	if rev, ok := data["revision"].(float64); ok && rev > 0 {
		b.revision = int(rev)
		return nil
	}
	return errors.New("integration receipt missing")
}
func validateInput(body []byte, limits map[string]float64) error {
	if len(body) > 65536 {
		return errors.New("music request exceeds approved 64 KiB input bound")
	}
	var v map[string]any
	if json.Unmarshal(body, &v) != nil {
		return errors.New("invalid music request")
	}
	allowed := map[string]bool{}
	for _, name := range []string{"client_ref", "title", "style", "lyrics", "cot", "duration_seconds", "lm_seed", "seed", "steps", "lm_batch_size", "synth_batch_size", "cfg_scale", "companion_scale", "solver", "output_format", "mp3_bitrate", "fade_in", "fade_out", "playlist_id", "cover_prompt", "abc", "transpose", "lyric_timing", "vocals_only", "semantic_tokens", "abc_sampling", "semantic_sampling", "harmony", "abc_continue", "adapters", "cover_of"} {
		allowed[name] = true
	}
	for name := range v {
		if !allowed[name] {
			return fmt.Errorf("unapproved generation field: %s", name)
		}
	}
	for _, name := range []string{"abc_sampling", "semantic_sampling", "harmony"} {
		if value := v[name]; value != nil {
			if object, ok := value.(map[string]any); !ok || len(object) > 0 {
				return errors.New("custom sampling is not calibrated")
			}
		}
	}
	if cot, ok := v["cot"].(string); ok && cot != "" && cot != "off" && cot != "melody" && cot != "full" {
		return errors.New("unapproved planning mode")
	}
	if solver, ok := v["solver"].(string); ok && solver != "" && solver != "midpoint" {
		return errors.New("unapproved solver")
	}
	number := func(key string, def float64) float64 {
		if n, ok := v[key].(float64); ok {
			return n
		}
		return def
	}
	for key, limit := range map[string]float64{"duration_seconds": limits["duration_seconds"], "steps": limits["steps"]} {
		n := number(key, 0)
		if n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) || limit <= 0 || n > limit {
			return fmt.Errorf("%s exceeds approved input limits", key)
		}
	}
	for key, limit := range map[string]float64{"style": limits["prompt_characters"], "lyrics": limits["lyrics_characters"]} {
		s, _ := v[key].(string)
		if limit <= 0 || float64(len([]rune(s))) > limit {
			return errors.New("text exceeds approved input limits")
		}
	}
	if number("batch_size", 1) != 1 || number("lm_batch_size", 1) != 1 || number("synth_batch_size", 1) != 1 || number("companion_scale", 1) < 0 || number("companion_scale", 1) > 1 {
		return errors.New("uncalibrated generation settings")
	}
	for _, key := range []string{"adapters", "semantic_tokens", "abc", "vocals_only", "score_edit", "abc_continue"} {
		if value, ok := v[key]; ok && value != nil && value != "" && value != false {
			if list, ok := value.([]any); !ok || len(list) > 0 {
				return errors.New("this generation variant is not calibrated")
			}
		}
	}
	return nil
}
func (b *broker) acquire(ctx context.Context, key, path string, body []byte, priority string) (*nativeLease, error) {
	b.mu.Lock()
	c := b.config
	active := b.active != nil
	b.mu.Unlock()
	if active {
		return nil, errors.New("previous execution must be confirmed stopped")
	}
	if !reflect.DeepEqual(c.Settings, normalizedSettings()) {
		return nil, errors.New("V-Scheduler calibration no longer matches this runtime")
	}
	if path != "/v1/music/jobs" || c.Models["yue2-q8"] == "" {
		return nil, errors.New("V-Scheduler has not calibrated this local operation")
	}
	if e := validateInput(body, c.Limits["yue2-q8"]); e != nil {
		return nil, e
	}
	if priority == "" {
		priority = "background"
	}
	if priority != "background" && priority != "interactive" && priority != "high" {
		return nil, errors.New("invalid scheduling priority")
	}
	if e := b.ensure(ctx, c); e != nil {
		return nil, e
	}
	lease := &nativeLease{Account: key, Kind: path, Execution: randomID(), Proof: randomID(), Executor: c.Models["yue2-q8"], Model: "yue2-q8", done: make(chan struct{}), live: true}
	lease.WaitDeadline = time.Now().Add(135 * time.Second).Unix()
	lease.Priority = priority
	lease.Stopped = true
	b.mu.Lock()
	b.active = lease
	b.mu.Unlock()
	if err := b.saveLease(lease); err != nil {
		return nil, err
	}
	receipt, e := b.call(ctx, c, "POST", "/permits/wait", map[string]any{"request": map[string]any{"executor_id": lease.Executor, "upstream_url": computeOrigin, "model_id": lease.Model, "execution_token": lease.Proof, "scheduling": map[string]any{"priority": priority, "preemptible": true, "allow_preemption": false, "force_admission": false}}, "wait_seconds": 120}, lease.Execution)
	if e != nil {
		// Keep an unknown admission receipt journaled. Maintenance resolves
		// only the admission key and confirms no native execution started.
		return nil, e
	}
	lease.Permit, _ = receipt["permit_id"].(string)
	if lease.Permit == "" {
		return nil, errors.New("missing compute permit")
	}
	b.mu.Lock()
	b.active = lease
	b.mu.Unlock()
	if e = b.saveLease(lease); e != nil {
		b.finish(lease)
		return nil, e
	}
	if _, e = b.call(ctx, c, "POST", "/permits/"+lease.Permit+"/verify", lease.verify(true), ""); e != nil {
		b.finish(lease)
		return nil, e
	}
	b.mu.Lock()
	b.owner = key
	b.mu.Unlock()
	return lease, nil
}
func (l *nativeLease) verify(claim bool) map[string]any {
	return map[string]any{"executor_id": l.Executor, "upstream_url": computeOrigin, "model_id": l.Model, "execution_token": l.Proof, "execution_id": l.Execution, "claim": claim}
}
func stopNative(w *worker) bool {
	if w == nil {
		return true
	}
	_ = syscall.Kill(-w.cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-w.done:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-w.cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-w.done:
		case <-time.After(5 * time.Second):
			return false
		}
	}
	for i := 0; i < 50; i++ {
		if errors.Is(syscall.Kill(-w.cmd.Process.Pid, 0), syscall.ESRCH) {
			return true
		}
		_ = syscall.Kill(-w.cmd.Process.Pid, syscall.SIGKILL)
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
func (b *broker) finish(l *nativeLease) {
	l.once.Do(func() {
		close(l.done)
		b.mu.Lock()
		b.owner = ""
		b.mu.Unlock()
		b.mu.Lock()
		w, pid, prior := l.worker, l.PID, l.Stopped
		b.mu.Unlock()
		stopped := stopNative(w)
		if w == nil && pid != 0 {
			stopped = prior
		}
		b.mu.Lock()
		l.Stopped = stopped
		l.live = false
		b.mu.Unlock()
		_ = b.saveLease(l)
		if stopped {
			b.cacheStoppedJob(l)
		}
		b.mu.Lock()
		c := b.config
		b.mu.Unlock()
		var e error
		b.mu.Lock()
		nonce := l.StopNonce
		b.mu.Unlock()
		if stopped && nonce != "" {
			_, e = b.call(context.Background(), c, "POST", "/permits/"+l.Permit+"/stopped", map[string]any{"nonce": nonce, "execution_id": l.Execution, "execution_token": l.Proof, "native_exit_barrier": true, "model_unloaded": true, "evidence": "v-yue2 native process group exit barrier"}, "")
		} else {
			_, e = b.call(context.Background(), c, "POST", "/permits/"+l.Permit+"/release", map[string]any{"stopped": stopped, "evidence": "v-yue2 native process group exited and weights unloaded"}, "")
		}
		if e == nil && stopped {
			b.mu.Lock()
			if b.active == l {
				b.active = nil
			}
			b.mu.Unlock()
			_ = os.Remove(filepath.Join(b.s.root, "native-lease.json"))
		}
	})
}
func (b *broker) watch(l *nativeLease, c binding) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			_, renew := b.call(ctx, c, "POST", "/permits/"+l.Permit+"/renew", map[string]any{}, "")
			_, verify := b.call(ctx, c, "POST", "/permits/"+l.Permit+"/verify", l.verify(false), "")
			b.io.Lock()
			heartbeat, e := b.call(ctx, c, "POST", "/integrations/heartbeat", map[string]any{"revision": b.revision}, "")
			b.io.Unlock()
			cancel()
			stop := renew != nil || verify != nil || e != nil
			nonce := ""
			if rows, ok := heartbeat["stop_requests"].([]any); ok {
				for _, v := range rows {
					r, _ := v.(map[string]any)
					if r["permit_id"] == l.Permit && r["execution_id"] == l.Execution {
						stop = true
						nonce, _ = r["nonce"].(string)
					}
				}
			}
			if stop {
				if nonce != "" {
					b.mu.Lock()
					l.StopNonce = nonce
					b.mu.Unlock()
					_ = b.saveLease(l)
				}
				b.finish(l)

				return
			}
		}
	}
}

func (s *server) computeAdmitted(key string) int {
	if s.broker == nil {
		return 0
	}
	s.broker.mu.Lock()
	defer s.broker.mu.Unlock()
	if s.broker.owner == key {
		return 1
	}
	return 0
}

// Heartbeats continue while idle; interrupted receipts stay private and are
// retried only after a real process-group exit barrier, never on timeout alone.
func (b *broker) maintain() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		b.mu.Lock()
		c, l := b.config, b.active
		var stopped bool
		var pid int
		var live bool
		if l != nil {
			stopped, pid, live = l.Stopped, l.PID, l.live
		}
		b.mu.Unlock()
		if len(c.Models) == 0 {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		if b.ensure(ctx, c) != nil {
			cancel()
			continue
		}
		b.io.Lock()
		reply, e := b.call(ctx, c, "POST", "/integrations/heartbeat", map[string]any{"revision": b.revision}, "")
		b.io.Unlock()
		cancel()
		if e != nil {
			b.io.Lock()
			b.revision = 0
			b.io.Unlock()
			continue
		}
		if l == nil || live {
			continue
		}
		if l.Permit == "" {
			result, err := b.call(context.Background(), c, "POST", "/permits/wait", map[string]any{"request": map[string]any{"executor_id": l.Executor, "upstream_url": computeOrigin, "model_id": l.Model, "execution_token": l.Proof, "scheduling": map[string]any{"priority": l.Priority, "preemptible": true, "allow_preemption": false, "force_admission": false}}, "wait_seconds": 0}, l.Execution)
			if err == nil {
				if id, ok := result["permit_id"].(string); ok && id != "" {
					b.mu.Lock()
					l.Permit = id
					l.Stopped = true
					b.mu.Unlock()
					_ = b.saveLease(l)
				}
			} else if time.Now().Unix() > l.WaitDeadline && strings.Contains(err.Error(), "(429)") {
				b.mu.Lock()
				if b.active == l {
					b.active = nil
				}
				b.mu.Unlock()
				_ = os.Remove(filepath.Join(b.s.root, "native-lease.json"))
			}
			continue
		}

		if !stopped && pid > 0 && errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH) {
			b.mu.Lock()
			l.Stopped = true
			b.mu.Unlock()
			stopped = true
			_ = b.saveLease(l)
		}
		if !stopped {
			continue
		}
		ackFailed := false
		if rows, ok := reply["stop_requests"].([]any); ok {
			for _, v := range rows {
				r, _ := v.(map[string]any)
				if r["permit_id"] == l.Permit && r["execution_id"] == l.Execution {
					_, ackErr := b.call(context.Background(), c, "POST", "/permits/"+l.Permit+"/stopped", map[string]any{"nonce": r["nonce"], "execution_id": l.Execution, "execution_token": l.Proof, "native_exit_barrier": true, "model_unloaded": true, "evidence": "v-yue2 observed native process group exit"}, "")
					ackFailed = ackFailed || ackErr != nil
				}
			}
		}
		if ackFailed {
			continue
		}
		b.cacheStoppedJob(l)
		if _, err := b.call(context.Background(), c, "POST", "/permits/"+l.Permit+"/release", map[string]any{"stopped": true, "evidence": "v-yue2 observed native process group exit"}, ""); err == nil {
			b.mu.Lock()
			if b.active == l {
				b.active = nil
			}
			b.mu.Unlock()
			_ = os.Remove(filepath.Join(b.s.root, "native-lease.json"))
		}
	}
}

func (b *broker) saveLease(l *nativeLease) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return privateJSON(filepath.Join(b.s.root, "native-lease.json"), l)
}
func (b *broker) isStopped(l *nativeLease) bool { b.mu.Lock(); defer b.mu.Unlock(); return l.Stopped }

// Legacy VOS application contexts may use user/check; its verified account
// identity must still be exactly the enabled Scheduler service account.
func (s *server) schedulerIdentity(r *http.Request) bool {
	if id, _, err := s.authenticate(r); err == nil && id.Sub == "com.ictrek.v-scheduler" {
		return true
	}
	target, err := url.Parse(s.userinfo)
	if err != nil {
		return false
	}
	target.Path = "/v1000/user/check"
	target.RawQuery = ""
	target.Fragment = ""
	req, err := http.NewRequestWithContext(r.Context(), "GET", target.String(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", r.Header.Get("Authorization"))
	client := *s.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var v struct {
		Code *int `json:"code"`
		Data struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role_code"`
			Enable   int    `json:"enable"`
			Deleted  *int   `json:"delete"`
		} `json:"data"`
	}
	return res.StatusCode == 200 && json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&v) == nil && v.Code != nil && *v.Code == 0 && v.Data.ID != "" && v.Data.Username == "com.ictrek.v-scheduler" && v.Data.Role == "system_admin" && v.Data.Enable == 1 && v.Data.Deleted != nil && *v.Data.Deleted == 0
}

func engineHash() string {
	body, err := os.ReadFile(env("YUE_ENGINE_BIN", "/opt/v-yue2/engine/yue-server"))
	if err != nil {
		return "unavailable"
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (s *server) terminalPath(key, kind, id string) string {
	return filepath.Join(s.root, "users", key, "jobs", strings.ReplaceAll(kind, "/", "_")+"-"+id+".json")
}
func (s *server) serveTerminal(w http.ResponseWriter, r *http.Request, key, path string) bool {
	if r.Method != "GET" && r.Method != "DELETE" {
		return false
	}
	for _, kind := range []string{"/v1/music/jobs", "/v1/scores", "/v1/transcriptions"} {
		if !strings.HasPrefix(path, kind+"/") {
			continue
		}
		id := strings.TrimPrefix(path, kind+"/")
		if len(id) < 8 || len(id) > 128 || !filepath.IsLocal(id) || strings.ContainsAny(id, "/\\.") {
			return false
		}
		target := s.terminalPath(key, kind, id)
		body, err := os.ReadFile(target)
		if err != nil && r.Method == "GET" && s.broker != nil {
			s.broker.mu.Lock()
			active := s.broker.active
			if active != nil && active.Account == key && active.Kind == kind && active.JobID == id {
				body = append([]byte{}, active.Job...)
				err = nil
			}
			s.broker.mu.Unlock()
		}
		if err != nil {
			return false
		}
		if r.Method == "DELETE" {
			_ = os.Remove(target)
			reply(w, 200, map[string]bool{"deleted": true})
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}
		return true
	}
	return false
}

func (b *broker) cacheStoppedJob(l *nativeLease) {
	b.mu.Lock()
	account, kind, id, body := l.Account, l.Kind, l.JobID, append([]byte{}, l.Job...)
	b.mu.Unlock()
	if account == "" || id == "" {
		return
	}
	path := b.s.terminalPath(account, kind, id)
	if _, err := os.Stat(path); err == nil {
		return
	}
	var job map[string]any
	if json.Unmarshal(body, &job) != nil {
		job = map[string]any{"id": id}
	}
	job["status"] = "cancelled"
	job["phase"] = "failed"
	job["message"] = "Stopped by V-Scheduler; native processes exited and models unloaded"
	_ = privateJSON(path, job)
}
