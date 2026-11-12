package relay

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// HTTPHandler builds the REST API mux. Requires Go 1.22+ pattern routing.
//
// Routes:
//
//	GET  /healthz                      liveness
//	GET  /readyz                       readiness
//	GET  /api/tvs                      list configured TVs
//	GET  /api/pubkey                   ADB public key (to pre-authorize on a TV)
//	POST /api/tv/{name}/cmd/{command}  run a symbolic command
//	POST /api/tv/{name}/power/{state}  state = on|off|toggle
//	POST /api/tv/{name}/input/{n}      HDMI input 1..4
//	POST /api/tv/{name}/key/{code}     raw Android key code
//	POST /api/cmd/{command}            convenience when a single TV is configured
func (m *Manager) HTTPHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("GET /api/tvs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"tvs": m.Names()})
	})
	mux.HandleFunc("GET /api/pubkey", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, m.PublicKey())
	})

	mux.HandleFunc("POST /api/tv/{name}/cmd/{command}", func(w http.ResponseWriter, r *http.Request) {
		m.run(w, r.PathValue("name"), r.PathValue("command"))
	})
	mux.HandleFunc("POST /api/tv/{name}/power/{state}", func(w http.ResponseWriter, r *http.Request) {
		m.run(w, r.PathValue("name"), r.PathValue("state"))
	})
	mux.HandleFunc("POST /api/tv/{name}/input/{n}", func(w http.ResponseWriter, r *http.Request) {
		m.run(w, r.PathValue("name"), "hdmi"+r.PathValue("n"))
	})
	mux.HandleFunc("POST /api/tv/{name}/key/{code}", func(w http.ResponseWriter, r *http.Request) {
		m.run(w, r.PathValue("name"), "key:"+r.PathValue("code"))
	})
	mux.HandleFunc("POST /api/cmd/{command}", func(w http.ResponseWriter, r *http.Request) {
		m.run(w, "", r.PathValue("command"))
	})

	return logRequests(mux)
}

func (m *Manager) run(w http.ResponseWriter, tv, command string) {
	if err := m.Do(tv, command); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok": false, "tv": tv, "command": command, "error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tv": tv, "command": command})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
