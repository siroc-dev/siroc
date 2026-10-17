package validate

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var gitBranchRe = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,128}$`)

const LaravelDeployCommand = `composer install --no-dev --no-interaction --optimize-autoloader
npm install
npm run build
php artisan migrate --force
php artisan optimize
php artisan queue:restart`

const NPMDeployCommand = `npm install
npm run build`

func GitRepo(raw string) (string, error) {
	repo, _, err := ParseGitRemote(raw)
	return repo, err
}

// ParseGitRemote accepts an SSH address or an HTTP URL. A URL that names a
// branch, such as https://github.com/org/repo/tree/develop, returns that branch.
func ParseGitRemote(raw string) (repo, branch string, err error) {
	n := strings.TrimSpace(raw)
	if n == "" {
		return "", "", fmt.Errorf("git repo is required")
	}
	if strings.ContainsAny(n, " \t\n\r;$|&<>`\\\"'") {
		return "", "", fmt.Errorf("invalid git repo")
	}
	if strings.HasPrefix(n, "git@") || strings.HasPrefix(n, "ssh://") {
		repo, branch = splitGitFragment(n)
		if err := checkSSHRepo(repo); err != nil {
			return "", "", err
		}
		if branch != "" {
			branch, err = GitBranch(branch)
			if err != nil {
				return "", "", err
			}
		}
		return repo, branch, nil
	}
	if !strings.HasPrefix(n, "https://") && !strings.HasPrefix(n, "http://") {
		return "", "", fmt.Errorf("use a git URL such as https://github.com/org/repo.git or git@github.com:org/repo.git")
	}
	n, branch = splitGitFragment(n)
	u, err := url.Parse(n)
	if err != nil || u.Host == "" || u.Path == "" || u.Path == "/" {
		return "", "", fmt.Errorf("invalid git repo")
	}
	path := strings.Trim(u.Path, "/")
	if path == "" || strings.Contains(path, "..") {
		return "", "", fmt.Errorf("invalid git repo")
	}
	if i := strings.Index(path, "/-/tree/"); i >= 0 {
		branch = strings.Trim(path[i+len("/-/tree/"):], "/")
		path = path[:i]
	} else if i := strings.Index(path, "/tree/"); i >= 0 {
		branch = strings.Trim(path[i+len("/tree/"):], "/")
		path = path[:i]
	} else if left, rest, ok := strings.Cut(path, "/src/"); ok && strings.Contains(left, "/") && rest != "" {
		branch = strings.Trim(rest, "/")
		path = left
	}
	if path == "" || strings.Contains(path, "..") {
		return "", "", fmt.Errorf("invalid git repo")
	}
	if !strings.HasSuffix(path, ".git") {
		path += ".git"
	}
	u.Path = "/" + path
	u.RawQuery = ""
	u.Fragment = ""
	if branch != "" {
		branch, err = GitBranch(branch)
		if err != nil {
			return "", "", err
		}
	}
	return u.String(), branch, nil
}

func splitGitFragment(raw string) (repo, branch string) {
	if i := strings.LastIndex(raw, "#"); i > 0 && i < len(raw)-1 {
		return raw[:i], raw[i+1:]
	}
	return raw, ""
}

func checkSSHRepo(n string) error {
	if strings.HasPrefix(n, "git@") {
		host, path, ok := strings.Cut(strings.TrimPrefix(n, "git@"), ":")
		if !ok || host == "" || path == "" || strings.Contains(host, "/") {
			return fmt.Errorf("use an SSH repo such as git@github.com:org/repo.git")
		}
		return nil
	}
	if strings.HasPrefix(n, "ssh://") {
		u, err := url.Parse(n)
		if err != nil || u.Host == "" || u.Path == "" || u.Path == "/" {
			return fmt.Errorf("invalid git repo")
		}
		return nil
	}
	return fmt.Errorf("invalid git repo")
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

// GitNeedsDeployKey reports a failed pull that looks like a private repository
// the server key cannot read.
func GitNeedsDeployKey(log string) bool {
	s := strings.ToLower(log)
	if s == "" {
		return false
	}
	hints := []string{
		"permission denied (publickey)",
		"could not read from remote repository",
		"correct access rights",
		"authentication failed",
		"terminal prompts disabled",
		"repository not found",
		"could not be found or you don't have permission",
		"http basic: access denied",
		"returned error: 401",
		"returned error: 403",
	}
	for _, hint := range hints {
		if strings.Contains(s, hint) {
			return true
		}
	}
	return false
}
