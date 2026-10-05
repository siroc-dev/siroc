package security

import "testing"

func TestFirewallCommentIDs(t *testing.T) {
	status := `
Status: active

     To                         Action      From
     --                         ------      ----
[ 1] Anywhere                   ALLOW IN    203.0.113.10               # siroc-whitelist
[ 2] Anywhere                   DENY IN     198.51.100.8               # siroc-blacklist
[ 3] 22/tcp                     ALLOW IN    Anywhere                   # siroc-default
[ 4] Anywhere (v6)              DENY IN     2001:db8::/32              # siroc-blacklist
`
	ids := FirewallCommentIDs(status)
	if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 4 {
		t.Fatalf("%v", ids)
	}
}
