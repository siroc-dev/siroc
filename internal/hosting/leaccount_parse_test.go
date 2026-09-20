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

func TestCertbotAccountExists(t *testing.T) {
	if !certbotAccountExists("There is an existing account; skipping registration.") {
		t.Fatal("existing account")
	}
	if certbotAccountExists("Account registered.") {
		t.Fatal("new account is not existing")
	}
}
