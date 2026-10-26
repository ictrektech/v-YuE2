// v-YuE2's VOS boundary. Each authenticated account owns a separate upstream
// process and data directory; only model files are shared through V-Scheduler.
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
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const prefix = "/app/com.ictrek.v-yue2"
const licenseRevision = "yue2-2026-09-16-v1"

type identity struct {
	Sub         string   `json:"sub"`
	Enable      int      `json:"enable"`
	Role        string   `json:"role_code"`
	Permissions []string `json:"permission_code_list"`
}
type worker struct {
	cmd  *exec.Cmd
	port int
	log  *os.File
	done chan struct{}
}
type session struct {
	token   string
	expires time.Time
}
type server struct {
	root, web, userinfo string
	client              *http.Client
	mu                  sync.Mutex
	compute             sync.Mutex
	users               map[string]*worker
	sessions            map[string]session
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]string{"error": message})
}
func userKey(sub string) string { h := sha256.Sum256([]byte(sub)); return hex.EncodeToString(h[:]) }
func (s *server) authenticate(r *http.Request) (identity, string, error) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		if c, e := r.Cookie("v_yue2_session"); e == nil {
			s.mu.Lock()
			v, ok := s.sessions[c.Value]
			s.mu.Unlock()
			if ok && time.Now().Before(v.expires) {
				token = v.token
			}
		}
	}
	if token == "" {
		return identity{}, "", errors.New("VOS sign-in required")
	}
	req, e := http.NewRequestWithContext(r.Context(), "GET", s.userinfo, nil)
	if e != nil {
		return identity{}, "", e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, e := s.client.Do(req)
	if e != nil {
		return identity{}, "", e
	}
	defer res.Body.Close()
	var id identity
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&id) != nil || id.Sub == "" || id.Enable != 1 {
		return identity{}, "", errors.New("VOS identity rejected")
	}
	return id, token, nil
}
func admin(id identity) bool {
	if id.Role == "system_admin" || id.Role == "platform_admin" {
		return true
	}
	for _, p := range id.Permissions {
		if p == "com.ictrek.v-yue2:model:manage" {
			return true
		}
	}
	return false
}
func freePort() (int, error) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return 0, e
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
func (s *server) studio(key string) (*worker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.users[key]; v != nil {
		return v, nil
	}
	if len(s.users) >= 16 {
		return nil, errors.New("Studio account capacity reached; restart the app to release idle sessions")
	}
	port, e := freePort()
	if e != nil {
		return nil, e
	}
	engine, e := freePort()
	if e != nil {
		return nil, e
	}
	root := filepath.Join(s.root, "users", key)
	if e = os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(root, "service.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	cmd := exec.Command(env("YUE_STUDIO_BIN", "/opt/v-yue2/music-server"))
	cmd.Env = append(os.Environ(), "YUE_VOS=1", "YUE_STUDIO_DATA_ROOT="+root, "YUE_STUDIO_PORT="+fmt.Sprint(port), "YUE_ENGINE_PORT="+fmt.Sprint(engine), "YUE_ENGINE_BASE_URL=http://127.0.0.1:"+fmt.Sprint(engine), "YUE_MODELS_ROOT="+filepath.Join(root, "models"), "STUDIO_TELEMETRY=0", "DO_NOT_TRACK=1")
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if e = cmd.Start(); e != nil {
		f.Close()
		return nil, e
	}
	v := &worker{cmd: cmd, port: port, log: f, done: make(chan struct{})}
	s.users[key] = v
	go func() {
		_ = cmd.Wait()
		close(v.done)
		f.Close()
		s.mu.Lock()
		if s.users[key] == v {
			delete(s.users, key)
		}
		s.mu.Unlock()
	}()
	for i := 0; i < 100; i++ {
		c, e := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if e == nil {
			c.Close()
			return v, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	return nil, errors.New("Studio did not start; see the account service log")
}
func (s *server) accepted(key string) bool {
	b, e := os.ReadFile(filepath.Join(s.root, "users", key, "license.json"))
	if e != nil {
		return false
	}
	var v struct{ Revision string }
	return json.Unmarshal(b, &v) == nil && v.Revision == licenseRevision
}
func restricted(path string) bool {
	// Desktop command execution, arbitrary server paths, network exposure and
	// software installation are not browser capabilities in the VOS product.
	for _, p := range []string{"/engine/pick-executable", "/engine/options", "/v1/network", "/v1/files/", "/v1/open-data-directory", "/v1/processing/vst", "/v1/assistant/cli-agents", "/setup/adopt", "/v1/adapters/import-folder", "/v1/training/"} {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
func terminal(status string) bool {
	return status == "completed" || status == "failed" || status == "cancelled"
}
func (s *server) followCompute(v *worker, path, id string, release func()) {
	defer release()
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-v.done:
			return
		case <-timer.C:
			res, err := s.client.Get(fmt.Sprintf("http://127.0.0.1:%d%s/%s", v.port, path, url.PathEscape(id)))
			if err != nil {
				continue
			}
			var job struct {
				Status string `json:"status"`
			}
			err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&job)
			res.Body.Close()
			if err == nil && terminal(job.Status) {
				return
			}
		}
	}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	appRequest := strings.HasPrefix(r.URL.Path, prefix)
	path := strings.TrimPrefix(r.URL.Path, prefix)
	if path == "" {
		path = "/"
	}
	r.URL.Path = path
	r.URL.RawPath = ""
	if path == "/health" && !appRequest && r.Method == "GET" {
		reply(w, 200, map[string]string{"status": "ok", "app": "v-YuE2", "version": env("APP_VERSION", "development")})
		return
	}
	api := path == "/health" || strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/vos/") || strings.HasPrefix(path, "/setup/") || strings.HasPrefix(path, "/engine/") || strings.HasPrefix(path, "/mcp")
	if !api {
		if (r.Method == "GET" || r.Method == "HEAD") && filepath.Ext(path) == "" {
			publicPath := filepath.Join(s.web, strings.TrimPrefix(filepath.Clean("/"+path), "/"))
			if _, err := os.Stat(publicPath); os.IsNotExist(err) {
				r.URL.Path = "/"
			}
		}
		http.FileServer(http.Dir(s.web)).ServeHTTP(w, r)
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host {
				fail(w, 403, "Cross-origin request rejected")
				return
			}
		}
	}
	id, token, e := s.authenticate(r)
	if e != nil {
		fail(w, 401, "VOS sign-in required / 请从 VOS 登录")
		return
	}
	key := userKey(id.Sub)
	if path == "/vos/session" && r.Method == "POST" {
		b := make([]byte, 32)
		if _, e = rand.Read(b); e != nil {
			fail(w, 500, "Session unavailable")
			return
		}
		cookie := hex.EncodeToString(b)
		s.mu.Lock()
		for k, v := range s.sessions {
			if time.Now().After(v.expires) {
				delete(s.sessions, k)
			}
		}
		s.sessions[cookie] = session{token, time.Now().Add(time.Hour)}
		s.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "v_yue2_session", Value: cookie, Path: prefix + "/", HttpOnly: true, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteStrictMode, MaxAge: 3600})
		reply(w, 200, map[string]any{"sub": id.Sub, "can_manage_models": admin(id), "accepted": s.accepted(key), "revision": licenseRevision})
		return
	}
	if path == "/vos/license" && r.Method == "POST" {
		var v struct {
			Revision string
			Accepted bool
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&v) != nil || !v.Accepted || v.Revision != licenseRevision {
			fail(w, 400, "Confirm the current model licenses")
			return
		}
		dir := filepath.Join(s.root, "users", key)
		if os.MkdirAll(dir, 0700) != nil {
			fail(w, 500, "Cannot save consent")
			return
		}
		b, _ := json.Marshal(map[string]any{"Revision": licenseRevision, "accepted_at": time.Now().UTC()})
		if os.WriteFile(filepath.Join(dir, "license.json"), b, 0600) != nil {
			fail(w, 500, "Cannot save consent")
			return
		}
		reply(w, 200, map[string]bool{"accepted": true})
		return
	}
	if path == "/vos/about" {
		reply(w, 200, map[string]any{"name": "v-YuE2", "version": env("APP_VERSION", "development"), "profile": env("YUE_PROFILE", "unknown"), "image": env("YUE_IMAGE", "unknown"), "modelscope": "vivibit/YuE2-Studio-GGUF", "models_bundled": false})
		return
	}
	if !s.accepted(key) {
		fail(w, 428, "Model license confirmation required")
		return
	}
	if path == "/setup/download" || path == "/setup/remove" || path == "/setup/cancel" {
		if !admin(id) {
			fail(w, 403, "Model administrator required / 需要模型管理权限")
			return
		}
	}
	if path == "/setup/remove" {
		fail(w, 409, "Shared models are managed in V-Scheduler / 请在 V-Scheduler 管理共享模型")
		return
	}
	if restricted(path) && r.Method != "GET" {
		fail(w, 403, "This desktop operation is unavailable in VOS")
		return
	}
	if r.Method != "GET" && strings.Contains(path, "/install") {
		fail(w, 403, "请使用准备模型功能；此附加模型尚未列入 VOS 目录 / Use Prepare models; this additional model is not in the VOS catalog")
		return
	}
	if strings.HasPrefix(path, "/mcp") {
		fail(w, 403, "Desktop automation is unavailable in VOS")
		return
	}
	if path == "/v1/hub/state" {
		reply(w, 200, map[string]any{"telemetry": map[string]any{"enabled": false, "acknowledged": true, "disabledByEnv": true}, "items": []any{}})
		return
	}
	if strings.HasPrefix(path, "/v1/hub/") {
		reply(w, 200, map[string]bool{"ok": true})
		return
	}
	v, e := s.studio(key)
	if e != nil {
		fail(w, 503, e.Error())
		return
	}
	computePath := ""
	if r.Method == "POST" {
		switch path {
		case "/v1/music/jobs", "/v1/music/replay":
			computePath = "/v1/music/jobs"
		case "/v1/scores":
			computePath = "/v1/scores"
		case "/v1/transcriptions":
			computePath = "/v1/transcriptions"
		}
	}
	if computePath != "" && !s.compute.TryLock() {
		fail(w, 409, "本机正在处理其他创作任务，请稍后重试 / Another creation task is running; try again shortly")
		return
	}
	var released sync.Once
	release := func() {
		if computePath != "" {
			released.Do(s.compute.Unlock)
		}
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 256<<20)
	}
	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", v.port))
	proxy := httputil.NewSingleHostReverseProxy(target)
	old := proxy.Director
	proxy.Director = func(req *http.Request) {
		old(req)
		req.Host = target.Host
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		req.Header.Del("Origin")
		req.Header.Del("Forwarded")
		req.Header.Del("X-Forwarded-For")
		// ReverseProxy treats a nil value as an explicit instruction not to
		// append its own forwarding header; the private loopback service uses
		// this distinction for its desktop/network guard.
		req.Header["X-Forwarded-For"] = nil
		req.Header.Del("X-Forwarded-Host")
		req.Header.Del("X-Real-IP")
		req.Header.Del("Cf-Connecting-IP")
	}
	if computePath != "" {
		proxy.ModifyResponse = func(res *http.Response) error {
			body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
			res.Body.Close()
			res.Body = io.NopCloser(bytes.NewReader(body))
			var job struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}
			if err != nil {
				return err
			}
			if res.StatusCode >= 400 {
				release()
				return nil
			}
			if json.Unmarshal(body, &job) != nil || job.ID == "" {
				return errors.New("unknown submission result")
			}
			if terminal(job.Status) {
				release()
			} else {
				go s.followCompute(v, computePath, job.ID, release)
			}
			return nil
		}
	}
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(out http.ResponseWriter, _ *http.Request, _ error) {
		if computePath != "" {
			_ = syscall.Kill(-v.cmd.Process.Pid, syscall.SIGTERM)
			go func() { <-v.done; release() }()
		}
		fail(out, 502, "Studio temporarily unavailable")
	}
	proxy.ServeHTTP(w, r)
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		c := &http.Client{Timeout: 3 * time.Second}
		r, e := c.Get("http://127.0.0.1:8791/health")
		if e != nil {
			os.Exit(1)
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	s := &server{root: env("YUE_DATA_ROOT", "/data"), web: env("YUE_WEB_ROOT", "/opt/v-yue2/web"), userinfo: env("VOS_USERINFO_URL", "http://host.docker.internal:1182/v1000/oauth2/userinfo"), client: &http.Client{Timeout: 10 * time.Second}, users: map[string]*worker{}, sessions: map[string]session{}}
	h := &http.Server{Addr: env("YUE_LISTEN", ":8791"), Handler: s, ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 65536}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = h.Shutdown(ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, v := range s.users {
			_ = syscall.Kill(-v.cmd.Process.Pid, syscall.SIGTERM)
		}
	}()
	log.Print("v-YuE2 VOS service ready")
	if e := h.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
		log.Fatal(e)
	}
}
