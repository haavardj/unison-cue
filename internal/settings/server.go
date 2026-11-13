// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

// Package settings serves the settings page shown in the app's settings
// window. The server listens on 127.0.0.1 only and accepts requests only
// with a random per-window token, from its own origin, so other local
// programs and web pages cannot read or change the settings.
package settings

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/haavardj/unison-cue/internal/core"
	"github.com/haavardj/unison-cue/internal/icon"
)

//go:embed web
var webFS embed.FS

// Profile is one profile as shown on the Profiles tab.
type Profile struct {
	Name     string               `json:"name"`
	Label    string               `json:"label,omitempty"`
	Roots    []string             `json:"roots"`
	Path     string               `json:"path"`
	Settings core.ProfileSettings `json:"settings"`
}

// EnvVar is a variable unison will see, and where its value comes from.
type EnvVar struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Source string `json:"source"` // "settings", "shell" or "system"
}

// Info is read-only information for the About tab and placeholders.
type Info struct {
	Version          string   `json:"version"`
	Platform         string   `json:"platform"` // GOOS
	DetectedUnison   string   `json:"detectedUnison"`
	UnisonVersion    string   `json:"unisonVersion"`
	DefaultDir       string   `json:"defaultProfileDir"`
	ProfileDir       string   `json:"profileDir"`
	ConfigPath       string   `json:"configPath"`
	LogDir           string   `json:"logDir"`
	ShellImportNames []string `json:"shellImportNames"`
}

// State is what the page loads and edits.
type State struct {
	Config    core.Config `json:"config"`
	Autostart bool        `json:"autostart"`
	Profiles  []Profile   `json:"profiles"`
	Env       []EnvVar    `json:"env"`
	Info      Info        `json:"info"`
}

// Update is what the page sends when the user saves.
type Update struct {
	Config    core.Config `json:"config"`
	Autostart bool        `json:"autostart"`
}

// Backend is implemented by the app.
type Backend interface {
	Snapshot() State
	// Apply validates and saves the update, returning field errors
	// (keyed by setting name) if it is rejected.
	Apply(Update) (State, map[string]string, error)
	// Open opens one of "config", "logs" or "profiles" in the file manager.
	Open(what string) error
}

// Server serves the settings page for one window.
type Server struct {
	backend Backend
	token   string
	ln      net.Listener
	srv     *http.Server
}

const cookieName = "uc_session"

// Start listens on a random loopback port.
func Start(b Backend) (*Server, error) {
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{backend: b, token: hex.EncodeToString(tok), ln: ln}
	s.srv = &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go s.srv.Serve(ln)
	return s, nil
}

// URL is the address the settings window opens. It carries the token
// once; the page then uses an HttpOnly cookie.
func (s *Server) URL() string {
	return "http://" + s.host() + "/?token=" + s.token
}

func (s *Server) host() string { return s.ln.Addr().String() }

// Close stops the server.
func (s *Server) Close() error { return s.srv.Close() }

func (s *Server) validToken(t string) bool {
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1
}

func (s *Server) handler() http.Handler {
	static, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(static))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.backend.Snapshot())
	})
	mux.HandleFunc("POST /api/state", func(w http.ResponseWriter, r *http.Request) {
		var u Update
		if err := decodeJSON(r, &u); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		st, fieldErrs, err := s.backend.Apply(u)
		if len(fieldErrs) > 0 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "Some settings are not valid.", "fields": fieldErrs})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("POST /api/open", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			What string `json:"what"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if err := s.backend.Open(req.What); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{})
	})
	mux.HandleFunc("GET /logo.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(icon.LogoSVG()))
	})
	mux.Handle("GET /", files)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")

		// DNS rebinding: a hostile page cannot make the browser send our
		// exact Host header, so anything else is refused.
		if r.Host != s.host() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// First load: exchange the token in the URL for a cookie and
		// drop it from the address.
		if t := r.URL.Query().Get("token"); t != "" && r.Method == http.MethodGet && r.URL.Path == "/" {
			if !s.validToken(t) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: s.token, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		c, err := r.Cookie(cookieName)
		if err != nil || !s.validToken(c.Value) {
			http.Error(w, "This page only opens from Unison Cue's Settings menu item.", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			// Cross-site requests: only our own origin, only JSON (which
			// a plain HTML form cannot send).
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+s.host() {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.New("invalid request: " + err.Error())
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
