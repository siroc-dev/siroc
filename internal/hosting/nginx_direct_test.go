//go:build linux

package hosting

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

func renderSite(t *testing.T, data siteData) string {
	t.Helper()
	tpl, err := template.New("cfg").Parse(nginxTmpl)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestDirectNginxTemplate(t *testing.T) {
	body := renderSite(t, siteData{Domain: "site.test", AllNames: "site.test", DocRoot: "/var/www/site", Direct: true, PostMaxSize: "64M"})
	if !strings.Contains(body, "try_files $uri $uri/ =404;") {
		t.Fatalf("missing try_files:\n%s", body)
	}
	if strings.Contains(body, "proxy_pass http://127.0.0.1:8080;") {
		t.Fatal("direct nginx should not proxy to Apache")
	}
	if !strings.Contains(body, "location ~ \\.php$ { return 404; }") {
		t.Fatal("php should be rejected")
	}
}

func TestPHPTemplateStillProxies(t *testing.T) {
	body := renderSite(t, siteData{Domain: "site.test", AllNames: "site.test", DocRoot: "/var/www/site", PostMaxSize: "64M"})
	if !strings.Contains(body, "proxy_pass http://127.0.0.1:8080;") {
		t.Fatal("php site should proxy to Apache")
	}
	if strings.Count(body, "proxy_hide_header Strict-Transport-Security;") != 1 {
		t.Fatalf("http site should hide HSTS once:\n%s", body)
	}
}

func TestSSLHTTP2Directive(t *testing.T) {
	old := renderSite(t, siteData{Domain: "site.test", AllNames: "site.test", DocRoot: "/var/www/site", SSL: true, PostMaxSize: "64M"})
	if !strings.Contains(old, "listen 443 ssl http2;") {
		t.Fatal("older nginx keeps http2 on the listen line")
	}
	body := renderSite(t, siteData{Domain: "site.test", AllNames: "site.test", DocRoot: "/var/www/site", SSL: true, HTTP2On: true, PostMaxSize: "64M"})
	if strings.Contains(body, "listen 443 ssl http2;") || !strings.Contains(body, "http2 on;") {
		t.Fatalf("new nginx should use http2 on:\n%s", body)
	}
	if !strings.Contains(body, "listen 443 ssl;") || !strings.Contains(body, "listen [::]:443 ssl;") {
		t.Fatalf("missing ssl listeners:\n%s", body)
	}
}
