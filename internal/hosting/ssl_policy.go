package hosting

func certPreferOrder(prefer string) []string {
	switch prefer {
	case "custom":
		return []string{"custom", "local", "letsencrypt"}
	case "letsencrypt":
		return []string{"letsencrypt", "local", "custom"}
	default:
		return []string{"local", "custom", "letsencrypt"}
	}
}

// sslRedirect is on only when the site saved Redirect HTTP to HTTPS.
// A local, Let's Encrypt, or pasted certificate does not turn it on.
func sslRedirect(ssl bool, _ string, force *bool) bool {
	return ssl && force != nil && *force
}
