package hosting

import "testing"

func TestParseCertbotAccount(t *testing.T) {
	out := `Saving debug log to /var/log/letsencrypt/letsencrypt.log
Account details for server https://acme-v02.api.letsencrypt.org/directory:
  Account URL: https://acme-v02.api.letsencrypt.org/acme/acct/12345
  Account Thumbprint: abcdef
  Email contact: admin@example.com
`
	uri, email, ok := parseCertbotAccount(out)
	if !ok {
		t.Fatal("expected parsed account")
	}
	if uri != "https://acme-v02.api.letsencrypt.org/acme/acct/12345" {
		t.Fatalf("uri=%q", uri)
	}
	if email != "admin@example.com" {
		t.Fatalf("email=%q", email)
	}
}

func TestCertbotBusy(t *testing.T) {
	msg := "Another instance of Certbot is already running.\nAsk for help or search for solutions at https://community.letsencrypt.org. See the logfile /tmp/certbot-log-115tvzkp/log or re-run Certbot with -v for more details."
	if !certbotBusy(msg) {
		t.Fatal("expected the lock message")
	}
	if certbotBusy("Successfully received certificate.") {
		t.Fatal("a finished issue is not busy")
	}
}

func TestDescribeCertbotBusy(t *testing.T) {
	cmd := []string{"/usr/bin/certbot certonly --webroot --cert-name movie.rdp-service.com -d movie.rdp-service.com -d www.movie.rdp-service.com"}
	got := describeCertbotBusy(cmd, "")
	if got != "Certbot is working on movie.rdp-service.com, www.movie.rdp-service.com." {
		t.Fatal(got)
	}
	renew := describeCertbotBusy([]string{"/usr/bin/certbot -q renew"}, "Processing /etc/letsencrypt/renewal/other.example.conf\nRenewing an existing certificate for other.example and www.other.example\n")
	if renew != "Certbot is working on other.example, www.other.example." {
		t.Fatal(renew)
	}
	onlyRenew := describeCertbotBusy([]string{"certbot renew --quiet"}, "Certificate not yet due for renewal\n")
	if onlyRenew != "Certbot is renewing certificates." {
		t.Fatal(onlyRenew)
	}
}

func TestCertbotAccountExists(t *testing.T) {
	if !certbotAccountExists("There is an existing account; skipping registration.") {
		t.Fatal("existing account")
	}
	if certbotAccountExists("Account registered.") {
		t.Fatal("new account is not existing")
	}
}
