package weblog

import (
	"errors"
	"testing"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestLeftoverFor(t *testing.T) {
	p, alt := leftoverFor("nginx", "access", "driedstrow.shop")
	if p != "/var/log/nginx/sites/driedstrow.shop-access.log" {
		t.Fatalf("primary %q", p)
	}
	if alt != "/var/log/nginx/driedstrow.shop-access.log" {
		t.Fatalf("leftover %q", alt)
	}
	if got := domainFromLogPath(p); got != "driedstrow.shop" {
		t.Fatalf("domain %q", got)
	}
}

func TestLogHintEmpty(t *testing.T) {
	hint := logHint(rpc.SiteLogReq{Domain: "driedstrow.shop"}, nil, nil, false, nil)
	if hint == "" {
		t.Fatal("expected empty-log hint")
	}
	if got := logHint(rpc.SiteLogReq{Probe: true}, nil, nil, true, nil); got != "Recorded a local test request to this domain." {
		t.Fatalf("probe hint %q", got)
	}
	if got := logHint(rpc.SiteLogReq{Probe: true}, nil, nil, false, errors.New("down")); got == "" {
		t.Fatal("expected probe error hint")
	}
}
