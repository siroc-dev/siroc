package validate

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var gitBranchRe = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,128}$`)

const LaravelDeployCommand = `composer install --no-dev --no-interaction --optimize-autoloader
php artisan migrate --force
php artisan optimize`

const NPMDeployCommand = `npm install
npm run build`

func GitRepo(raw string) (string, error) {
	n := strings.TrimSpace(raw)
	if n == "" {
		return "", fmt.Errorf("git repo is required")
	}
	if strings.ContainsAny(n, " \t\n\r;$|&<>`\\\"'") {
		return "", fmt.Errorf("invalid git repo")
	}
	if strings.HasPrefix(n, "git@") {
		host, path, ok := strings.Cut(strings.TrimPrefix(n, "git@"), ":")
		if !ok || host == "" || path == "" || strings.Contains(host, "/") {
			return "", fmt.Errorf("use an SSH repo such as git@github.com:org/repo.git")
		}
		return n, nil
	}
	if strings.HasPrefix(n, "ssh://") {
		u, err := url.Parse(n)
		if err != nil || u.Host == "" || u.Path == "" || u.Path == "/" {
			return "", fmt.Errorf("invalid git repo")
		}
		return n, nil
	}
	return "", fmt.Errorf("use an SSH repo such as git@github.com:org/repo.git")
}

func GitBranch(raw string) (string, error) {
	n := strings.TrimSpace(raw)
	if n == "" {
		n = "main"
	}
	if strings.Contains(n, "..") || !gitBranchRe.MatchString(n) {
		return "", fmt.Errorf("invalid git branch")
	}
	return n, nil
}

func GitCommand(raw string) (string, error) {
	if len(raw) > 8192 {
		return "", fmt.Errorf("deploy command is too long")
	}
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("invalid deploy command")
	}
	return raw, nil
}
