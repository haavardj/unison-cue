// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package settings

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/haavardj/unison-cue/internal/core"
)

type fakeBackend struct {
	cfg    core.Config
	opened string
}

func (f *fakeBackend) Snapshot() State { return State{Config: f.cfg} }
func (f *fakeBackend) Apply(u Update) (State, map[string]string, error) {
	if errs := u.Config.Validate(); len(errs) > 0 {
		return State{}, errs, nil
	}
	f.cfg = u.Config
	return f.Snapshot(), nil, nil
}
func (f *fakeBackend) Open(what string) error { f.opened = what; return nil }

func start(t *testing.T) (*Server, *fakeBackend) {
	t.Helper()
	b := &fakeBackend{cfg: *core.DefaultConfig()}
	s, err := Start(b)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, b
}

// session opens the page the way the window does and returns a client
// holding the session cookie.
func session(t *testing.T, s *Server) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	res, err := c.Get(s.URL())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "Unison Cue Settings") {
		t.Fatalf("page: %d %s", res.StatusCode, body)
	}
	if res.Request.URL.RawQuery != "" {
		t.Errorf("token left in the address: %s", res.Request.URL)
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP = %q", csp)
	}
	return c
}

func base(s *Server) string { return "http://" + s.host() }

func TestRequiresToken(t *testing.T) {
	s, _ := start(t)
	for _, path := range []string{"/", "/api/state", "/app.js", "/?token=wrong"} {
		res, err := http.Get(base(s) + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s without session: %d", path, res.StatusCode)
		}
	}
}

func TestRejectsForeignHost(t *testing.T) {
	s, _ := start(t)
	c := session(t, s)
	req, _ := http.NewRequest("GET", base(s)+"/api/state", nil)
	req.Host = "evil.example:" + strings.Split(s.host(), ":")[1]
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("rebinding host accepted: %d", res.StatusCode)
	}
}

func post(t *testing.T, c *http.Client, u, ctype, origin, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", u, strings.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestStateRoundTripAndCSRF(t *testing.T) {
	s, b := start(t)
	c := session(t, s)

	res, err := c.Get(base(s) + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var st State
	json.NewDecoder(res.Body).Decode(&st)
	res.Body.Close()
	if st.Config.ReminderHours != 24 {
		t.Fatalf("state = %+v", st.Config)
	}

	st.Config.ReminderHours = 48
	st.Config.Environment = map[string]string{"UNISONLOCALHOSTNAME": "laptop"}
	body, _ := json.Marshal(Update{Config: st.Config})

	// A form post from another site: wrong type, wrong origin.
	r := post(t, c, base(s)+"/api/state", "application/x-www-form-urlencoded", "", "x=1")
	r.Body.Close()
	if r.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("form post: %d", r.StatusCode)
	}
	r = post(t, c, base(s)+"/api/state", "application/json", "http://evil.example", string(body))
	r.Body.Close()
	if r.StatusCode != http.StatusForbidden {
		t.Errorf("foreign origin: %d", r.StatusCode)
	}
	if b.cfg.ReminderHours != 24 {
		t.Fatal("rejected request changed the settings")
	}

	r = post(t, c, base(s)+"/api/state", "application/json", base(s), string(body))
	r.Body.Close()
	if r.StatusCode != 200 || b.cfg.ReminderHours != 48 || b.cfg.Environment["UNISONLOCALHOSTNAME"] != "laptop" {
		t.Errorf("save: %d, cfg = %+v", r.StatusCode, b.cfg)
	}

	// Invalid values come back as field errors.
	st.Config.ReminderHours = 0
	st.Config.Environment = map[string]string{"BAD NAME": "x"}
	body, _ = json.Marshal(Update{Config: st.Config})
	r = post(t, c, base(s)+"/api/state", "application/json", base(s), string(body))
	var e struct {
		Fields map[string]string `json:"fields"`
	}
	json.NewDecoder(r.Body).Decode(&e)
	r.Body.Close()
	if r.StatusCode != http.StatusUnprocessableEntity || e.Fields["reminderHours"] == "" || e.Fields["environment"] == "" {
		t.Errorf("validation: %d %v", r.StatusCode, e.Fields)
	}

	// Unknown fields are refused rather than silently dropped.
	r = post(t, c, base(s)+"/api/state", "application/json", base(s), `{"config":{},"bogus":1}`)
	r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown field: %d", r.StatusCode)
	}

	r = post(t, c, base(s)+"/api/open", "application/json", base(s), `{"what":"logs"}`)
	r.Body.Close()
	if r.StatusCode != 200 || b.opened != "logs" {
		t.Errorf("open: %d %q", r.StatusCode, b.opened)
	}
}

func TestStaticFiles(t *testing.T) {
	s, _ := start(t)
	c := session(t, s)
	for path, ctype := range map[string]string{"/app.js": "javascript", "/app.css": "text/css", "/logo.svg": "image/svg+xml"} {
		res, err := c.Get(base(s) + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Type"), ctype) {
			t.Errorf("%s: %d %s", path, res.StatusCode, res.Header.Get("Content-Type"))
		}
	}
	u, _ := url.Parse(s.URL())
	if u.Hostname() != "127.0.0.1" {
		t.Errorf("listening on %s", u.Hostname())
	}
}
