package api

import (
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

var browserUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u := origin
		if i := strings.Index(u, "://"); i >= 0 {
			u = u[i+3:]
		}
		return u == r.Host
	},
}

func (s *Server) fileLSPStatus(w http.ResponseWriter, r *http.Request) {
	ok, err := s.Agent.FileLSPStatus(r.URL.Query().Get("lang"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "lang": r.URL.Query().Get("lang")})
}

func (s *Server) fileLSP(w http.ResponseWriter, r *http.Request) {
	username, root, ok := s.fileUser(w, r, r.URL.Query().Get("user"))
	if !ok {
		return
	}
	q := r.URL.Query()
	q.Set("user", username)
	if root {
		q.Set("root", "1")
	} else {
		q.Del("root")
	}
	dialer := websocket.Dialer{
		NetDial: func(_, _ string) (net.Conn, error) {
			return net.Dial("unix", s.Cfg.SocketPath)
		},
	}
	agentConn, resp, err := dialer.Dial("ws://agent/files/lsp?"+q.Encode(), nil)
	if err != nil {
		msg := err.Error()
		if resp != nil {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			_ = resp.Body.Close()
			if len(b) > 0 {
				msg = string(b)
			}
		}
		http.Error(w, msg, http.StatusBadGateway)
		return
	}
	defer agentConn.Close()

	browserConn, err := browserUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer browserConn.Close()

	errc := make(chan struct{}, 2)
	go copyWS(agentConn, browserConn, errc)
	go copyWS(browserConn, agentConn, errc)
	<-errc
}

func copyWS(dst, src *websocket.Conn, errc chan struct{}) {
	defer func() { errc <- struct{}{} }()
	for {
		typ, msg, err := src.ReadMessage()
		if err != nil {
			return
		}
		if err := dst.WriteMessage(typ, msg); err != nil {
			return
		}
	}
}
