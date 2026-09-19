//go:build linux

package tty

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

type clientMsg struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

func Serve(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("user")
	root := r.URL.Query().Get("root") == "1" || username == "root"
	cols := parseSize(r.URL.Query().Get("cols"), 120)
	rows := parseSize(r.URL.Query().Get("rows"), 32)

	cmd, err := shellCmd(username, root, r.URL.Query().Get("cwd"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("terminal: "+err.Error()+"\r\n"))
		return
	}
	defer func() {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP)
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	var wsMu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				wsMu.Lock()
				werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n])
				wsMu.Unlock()
				if werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Minute))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg clientMsg
		if json.Unmarshal(raw, &msg) != nil {
			_, _ = ptmx.Write(raw)
			continue
		}
		switch msg.Type {
		case "ping":
		case "in":
			if msg.Data != "" {
				_, _ = io.WriteString(ptmx, msg.Data)
			}
		case "resize":
			c, rw := msg.Cols, msg.Rows
			if c == 0 {
				c = cols
			}
			if rw == 0 {
				rw = rows
			}
			_ = pty.Setsize(ptmx, &pty.Winsize{Cols: clampSize(c), Rows: clampSize(rw)})
		}
	}
	<-done
}

func shellCmd(username string, root bool, cwd string) (*exec.Cmd, error) {
	name := username
	if root {
		name = "root"
	}
	if !root {
		if err := validUser(name); err != nil {
			return nil, err
		}
	}
	u, err := user.Lookup(name)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	home := u.HomeDir
	if home == "" {
		home = "/"
	}
	dir := home
	if cwd != "" {
		p := filepath.Clean(cwd)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			if root {
				dir = p
			} else {
				homeAbs := filepath.Clean(home)
				if p == homeAbs || strings.HasPrefix(p, homeAbs+string(os.PathSeparator)) {
					dir = p
				}
			}
		}
	}
	cmd := exec.Command("/bin/bash", "-l")
	cmd.Dir = dir
	cmd.Env = []string{
		"HOME=" + home,
		"USER=" + name,
		"LOGNAME=" + name,
		"SHELL=/bin/bash",
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:" + home + "/bin",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:    true,
		Pdeathsig: syscall.SIGKILL,
	}
	if !root {
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	}
	return cmd, nil
}

func validUser(name string) error {
	if name == "" || strings.ContainsAny(name, "/:\n\r\x00") {
		return fmt.Errorf("invalid user")
	}
	if name == "root" {
		return fmt.Errorf("invalid user")
	}
	return nil
}

func parseSize(v string, fallback uint16) uint16 {
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return clampSize(uint16(n))
}

func clampSize(n uint16) uint16 {
	if n < 10 {
		return 10
	}
	if n > 400 {
		return 400
	}
	return n
}
