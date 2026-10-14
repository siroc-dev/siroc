package validate

import "strings"

// FormatNginx pretty-prints an nginx config: one directive per line, braces on
// the directive line, and four spaces per block. Text inside quotes stays as written.
func FormatNginx(src string) string {
	src = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(src, "\r\n", "\n"), "\r", "\n"))
	if src == "" {
		return ""
	}
	var b strings.Builder
	indent := 0
	lineHas := false
	writeIndent := func() {
		for n := 0; n < indent; n++ {
			b.WriteString("    ")
		}
	}
	endLine := func() {
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		lineHas = false
	}
	for _, t := range scanNginx(src) {
		switch t.kind {
		case '{':
			if lineHas {
				b.WriteByte(' ')
			} else {
				writeIndent()
			}
			b.WriteByte('{')
			endLine()
			indent++
		case '}':
			if lineHas {
				endLine()
			}
			if indent > 0 {
				indent--
			}
			writeIndent()
			b.WriteByte('}')
			endLine()
		case ';':
			b.WriteByte(';')
			endLine()
		case '#':
			if lineHas {
				endLine()
			}
			writeIndent()
			b.WriteString(t.text)
			endLine()
		default:
			if !lineHas {
				writeIndent()
			} else {
				b.WriteByte(' ')
			}
			b.WriteString(t.text)
			lineHas = true
		}
	}
	if lineHas {
		endLine()
	}
	return b.String()
}

type nginxTok struct {
	kind byte
	text string
}

func scanNginx(src string) []nginxTok {
	var out []nginxTok
	for i := 0; i < len(src); {
		c := src[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if c == '#' {
			j := i + 1
			for j < len(src) && src[j] != '\n' {
				j++
			}
			out = append(out, nginxTok{'#', strings.TrimRight(src[i:j], " \t\r")})
			i = j
			continue
		}
		if c == '"' || c == '\'' {
			q := c
			j := i + 1
			for j < len(src) {
				if src[j] == '\\' && j+1 < len(src) {
					j += 2
					continue
				}
				if src[j] == q {
					j++
					break
				}
				if src[j] == '\n' {
					break
				}
				j++
			}
			out = append(out, nginxTok{'q', src[i:j]})
			i = j
			continue
		}
		if c == '{' || c == '}' || c == ';' {
			out = append(out, nginxTok{c, string(c)})
			i++
			continue
		}
		j := i + 1
		for j < len(src) {
			d := src[j]
			if d == ' ' || d == '\t' || d == '\n' || d == '\r' || d == '{' || d == '}' || d == ';' || d == '#' || d == '"' || d == '\'' {
				break
			}
			j++
		}
		out = append(out, nginxTok{'w', src[i:j]})
		i = j
	}
	return out
}
