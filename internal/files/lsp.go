//go:build linux

package files

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

func ServeLSP(w http.ResponseWriter, r *http.Request, m *Manager, installRoot string) {
	lang := r.URL.Query().Get("lang")
	workspace := r.URL.Query().Get("workspace")
	username := r.URL.Query().Get("user")
	root := r.URL.Query().Get("root") == "1"
	cmdPath, args, err := lspCommand(lang, installRoot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	dir, uid, gid, home, err := m.ResolveLSP(username, workspace, root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	cache := filepath.Join(home, ".cache")
	cmd := exec.Command(cmdPath, args...)
	cmd.Dir = dir
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"TERM=xterm",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"NODE_PATH=" + filepath.Join(installRoot, "lsp", "node_modules"),
		"XDG_CACHE_HOME=" + cache,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"TMPDIR=/tmp",
	}
	if !root {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
		}
	} else {
		cmd.Env = append([]string{}, cmd.Env...)
		for i, e := range cmd.Env {
			if strings.HasPrefix(e, "HOME=") {
				cmd.Env[i] = "HOME=/root"
			}
			if strings.HasPrefix(e, "XDG_CACHE_HOME=") {
				cmd.Env[i] = "XDG_CACHE_HOME=/root/.cache"
			}
			if strings.HasPrefix(e, "XDG_CONFIG_HOME=") {
				cmd.Env[i] = "XDG_CONFIG_HOME=/root/.config"
			}
		}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"jsonrpc":"2.0","method":"window/showMessage","params":{"type":1,"message":"language server failed to start"}}`))
		return
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	var wsMu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		br := bufio.NewReader(stdout)
		for {
			body, err := readLSPFrame(br)
			if err != nil {
				return
			}
			wsMu.Lock()
			err = conn.WriteMessage(websocket.TextMessage, body)
			wsMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	patched := false
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if !patched {
			if next, ok := patchInitialize(msg, dir, home, lang); ok {
				msg = next
				patched = true
			}
		}
		if err := writeLSPFrame(stdin, msg); err != nil {
			break
		}
	}
	_ = stdin.Close()
	<-done
}

func lspCommand(lang, installRoot string) (string, []string, error) {
	binDir := filepath.Join(installRoot, "lsp", "node_modules", ".bin")
	resolve := func(name string, args []string, missing string) (string, []string, error) {
		p := filepath.Join(binDir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, args, nil
		}
		if lp, err := exec.LookPath(name); err == nil {
			return lp, args, nil
		}
		return "", nil, fmt.Errorf("%s", missing)
	}
	switch lang {
	case "php":
		return resolve("intelephense", []string{"--stdio"}, "PHP language server is not installed")
	case "yaml":
		return resolve("yaml-language-server", []string{"--stdio"}, "YAML language server is not installed")
	case "python":
		if p, args, err := resolve("pyright-langserver", []string{"--stdio"}, ""); err == nil {
			return p, args, nil
		}
		if p, err := exec.LookPath("pylsp"); err == nil {
			return p, nil, nil
		}
		if p, err := exec.LookPath("pyright-langserver"); err == nil {
			return p, []string{"--stdio"}, nil
		}
		return "", nil, fmt.Errorf("Python language server is not installed")
	case "javascript", "typescript":
		return resolve("typescript-language-server", []string{"--stdio"}, "TypeScript language server is not installed")
	case "html":
		return resolve("vscode-html-language-server", []string{"--stdio"}, "HTML language server is not installed")
	case "css":
		return resolve("vscode-css-language-server", []string{"--stdio"}, "CSS language server is not installed")
	case "json":
		return resolve("vscode-json-language-server", []string{"--stdio"}, "JSON language server is not installed")
	case "shell":
		return resolve("bash-language-server", []string{"start"}, "Bash language server is not installed")
	case "dockerfile":
		return resolve("docker-langserver", []string{"--stdio"}, "Dockerfile language server is not installed")
	default:
		return "", nil, fmt.Errorf("no language server for %s", lang)
	}
}

func HasLSP(lang, installRoot string) bool {
	path, _, err := lspCommand(lang, installRoot)
	if err != nil {
		return false
	}
	if filepath.IsAbs(path) {
		_, err := os.Stat(path)
		return err == nil
	}
	_, err = exec.LookPath(path)
	return err == nil
}

func readLSPFrame(r *bufio.Reader) ([]byte, error) {
	n := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			n, _ = strconv.Atoi(strings.TrimSpace(line[len("Content-Length:"):]))
		}
	}
	if n <= 0 || n > 8<<20 {
		return nil, io.ErrUnexpectedEOF
	}
	body := make([]byte, n)
	_, err := io.ReadFull(r, body)
	return body, err
}

func writeLSPFrame(w io.Writer, body []byte) error {
	_, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body))
	if err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

func patchInitialize(raw []byte, workspace, home, lang string) ([]byte, bool) {
	var msg map[string]any
	if json.Unmarshal(raw, &msg) != nil {
		return raw, false
	}
	if msg["method"] != "initialize" {
		return raw, false
	}
	params, _ := msg["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
		msg["params"] = params
	}
	uri := "file://" + filepath.ToSlash(workspace)
	params["rootUri"] = uri
	params["rootPath"] = workspace
	params["workspaceFolders"] = []any{
		map[string]any{"uri": uri, "name": filepath.Base(workspace)},
	}
	params["processId"] = nil
	opts, _ := params["initializationOptions"].(map[string]any)
	if opts == nil {
		opts = map[string]any{}
	}
	switch lang {
	case "php":
		cache := filepath.Join(home, ".cache", "intelephense")
		if home == "/" || home == "/root" {
			cache = "/root/.cache/intelephense"
		}
		opts["storagePath"] = cache
		opts["clearCache"] = false
	case "javascript", "typescript":
		opts["hostInfo"] = "siroc"
	case "html":
		opts["provideFormatter"] = true
		opts["embeddedLanguages"] = map[string]any{"css": true, "javascript": true}
	case "css":
		opts["provideFormatter"] = true
	case "json":
		opts["provideFormatter"] = true
	}
	params["initializationOptions"] = opts
	caps, _ := params["capabilities"].(map[string]any)
	if caps == nil {
		caps = map[string]any{}
		params["capabilities"] = caps
	}
	ws, _ := caps["workspace"].(map[string]any)
	if ws == nil {
		ws = map[string]any{}
		caps["workspace"] = ws
	}
	ws["workspaceFolders"] = true
	out, err := json.Marshal(msg)
	if err != nil {
		return raw, true
	}
	return out, true
}
