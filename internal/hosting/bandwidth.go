package hosting

import (
	"bytes"
	"io"
	"strconv"
	"strings"
)

func bandwidthLogPath(domain string) (string, bool) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || domain == "default" || domain == "siroc-default" || strings.Contains(domain, "..") || strings.ContainsAny(domain, "/\\ \t") {
		return "", false
	}
	for _, r := range domain {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' && r != '-' {
			return "", false
		}
	}
	return "/var/log/nginx/sites/" + domain + "-bw.log", true
}

// AddBandwidthLog records bytes sent for every request, including images and other files
// that are left out of the access log. Each server block gets one line.
func AddBandwidthLog(conf, domain string) string {
	path, ok := bandwidthLogPath(domain)
	if !ok {
		return conf
	}
	line := "    access_log " + path + " siroc_bw;"
	var b strings.Builder
	rest := conf
	for {
		rel := indexServer(rest)
		if rel < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:rel])
		block, n, ok := serverBlock(rest[rel:])
		if !ok {
			b.WriteString(rest[rel:])
			break
		}
		if !strings.Contains(block, path) {
			block = insertAfterServerBrace(block, line)
		}
		b.WriteString(block)
		rest = rest[rel+n:]
	}
	return b.String()
}

func indexServer(s string) int {
	for i := 0; i < len(s); {
		j := strings.Index(s[i:], "server")
		if j < 0 {
			return -1
		}
		j += i
		if j > 0 {
			prev := s[j-1]
			if prev != '\n' && prev != ' ' && prev != '\t' && prev != '{' && prev != ';' {
				i = j + len("server")
				continue
			}
		}
		k := j + len("server")
		if k >= len(s) {
			return -1
		}
		if s[k] == '_' {
			i = k
			continue
		}
		if s[k] == ' ' || s[k] == '\t' || s[k] == '{' || s[k] == '\n' || s[k] == '\r' {
			return j
		}
		i = k
	}
	return -1
}

func serverBlock(s string) (string, int, bool) {
	brace := strings.IndexByte(s, '{')
	if brace < 0 {
		return "", 0, false
	}
	depth := 0
	for i := brace; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1], i + 1, true
			}
		}
	}
	return "", 0, false
}

func insertAfterServerBrace(block, line string) string {
	brace := strings.IndexByte(block, '{')
	if brace < 0 {
		return block
	}
	return block[:brace+1] + "\n" + line + block[brace+1:]
}

// SumLines adds complete decimal lines from r. A trailing partial line is left uncounted
// so the next read can finish it. consumed is how many bytes of r were finished lines.
func SumLines(r io.Reader) (sum, consumed int64, err error) {
	buf := make([]byte, 64*1024)
	var carry []byte
	for {
		n, readErr := r.Read(buf)
		data := append(carry, buf[:n]...)
		part, used := sumCompleteLines(data)
		sum += part
		if used > len(carry) {
			consumed += int64(used - len(carry))
		}
		if used > 0 {
			carry = append([]byte(nil), data[used:]...)
		} else if len(data) > 1<<20 {
			consumed += int64(len(data) - len(carry))
			carry = nil
		} else {
			carry = data
		}
		if readErr == io.EOF {
			return sum, consumed, nil
		}
		if readErr != nil {
			return sum, consumed, readErr
		}
	}
}

func sumCompleteLines(data []byte) (int64, int) {
	end := bytes.LastIndexByte(data, '\n')
	if end < 0 {
		return 0, 0
	}
	var sum int64
	for _, line := range bytes.Split(data[:end], []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		n, err := strconv.ParseInt(string(line), 10, 64)
		if err != nil || n < 0 {
			continue
		}
		sum += n
	}
	return sum, end + 1
}
