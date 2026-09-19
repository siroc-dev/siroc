package api

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/siroc-dev/siroc/internal/validate"
)

func (s *Server) terminalWS(w http.ResponseWriter, r *http.Request) {
	username, root, ok := s.fileUser(w, r, r.URL.Query().Get("user"))
	if !ok {
		return
	}
	if !root {
		acc, aok := s.accountOrErr(w, r, username)
		if !aok {
			return
		}
		if acc.Suspended {
			writeErr(w, http.StatusForbidden, fmt.Errorf("account is suspended"))
			return
		}
	}
	q := r.URL.Query()
	q.Set("user", username)
	if root {
		q.Set("root", "1")
	} else {
		q.Del("root")
	}
	if cwd := strings.TrimSpace(q.Get("cwd")); cwd != "" {
		abs, err := validate.TermCWD(s.Cfg.HomeRoot, username, cwd, root)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		q.Set("cwd", abs)
	} else {
		q.Del("cwd")
	}
	dialer := websocket.Dialer{
		NetDial: func(_, _ string) (net.Conn, error) {
			return net.Dial("unix", s.Cfg.SocketPath)
		},
	}
	agentConn, resp, err := dialer.Dial("ws://agent/term?"+q.Encode(), nil)
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
