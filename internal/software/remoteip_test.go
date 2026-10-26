package software

import (
	"strings"
	"testing"
)

func TestApacheRemoteIPUsesVisitorAddress(t *testing.T) {
	body := apacheRemoteIPConf()
	for _, want := range []string{
		"RemoteIPHeader X-Real-IP",
		"RemoteIPInternalProxy 127.0.0.1",
		"RemoteIPInternalProxy ::1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s\n%s", want, body)
		}
	}
}
