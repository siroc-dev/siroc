package siteopts

import (
	"fmt"
	"strings"
)

// DefaultStaticExt is the Plesk "serve static files directly by nginx" list,
// plus webp and the woff fonts those sites usually add.
const DefaultStaticExt = "ac3 avi bmp bz2 css cue dat doc docx dts eot exe flv gif gz htm html ico img iso jpeg jpg js mkv mp3 mp4 mpeg mpg ogg pdf png ppt pptx qt rar rm svg swf tar tgz ttf txt wav webp woff woff2 xls xlsx zip"

// StaticOn reports whether Nginx should serve static files from the document root.
// A nil Static keeps the default, which is on.
func StaticOn(o Options) bool {
	return o.Static == nil || *o.Static
}

func staticExplicitOff(o Options) bool {
	return o.Static != nil && !*o.Static
}

// NginxStatic is the extension location plus the Apache fallback.
// Empty when static serving is off. Only PHP sites insert it.
func NginxStatic(o Options) (string, error) {
	norm, err := Normalize(o)
	if err != nil {
		return "", err
	}
	if !StaticOn(norm) {
		return "", nil
	}
	ext := strings.Fields(norm.StaticExt)
	if len(ext) == 0 {
		ext = strings.Fields(DefaultStaticExt)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "    location ~* \\.(%s)$ {\n", strings.Join(ext, "|"))
	b.WriteString("        try_files $uri @siroc_apache;\n")
	b.WriteString("    }\n")
	b.WriteString("    location @siroc_apache {\n")
	b.WriteString("        proxy_pass http://127.0.0.1:8080;\n")
	b.WriteString("        proxy_http_version 1.1;\n")
	b.WriteString("        proxy_set_header Host $http_host;\n")
	b.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
	b.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	b.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	b.WriteString("    }\n")
	return b.String(), nil
}

func parseStaticExt(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ',' || r == '|' || r == ';'
	})
	seen := map[string]struct{}{}
	var out []string
	for _, part := range parts {
		ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(part), "."))
		if ext == "" {
			continue
		}
		if !staticExtOK(ext) {
			return nil, fmt.Errorf("static extension %q is invalid", part)
		}
		if staticExtDenied(ext) {
			return nil, fmt.Errorf("static extension %q must stay on Apache", part)
		}
		if _, ok := seen[ext]; ok {
			continue
		}
		seen[ext] = struct{}{}
		out = append(out, ext)
		if len(out) > 80 {
			return nil, fmt.Errorf("at most 80 static extensions")
		}
	}
	return out, nil
}

func staticExtOK(ext string) bool {
	if len(ext) < 1 || len(ext) > 10 {
		return false
	}
	for _, r := range ext {
		if r < 'a' || r > 'z' {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func staticExtDenied(ext string) bool {
	switch ext {
	case "php", "phtml", "phar", "php3", "php4", "php5", "php7", "php8", "phps", "cgi", "fcgi", "pl", "pm", "py", "rb", "sh", "bash", "inc", "asp", "aspx", "jsp":
		return true
	default:
		return false
	}
}
