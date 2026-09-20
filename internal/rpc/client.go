package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	http *http.Client
	base string
}

func NewClient(socket string) *Client {
	t := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
	}
	return &Client{
		http: &http.Client{Transport: t, Timeout: 15 * time.Minute},
		base: "http://agent",
	}
}

func (c *Client) do(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("agent unreachable: %w", err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		var eb ErrorBody
		if json.Unmarshal(b, &eb) == nil && eb.Error != "" {
			return fmt.Errorf("%s", eb.Error)
		}
		return fmt.Errorf("agent error (%d): %s", res.StatusCode, string(b))
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

func (c *Client) Health() (*HealthResp, error) {
	var h HealthResp
	err := c.do(http.MethodGet, "/health", nil, &h)
	return &h, err
}

func (c *Client) UserCreate(username, password string, ssh, ftp bool) (*UserResp, error) {
	var out UserResp
	err := c.do(http.MethodPost, "/users", UserCreateReq{Username: username, Password: password, SSH: ssh, FTP: ftp}, &out)
	return &out, err
}

func (c *Client) UserPassword(username, password string) error {
	return c.do(http.MethodPost, "/users/password", UserPasswordReq{Username: username, Password: password}, nil)
}

func (c *Client) UserEnsure(in UserEnsureReq) (*UserResp, error) {
	var out UserResp
	err := c.do(http.MethodPost, "/users/ensure", in, &out)
	return &out, err
}

func (c *Client) UserAccess(username string, ssh, ftp bool) error {
	return c.do(http.MethodPost, "/users/access", UserAccessReq{Username: username, SSH: ssh, FTP: ftp}, nil)
}

func (c *Client) FTPCreate(in FTPCreateReq) (*FTPUserResp, error) {
	var out FTPUserResp
	err := c.do(http.MethodPost, "/ftp", in, &out)
	return &out, err
}

func (c *Client) FTPPassword(login, password string) error {
	return c.do(http.MethodPost, "/ftp/password", FTPActionReq{Login: login, Password: password}, nil)
}

func (c *Client) FTPDelete(login string) error {
	return c.do(http.MethodPost, "/ftp/delete", FTPActionReq{Login: login}, nil)
}

func (c *Client) UserList() ([]UserResp, error) {
	var out []UserResp
	err := c.do(http.MethodGet, "/users", nil, &out)
	return out, err
}

func (c *Client) UserSuspend(username string, suspend bool) error {
	path := "/users/suspend"
	if !suspend {
		path = "/users/unsuspend"
	}
	return c.do(http.MethodPost, path, UserActionReq{Username: username}, nil)
}

func (c *Client) UserDelete(username string) error {
	return c.do(http.MethodPost, "/users/delete", UserActionReq{Username: username}, nil)
}

func (c *Client) UserCLI(in UserCLIReq) error {
	return c.do(http.MethodPost, "/users/cli", in, nil)
}

func (c *Client) FileList(username, path string, root bool) (*FileListResp, error) {
	var out FileListResp
	err := c.do(http.MethodPost, "/files/list", FileReq{Username: username, Path: path, Root: root}, &out)
	return &out, err
}

func (c *Client) FileRead(username, path string, root bool) (*FileContentResp, error) {
	var out FileContentResp
	err := c.do(http.MethodPost, "/files/read", FileReq{Username: username, Path: path, Root: root}, &out)
	return &out, err
}

func (c *Client) FileWrite(username, path, content string, root bool) error {
	return c.do(http.MethodPost, "/files/write", FileReq{Username: username, Path: path, Content: content, Root: root}, nil)
}

func (c *Client) FileMkdir(username, path string, root bool) error {
	return c.do(http.MethodPost, "/files/mkdir", FileReq{Username: username, Path: path, Root: root}, nil)
}

func (c *Client) FileDelete(username, path string, root bool) error {
	return c.do(http.MethodPost, "/files/delete", FileReq{Username: username, Path: path, Root: root}, nil)
}

func (c *Client) FileRename(username, path, dest string, root bool) error {
	return c.do(http.MethodPost, "/files/rename", FileReq{Username: username, Path: path, Dest: dest, Root: root}, nil)
}

func (c *Client) FileChmod(username, path, mode string, root bool) error {
	return c.do(http.MethodPost, "/files/chmod", FileReq{Username: username, Path: path, Mode: mode, Root: root}, nil)
}

func (c *Client) FileCopy(username, path, dest string, root bool) error {
	return c.do(http.MethodPost, "/files/copy", FileReq{Username: username, Path: path, Dest: dest, Root: root}, nil)
}

func (c *Client) FileExtract(username, path string, root bool) (*FileOpResp, error) {
	var out FileOpResp
	err := c.do(http.MethodPost, "/files/extract", FileReq{Username: username, Path: path, Root: root}, &out)
	return &out, err
}

func (c *Client) FileDownload(username, path string, root bool) (*http.Response, error) {
	q := url.Values{}
	q.Set("username", username)
	q.Set("path", path)
	if root {
		q.Set("root", "1")
	}
	req, err := http.NewRequest(http.MethodGet, c.base+"/files/download?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent unreachable: %w", err)
	}
	if res.StatusCode >= 400 {
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		var eb ErrorBody
		if json.Unmarshal(b, &eb) == nil && eb.Error != "" {
			return nil, fmt.Errorf("%s", eb.Error)
		}
		return nil, fmt.Errorf("agent error (%d): %s", res.StatusCode, string(b))
	}
	return res, nil
}

func (c *Client) FileLSPStatus(lang string) (bool, error) {
	var out struct {
		OK bool `json:"ok"`
	}
	err := c.do(http.MethodGet, "/files/lsp-status?lang="+url.QueryEscape(lang), nil, &out)
	return out.OK, err
}

func (c *Client) FileUpload(username, path, filename string, r io.Reader, root bool) error {
	pr, pw := io.Pipe()
	w := multipart.NewWriter(pw)
	errCh := make(chan error, 1)
	go func() {
		var err error
		defer func() {
			_ = w.Close()
			_ = pw.CloseWithError(err)
			errCh <- err
		}()
		if err = w.WriteField("username", username); err != nil {
			return
		}
		if err = w.WriteField("path", path); err != nil {
			return
		}
		if root {
			if err = w.WriteField("root", "1"); err != nil {
				return
			}
		}
		part, e := w.CreateFormFile("file", filename)
		if e != nil {
			err = e
			return
		}
		_, err = io.Copy(part, r)
	}()
	req, err := http.NewRequest(http.MethodPost, c.base+"/files/upload", pr)
	if err != nil {
		_ = pw.Close()
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := c.http.Do(req)
	copyErr := <-errCh
	if err != nil {
		return fmt.Errorf("agent unreachable: %w", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if copyErr != nil {
		return copyErr
	}
	if res.StatusCode >= 400 {
		var eb ErrorBody
		if json.Unmarshal(b, &eb) == nil && eb.Error != "" {
			return fmt.Errorf("%s", eb.Error)
		}
		return fmt.Errorf("agent error (%d): %s", res.StatusCode, string(b))
	}
	return nil
}

func (c *Client) Packages() ([]PackageInfo, error) {
	var out []PackageInfo
	err := c.do(http.MethodGet, "/software", nil, &out)
	return out, err
}

func (c *Client) Install(name, version string) (*OKResp, error) {
	var out OKResp
	err := c.do(http.MethodPost, "/software/install", PkgInstallReq{Name: name, Version: version}, &out)
	return &out, err
}

func (c *Client) CleanTemp() (*CleanupResult, error) {
	var out CleanupResult
	err := c.do(http.MethodPost, "/cleanup/temp", nil, &out)
	return &out, err
}

func (c *Client) CleanLogs() (*CleanupResult, error) {
	var out CleanupResult
	err := c.do(http.MethodPost, "/cleanup/logs", nil, &out)
	return &out, err
}

func (c *Client) Service(name, action string) error {
	return c.do(http.MethodPost, "/software/service", ServiceReq{Name: name, Action: action}, nil)
}

func (c *Client) SiteWrite(in SiteWriteReq) error {
	return c.do(http.MethodPost, "/sites/write", in, nil)
}

func (c *Client) SiteSSL(in SiteSSLReq) (*SiteSSLResp, error) {
	var out SiteSSLResp
	err := c.do(http.MethodPost, "/sites/ssl", in, &out)
	return &out, err
}

func (c *Client) RegisterLEAccount(in LEAccountReq) (*LEAccountResp, error) {
	var out LEAccountResp
	err := c.do(http.MethodPost, "/ssl/letsencrypt/account", in, &out)
	return &out, err
}

func (c *Client) LEAccountStatus(in LEAccountReq) (*LEAccountResp, error) {
	var out LEAccountResp
	err := c.do(http.MethodPost, "/ssl/letsencrypt/account/status", in, &out)
	return &out, err
}

func (c *Client) SiteApp(in SiteAppReq) (*SiteAppResp, error) {
	var out SiteAppResp
	err := c.do(http.MethodPost, "/sites/app", in, &out)
	return &out, err
}

func (c *Client) SiteDelete(username, domain string) error {
	return c.do(http.MethodPost, "/sites/delete", SiteDeleteReq{Username: username, Domain: domain}, nil)
}

func (c *Client) SiteRename(in SiteRenameReq) error {
	return c.do(http.MethodPost, "/sites/rename", in, nil)
}

func (c *Client) SiteRuntime(in SiteRuntimeReq) (*SiteRuntimeResp, error) {
	var out SiteRuntimeResp
	err := c.do(http.MethodPost, "/sites/runtime", in, &out)
	return &out, err
}

func (c *Client) PanelUpdateStatus() (*PanelUpdateStatus, error) {
	var out PanelUpdateStatus
	err := c.do(http.MethodGet, "/update/status", nil, &out)
	return &out, err
}

func (c *Client) PanelUpdate(in PanelUpdateReq) (*PanelUpdateStatus, error) {
	var out PanelUpdateStatus
	err := c.do(http.MethodPost, "/update", in, &out)
	return &out, err
}

func (c *Client) SetCLI(name, version string) error {
	return c.do(http.MethodPost, "/software/cli", CLISetReq{Name: name, Version: version}, nil)
}

func (c *Client) PHPVersions() ([]string, error) {
	var out []string
	err := c.do(http.MethodGet, "/software/php-versions", nil, &out)
	return out, err
}

func (c *Client) PHPExtensions(version string) (*PHPExtListResp, error) {
	var out PHPExtListResp
	err := c.do(http.MethodGet, "/php/extensions?version="+url.QueryEscape(version), nil, &out)
	return &out, err
}

func (c *Client) PHPUserApply(in PHPUserApplyReq) error {
	return c.do(http.MethodPost, "/php/apply", in, nil)
}

func (c *Client) DBCreate(in DBCreateReq) error {
	return c.do(http.MethodPost, "/db/create", in, nil)
}

func (c *Client) DBDrop(in DBDropReq) error {
	return c.do(http.MethodPost, "/db/drop", in, nil)
}

func (c *Client) DBEngine() (string, error) {
	var out OKResp
	err := c.do(http.MethodGet, "/db/engine", nil, &out)
	return out.Message, err
}

func (c *Client) DBConfig(ramGB int) (*DBConfig, error) {
	var out DBConfig
	path := "/db/config"
	if ramGB > 0 {
		path = fmt.Sprintf("/db/config?ramGB=%d", ramGB)
	}
	err := c.do(http.MethodGet, path, nil, &out)
	return &out, err
}

func (c *Client) SetDBConfig(in DBConfigApply) (*DBConfig, error) {
	var out DBConfig
	err := c.do(http.MethodPost, "/db/config", in, &out)
	return &out, err
}

func (c *Client) FirewallStatus() (*FirewallStatus, error) {
	var out FirewallStatus
	err := c.do(http.MethodGet, "/security/firewall", nil, &out)
	return &out, err
}

func (c *Client) FirewallEnable(enable bool) error {
	return c.do(http.MethodPost, "/security/firewall/enable", map[string]bool{"enable": enable}, nil)
}

func (c *Client) FirewallAdd(in FirewallRuleReq) error {
	return c.do(http.MethodPost, "/security/firewall/rules", in, nil)
}

func (c *Client) FirewallDelete(id int) error {
	return c.do(http.MethodPost, "/security/firewall/delete", FirewallDeleteReq{ID: id}, nil)
}

func (c *Client) WAFStatus() (*WAFStatus, error) {
	var out WAFStatus
	err := c.do(http.MethodGet, "/security/waf", nil, &out)
	return &out, err
}

func (c *Client) WAFSetMode(mode string) error {
	return c.WAFApply(WAFModeReq{Mode: mode})
}

func (c *Client) WAFApply(in WAFModeReq) error {
	return c.do(http.MethodPost, "/security/waf", in, nil)
}

func (c *Client) WAFRules(q, pack string) (*WAFRulesResp, error) {
	var out WAFRulesResp
	path := "/security/waf/rules?q=" + url.QueryEscape(q) + "&pack=" + url.QueryEscape(pack)
	err := c.do(http.MethodGet, path, nil, &out)
	return &out, err
}

func (c *Client) AVStatus() (*AVStatus, error) {
	var out AVStatus
	err := c.do(http.MethodGet, "/security/av", nil, &out)
	return &out, err
}

func (c *Client) AVUpdate() error {
	return c.do(http.MethodPost, "/security/av/update", nil, nil)
}

func (c *Client) AVScan(path string) (*AVScanResp, error) {
	var out AVScanResp
	err := c.do(http.MethodPost, "/security/av/scan", AVScanReq{Path: path}, &out)
	return &out, err
}

func (c *Client) Redis() (*RedisSettings, error) {
	var out RedisSettings
	err := c.do(http.MethodGet, "/software/redis", nil, &out)
	return &out, err
}

func (c *Client) SetRedis(in RedisSettings) error {
	return c.do(http.MethodPost, "/software/redis", in, nil)
}

func (c *Client) UserRedis(username string) (*UserRedis, error) {
	var out UserRedis
	err := c.do(http.MethodPost, "/users/redis", UserRedisReq{Username: username}, &out)
	return &out, err
}

func (c *Client) ListUserRedis() ([]UserRedis, error) {
	var out []UserRedis
	err := c.do(http.MethodGet, "/users/redis", nil, &out)
	if out == nil {
		out = []UserRedis{}
	}
	return out, err
}

func (c *Client) SetUserRedis(in UserRedisReq) (*UserRedis, error) {
	var out UserRedis
	err := c.do(http.MethodPost, "/users/redis/set", in, &out)
	return &out, err
}

func (c *Client) DropUserRedis(username string) error {
	off := false
	_, err := c.SetUserRedis(UserRedisReq{Username: username, Enabled: &off})
	return err
}

func (c *Client) Scanners() (*ScannersStatus, error) {
	var out ScannersStatus
	err := c.do(http.MethodGet, "/security/scanners", nil, &out)
	return &out, err
}

func (c *Client) RunScan(in ScanReq) (*ScanResp, error) {
	var out ScanResp
	err := c.do(http.MethodPost, "/security/scan", in, &out)
	return &out, err
}

func (c *Client) ScanLogs() ([]ScanResp, error) {
	var out []ScanResp
	err := c.do(http.MethodGet, "/security/scan/logs", nil, &out)
	if out == nil {
		out = []ScanResp{}
	}
	return out, err
}

func (c *Client) ScanLog(id string) (*ScanResp, error) {
	var out ScanResp
	err := c.do(http.MethodGet, "/security/scan/log?id="+url.QueryEscape(id), nil, &out)
	return &out, err
}

func (c *Client) ScanReport(id string) (string, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.base+"/security/scan/report?id="+url.QueryEscape(id), nil)
	if err != nil {
		return "", nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("agent unreachable: %w", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		var eb ErrorBody
		if json.Unmarshal(b, &eb) == nil && eb.Error != "" {
			return "", nil, fmt.Errorf("%s", eb.Error)
		}
		return "", nil, fmt.Errorf("agent error (%d): %s", res.StatusCode, string(b))
	}
	ctype := res.Header.Get("Content-Type")
	if ctype == "" {
		ctype = "text/plain; charset=utf-8"
	}
	return ctype, b, nil
}

func (c *Client) SystemStats() (*SystemStats, error) {
	var out SystemStats
	err := c.do(http.MethodGet, "/system/stats", nil, &out)
	return &out, err
}

func (c *Client) ApacheStatus() (*ApacheStatus, error) {
	var out ApacheStatus
	err := c.do(http.MethodGet, "/apache/status", nil, &out)
	return &out, err
}

func (c *Client) NginxStatus() (*NginxStatus, error) {
	var out NginxStatus
	err := c.do(http.MethodGet, "/nginx/status", nil, &out)
	return &out, err
}

func (c *Client) WebOptimize(ramGB int) (*WebOptimize, error) {
	var out WebOptimize
	path := "/webserver/optimize"
	if ramGB > 0 {
		path = fmt.Sprintf("/webserver/optimize?ramGB=%d", ramGB)
	}
	err := c.do(http.MethodGet, path, nil, &out)
	return &out, err
}

func (c *Client) SetWebOptimize(in WebOptimizeApply) (*WebOptimize, error) {
	var out WebOptimize
	err := c.do(http.MethodPost, "/webserver/optimize", in, &out)
	return &out, err
}

func (c *Client) PHPFPMStatus() (*PHPFPMStatus, error) {
	var out PHPFPMStatus
	err := c.do(http.MethodGet, "/php-fpm/status", nil, &out)
	return &out, err
}

func (c *Client) DBStatus(engine string) (*DBStatus, error) {
	var out DBStatus
	err := c.do(http.MethodGet, "/"+engine+"/status", nil, &out)
	return &out, err
}

func (c *Client) LogStatus() (*LogStatus, error) {
	var out LogStatus
	err := c.do(http.MethodGet, "/logs/status", nil, &out)
	return &out, err
}

func (c *Client) LogRetention(days int) (*LogStatus, error) {
	var out LogStatus
	err := c.do(http.MethodPost, "/logs/retention", LogRetentionReq{Days: days}, &out)
	return &out, err
}

func (c *Client) CloudflareRefresh() (*LogStatus, error) {
	var out LogStatus
	err := c.do(http.MethodPost, "/logs/cloudflare", nil, &out)
	return &out, err
}

func (c *Client) GoAccessInfo(domain string, refresh bool) (*GoAccessInfo, error) {
	var out GoAccessInfo
	err := c.do(http.MethodPost, "/logs/goaccess", GoAccessReq{Domain: domain, Refresh: refresh}, &out)
	return &out, err
}

func (c *Client) SiteLogs(in SiteLogReq) (*SiteLogsResp, error) {
	var out SiteLogsResp
	err := c.do(http.MethodPost, "/logs/site", in, &out)
	return &out, err
}

func (c *Client) GoAccessHTML(domain string, refresh bool) ([]byte, error) {
	q := "/logs/goaccess.html?domain=" + url.QueryEscape(domain)
	if refresh {
		q += "&refresh=1"
	}
	return c.doBytes(http.MethodGet, q)
}

func (c *Client) FileFetch(in FileReq) (*FileOpResp, error) {
	var out FileOpResp
	err := c.do(http.MethodPost, "/files/fetch", in, &out)
	return &out, err
}

func (c *Client) FileSearch(in FileReq) (*FileSearchResp, error) {
	var out FileSearchResp
	err := c.do(http.MethodPost, "/files/search", in, &out)
	return &out, err
}

func (c *Client) Sysops(in SysopsReq) (*SysopsStatus, error) {
	var out SysopsStatus
	err := c.do(http.MethodPost, "/sysops", in, &out)
	return &out, err
}

func (c *Client) Disk() (*DiskAnalysis, error) {
	var out DiskAnalysis
	err := c.do(http.MethodGet, "/sysops/disk", nil, &out)
	return &out, err
}

func (c *Client) QuotaGet(username string) (*QuotaInfo, error) {
	var out QuotaInfo
	err := c.do(http.MethodPost, "/users/quota", QuotaReq{Username: username}, &out)
	return &out, err
}

func (c *Client) QuotaSet(username string, limitMB int64) (*QuotaInfo, error) {
	var out QuotaInfo
	err := c.do(http.MethodPost, "/users/quota/set", QuotaReq{Username: username, LimitMB: limitMB}, &out)
	return &out, err
}

func (c *Client) BackupRun(in BackupReq) (*BackupResp, error) {
	var out BackupResp
	err := c.do(http.MethodPost, "/backup/run", in, &out)
	return &out, err
}

func (c *Client) PMASignon(in PMASignonReq) (*PMASignonResp, error) {
	var out PMASignonResp
	err := c.do(http.MethodPost, "/pma/signon", in, &out)
	return &out, err
}

func (c *Client) DBPassword(user, password string) error {
	return c.do(http.MethodPost, "/db/password", DBPasswordReq{DBUser: user, Password: password}, nil)
}

func (c *Client) doBytes(method, path string) ([]byte, error) {
	req, err := http.NewRequest(method, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent unreachable: %w", err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		var eb ErrorBody
		if json.Unmarshal(b, &eb) == nil && eb.Error != "" {
			return nil, fmt.Errorf("%s", eb.Error)
		}
		return nil, fmt.Errorf("agent error (%d): %s", res.StatusCode, string(b))
	}
	return b, nil
}
