package rpc

type ErrorBody struct {
	Error string `json:"error"`
}

type UserCreateReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	SSH      bool   `json:"ssh"`
	FTP      bool   `json:"ftp"`
}

type UserPasswordReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type UserEnsureReq struct {
	Username string `json:"username"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
	Password string `json:"password,omitempty"`
}

type UserAccessReq struct {
	Username string `json:"username"`
	SSH      bool   `json:"ssh"`
	FTP      bool   `json:"ftp"`
}

type FTPCreateReq struct {
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Home     string `json:"home"`
}

type FTPActionReq struct {
	Login    string `json:"login"`
	Password string `json:"password,omitempty"`
}

type FTPUserResp struct {
	Login string `json:"login"`
	Home  string `json:"home"`
}

type UserResp struct {
	Username  string `json:"username"`
	UID       int    `json:"uid"`
	GID       int    `json:"gid"`
	Home      string `json:"home"`
	Suspended bool   `json:"suspended"`
}

type UserActionReq struct {
	Username string `json:"username"`
}

type UserCLIReq struct {
	Username string `json:"username"`
	PHP      string `json:"php"`
	Python   string `json:"python"`
	Node     string `json:"nodejs"`
}

type FileReq struct {
	Username string `json:"username"`
	Path     string `json:"path"`
	Dest     string `json:"dest,omitempty"`
	Mode     string `json:"mode,omitempty"`
	Content  string `json:"content,omitempty"`
	URL      string `json:"url,omitempty"`
	Query    string `json:"query,omitempty"`
	Root     bool   `json:"root,omitempty"`
}

type FileOpResp struct {
	OK   bool   `json:"ok"`
	Dest string `json:"dest,omitempty"`
	Path string `json:"path,omitempty"`
}

type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"modTime"`
}

type FileListResp struct {
	Path    string      `json:"path"`
	AbsPath string      `json:"absPath,omitempty"`
	Root    bool        `json:"root,omitempty"`
	Entries []FileEntry `json:"entries"`
}

type FileContentResp struct {
	Path    string `json:"path"`
	AbsPath string `json:"absPath,omitempty"`
	Content string `json:"content"`
}

type PkgInstallReq struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type PackageInfo struct {
	Name              string   `json:"name"`
	Title             string   `json:"title"`
	Description       string   `json:"description,omitempty"`
	Installed         bool     `json:"installed"`
	Version           string   `json:"version"`
	Service           string   `json:"service"`
	Active            bool     `json:"active"`
	Versions          []string `json:"versions"`
	InstalledVersions []string `json:"installedVersions,omitempty"`
	CLIVersion        string   `json:"cliVersion,omitempty"`
	ExclusiveOf       string   `json:"exclusiveOf,omitempty"`
}

type CLISetReq struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ServiceReq struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

type SiteWriteReq struct {
	Username   string          `json:"username"`
	Domain     string          `json:"domain"`
	DocRoot    string          `json:"docRoot"`
	PHPVersion string          `json:"phpVersion"`
	Enabled    bool            `json:"enabled"`
	Aliases    []string        `json:"aliases,omitempty"`
	SSL        bool            `json:"ssl,omitempty"`
	SSLKind    string          `json:"sslKind,omitempty"`
	Rewrite    string          `json:"rewrite,omitempty"`
	FPM        *PHPFPMSettings `json:"fpm,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	ProxyPass  string          `json:"proxyPass,omitempty"`
	AppPort    int             `json:"appPort,omitempty"`
	AppCmd     string          `json:"appCmd,omitempty"`
	WAF        bool            `json:"waf"`
}

type SiteRuntimeReq struct {
	Username string `json:"username"`
	Domain   string `json:"domain"`
	DocRoot  string `json:"docRoot"`
	Kind     string `json:"kind"`
	AppPort  int    `json:"appPort,omitempty"`
	AppCmd   string `json:"appCmd,omitempty"`
	Action   string `json:"action"`
}

type SiteRuntimeResp struct {
	OK      bool   `json:"ok"`
	Kind    string `json:"kind"`
	Active  bool   `json:"active"`
	Status  string `json:"status,omitempty"`
	Port    int    `json:"port,omitempty"`
	Cmd     string `json:"cmd,omitempty"`
	Unit    string `json:"unit,omitempty"`
	Log     string `json:"log,omitempty"`
	Message string `json:"message,omitempty"`
}

type SiteSSLReq struct {
	Username   string   `json:"username"`
	Domain     string   `json:"domain"`
	Aliases    []string `json:"aliases,omitempty"`
	Email      string   `json:"email,omitempty"`
	Server     string   `json:"server,omitempty"`
	Directory  string   `json:"directory,omitempty"`
	KeyType    string   `json:"keyType,omitempty"`
	RSAKeySize int      `json:"rsaKeySize,omitempty"`
	EABKID     string   `json:"eabKid,omitempty"`
	EABHMAC    string   `json:"eabHmac,omitempty"`
	NoVerify   bool     `json:"noVerify,omitempty"`
}

type LEConfig struct {
	Email            string `json:"email"`
	Server           string `json:"server"`
	Directory        string `json:"directory,omitempty"`
	KeyType          string `json:"keyType"`
	RSAKeySize       int    `json:"rsaKeySize,omitempty"`
	EABKID           string `json:"eabKid,omitempty"`
	EABHMAC          string `json:"eabHmac,omitempty"`
	HasEABHMAC       bool   `json:"hasEabHmac"`
	NoVerify         bool   `json:"noVerify,omitempty"`
	Registered       bool   `json:"registered"`
	AccountURI       string `json:"accountUri,omitempty"`
	CertbotInstalled bool   `json:"certbotInstalled"`
}

type LEAccountReq struct {
	Email     string `json:"email,omitempty"`
	Server    string `json:"server,omitempty"`
	Directory string `json:"directory,omitempty"`
	EABKID    string `json:"eabKid,omitempty"`
	EABHMAC   string `json:"eabHmac,omitempty"`
	NoVerify  bool   `json:"noVerify,omitempty"`
}

type LEAccountResp struct {
	OK               bool   `json:"ok"`
	Registered       bool   `json:"registered"`
	CertbotInstalled bool   `json:"certbotInstalled"`
	Email            string `json:"email,omitempty"`
	URI              string `json:"uri,omitempty"`
	Message          string `json:"message,omitempty"`
}

type SiteSSLResp struct {
	OK     bool   `json:"ok"`
	Kind   string `json:"kind,omitempty"`
	Expiry string `json:"expiry,omitempty"`
	Cert   string `json:"cert,omitempty"`
}

type SiteAppReq struct {
	Username   string            `json:"username"`
	Domain     string            `json:"domain"`
	DocRoot    string            `json:"docRoot"`
	PHPVersion string            `json:"phpVersion"`
	Action     string            `json:"action"`
	Queue      *bool             `json:"queue,omitempty"`
	QueueName  string            `json:"queueName,omitempty"`
	Workers    int               `json:"workers,omitempty"`
	Queues     []LaravelQueueRow `json:"queues,omitempty"`
	Scheduler  *bool             `json:"scheduler,omitempty"`
	Env        string            `json:"env,omitempty"`
	Composer   string            `json:"composer,omitempty"`
	NPM        string            `json:"npm,omitempty"`
	Artisan    string            `json:"artisan,omitempty"`
	URL        string            `json:"url,omitempty"`
	Name       string            `json:"name,omitempty"`
	Plugin     string            `json:"plugin,omitempty"`
	UserID     int64             `json:"userId,omitempty"`
	AdminUser  string            `json:"adminUser,omitempty"`
	AdminPass  string            `json:"adminPass,omitempty"`
	AdminEmail string            `json:"adminEmail,omitempty"`
	DBName     string            `json:"dbName,omitempty"`
	DBUser     string            `json:"dbUser,omitempty"`
	DBPass     string            `json:"dbPass,omitempty"`
	Prefix     string            `json:"prefix,omitempty"`
	WPCLI      string            `json:"wpcli,omitempty"`
	Login      string            `json:"login,omitempty"`
	XMLRPC     *bool             `json:"xmlrpc,omitempty"`
	Pingbacks  *bool             `json:"pingbacks,omitempty"`
	UploadsPHP *bool             `json:"uploadsPhp,omitempty"`
}

type SiteAppResp struct {
	OK        bool           `json:"ok"`
	Kind      string         `json:"kind"`
	AppRoot   string         `json:"appRoot,omitempty"`
	DocRoot   string         `json:"docRoot,omitempty"`
	Message   string         `json:"message,omitempty"`
	Output    string         `json:"output,omitempty"`
	LoginURLs []string       `json:"loginUrls,omitempty"`
	Laravel   *LaravelStatus `json:"laravel,omitempty"`
	WordPress *WPStatus      `json:"wordpress,omitempty"`
	PkgHits   []PkgHit       `json:"pkgHits,omitempty"`
}

type LaravelStatus struct {
	Queue        bool               `json:"queue"`
	QueueName    string             `json:"queueName,omitempty"`
	Workers      int                `json:"workers"`
	Queues       []LaravelQueueRow  `json:"queues,omitempty"`
	Processes    []LaravelQueueProc `json:"processes,omitempty"`
	Scheduler    bool               `json:"scheduler"`
	Schedule     []LaravelSchedule  `json:"schedule,omitempty"`
	ScheduleLast string             `json:"scheduleLast,omitempty"`
	EnvExists    bool               `json:"envExists"`
	Env          string             `json:"env,omitempty"`
	HasComposer  bool               `json:"hasComposer"`
	HasPackage   bool               `json:"hasPackage"`
	HasArtisan   bool               `json:"hasArtisan"`
	ComposerPkgs []PkgRow           `json:"composerPkgs,omitempty"`
	NPMPkgs      []PkgRow           `json:"npmPkgs,omitempty"`
	NPMScripts   []string           `json:"npmScripts,omitempty"`
	ArtisanCmds  []ArtisanCmd       `json:"artisanCmds,omitempty"`
}

type LaravelQueueRow struct {
	Name      string             `json:"name"`
	Workers   int                `json:"workers"`
	Running   int                `json:"running"`
	Status    string             `json:"status,omitempty"`
	LastRun   string             `json:"lastRun,omitempty"`
	NextRun   string             `json:"nextRun,omitempty"`
	Log       string             `json:"log,omitempty"`
	Processes []LaravelQueueProc `json:"processes,omitempty"`
}

type LaravelQueueProc struct {
	Name    string `json:"name"`
	Queue   string `json:"queue,omitempty"`
	Status  string `json:"status"`
	PID     int    `json:"pid,omitempty"`
	Uptime  string `json:"uptime,omitempty"`
	LastRun string `json:"lastRun,omitempty"`
	NextRun string `json:"nextRun,omitempty"`
}

type LaravelSchedule struct {
	Expression string `json:"expression"`
	Command    string `json:"command"`
	LastRun    string `json:"lastRun,omitempty"`
	NextRun    string `json:"nextRun,omitempty"`
}

type ArtisanCmd struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type PkgRow struct {
	Name       string `json:"name"`
	Section    string `json:"section,omitempty"`
	Constraint string `json:"constraint,omitempty"`
	Installed  string `json:"installed,omitempty"`
	Wanted     string `json:"wanted,omitempty"`
	Latest     string `json:"latest,omitempty"`
	Update     string `json:"update,omitempty"`
}

type PkgHit struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
}

type WPStatus struct {
	Name              string     `json:"name"`
	URL               string     `json:"url"`
	Home              string     `json:"home"`
	Version           string     `json:"version"`
	Prefix            string     `json:"prefix,omitempty"`
	XMLRPCBlocked     bool       `json:"xmlrpcBlocked"`
	PingbacksOff      bool       `json:"pingbacksOff"`
	UploadsPHPBlocked bool       `json:"uploadsPhpBlocked"`
	Users             []WPUser   `json:"users,omitempty"`
	Plugins           []WPPlugin `json:"plugins,omitempty"`
	Updates           *WPUpdates `json:"updates,omitempty"`
	Security          []WPIssue  `json:"security,omitempty"`
	WPCmds            []WPCmd    `json:"wpCmds,omitempty"`
}

type WPCmd struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type WPUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Roles string `json:"roles"`
}

type WPPlugin struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Version string `json:"version"`
	Update  string `json:"update,omitempty"`
	Title   string `json:"title"`
}

type WPUpdates struct {
	Core    string     `json:"core,omitempty"`
	Plugins []WPPlugin `json:"plugins,omitempty"`
	Themes  []string   `json:"themes,omitempty"`
}

type WPIssue struct {
	ID     string `json:"id"`
	Level  string `json:"level"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

type PHPFPMSettings struct {
	PM                string              `json:"pm"`
	MaxChildren       int                 `json:"maxChildren"`
	StartServers      int                 `json:"startServers"`
	MinSpare          int                 `json:"minSpare"`
	MaxSpare          int                 `json:"maxSpare"`
	IdleTimeout       string              `json:"idleTimeout"`
	MaxRequests       int                 `json:"maxRequests"`
	MemoryLimit       string              `json:"memoryLimit"`
	MaxExecutionTime  int                 `json:"maxExecutionTime"`
	MaxInputTime      int                 `json:"maxInputTime"`
	PostMaxSize       string              `json:"postMaxSize"`
	UploadMaxFilesize string              `json:"uploadMaxFilesize"`
	DisplayErrors     bool                `json:"displayErrors"`
	Timezone          string              `json:"timezone"`
	DisableFunctions  string              `json:"disableFunctions"`
	OpenBasedir       bool                `json:"openBasedir"`
	Extensions        map[string][]string `json:"extensions"`
}

type PHPExtInfo struct {
	Name      string `json:"name"`
	Title     string `json:"title"`
	Source    string `json:"source"`
	AptPkg    string `json:"aptPkg,omitempty"`
	Pecl      string `json:"pecl,omitempty"`
	Installed bool   `json:"installed"`
	Available bool   `json:"available"`
	Zend      bool   `json:"zend,omitempty"`
}

type PHPExtListResp struct {
	Version    string       `json:"version"`
	Extensions []PHPExtInfo `json:"extensions"`
}

type PHPExtInstallReq struct {
	Version string `json:"version"`
	Name    string `json:"name"`
	Source  string `json:"source"`
}

type PHPUserApplyReq struct {
	Username string         `json:"username"`
	Settings PHPFPMSettings `json:"settings"`
	Sites    []SiteWriteReq `json:"sites"`
}

type SiteDeleteReq struct {
	Username string `json:"username"`
	Domain   string `json:"domain"`
}

type SiteRenameReq struct {
	SiteWriteReq
	OldDomain string `json:"oldDomain"`
}

type DBCreateReq struct {
	DBName   string `json:"dbName"`
	DBUser   string `json:"dbUser"`
	Password string `json:"password"`
	Engine   string `json:"engine"`
}

type DBDropReq struct {
	DBName string `json:"dbName"`
	DBUser string `json:"dbUser"`
	Engine string `json:"engine"`
}

type DBConfigItem struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Value     string `json:"value"`
	Live      string `json:"live,omitempty"`
	Recommend string `json:"recommend,omitempty"`
}

type DBConfig struct {
	Engine      string         `json:"engine"`
	Installed   bool           `json:"installed"`
	Active      bool           `json:"active"`
	Version     string         `json:"version,omitempty"`
	ConfPath    string         `json:"confPath,omitempty"`
	TotalRAM    int64          `json:"totalRAM"`
	TotalRAMGB  int            `json:"totalRAMGB"`
	SuggestedGB int            `json:"suggestedGB"`
	RAMGB       int            `json:"ramGB"`
	Settings    []DBConfigItem `json:"settings,omitempty"`
	Message     string         `json:"message,omitempty"`
}

type DBConfigApply struct {
	RAMGB    int               `json:"ramGB"`
	Settings map[string]string `json:"settings,omitempty"`
}

type OKResp struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

type CleanupResult struct {
	OK      bool   `json:"ok"`
	Freed   int64  `json:"freed"`
	Files   int    `json:"files"`
	Jobs    int    `json:"jobs,omitempty"`
	Message string `json:"message,omitempty"`
}

type PanelUpdateStatus struct {
	OK         bool   `json:"ok"`
	Version    string `json:"version"`
	Latest     string `json:"latest,omitempty"`
	Available  bool   `json:"available"`
	Channel    string `json:"channel,omitempty"`
	PackageURL string `json:"packageUrl,omitempty"`
	CheckedAt  string `json:"checkedAt,omitempty"`
	Restarting bool   `json:"restarting,omitempty"`
	Message    string `json:"message,omitempty"`
}

type PanelUpdateReq struct {
	Action  string `json:"action"`
	Channel string `json:"channel,omitempty"`
	URL     string `json:"url,omitempty"`
	Path    string `json:"path,omitempty"`
}

type HealthResp struct {
	OK     bool   `json:"ok"`
	Root   bool   `json:"root"`
	Socket string `json:"socket"`
}

type FirewallRule struct {
	ID     int    `json:"id"`
	Action string `json:"action"`
	Port   string `json:"port"`
	Proto  string `json:"proto"`
	Source string `json:"source"`
}

type FirewallStatus struct {
	Installed       bool           `json:"installed"`
	Active          bool           `json:"active"`
	DefaultIncoming string         `json:"defaultIncoming"`
	DefaultPorts    []string       `json:"defaultPorts,omitempty"`
	Rules           []FirewallRule `json:"rules"`
	Message         string         `json:"message,omitempty"`
}

type FirewallRuleReq struct {
	Action string `json:"action"`
	Port   string `json:"port"`
	Proto  string `json:"proto"`
	Source string `json:"source,omitempty"`
}

type FirewallDeleteReq struct {
	ID int `json:"id"`
}

type WAFPack struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Available   bool   `json:"available"`
}

type WAFStatus struct {
	Installed         bool      `json:"installed"`
	Enabled           bool      `json:"enabled"`
	Mode              string    `json:"mode"`
	Version           string    `json:"version,omitempty"`
	CRS               bool      `json:"crs"`
	Paranoia          int       `json:"paranoia"`
	InboundThreshold  int       `json:"inboundThreshold"`
	OutboundThreshold int       `json:"outboundThreshold"`
	Audit             string    `json:"audit"`
	Packs             []WAFPack `json:"packs,omitempty"`
	DisabledIDs       []int     `json:"disabledIds,omitempty"`
	Message           string    `json:"message,omitempty"`
}

type WAFRule struct {
	ID       int    `json:"id"`
	Msg      string `json:"msg"`
	File     string `json:"file"`
	Pack     string `json:"pack"`
	Disabled bool   `json:"disabled"`
}

type WAFRulesResp struct {
	Rules []WAFRule `json:"rules"`
	Total int       `json:"total"`
}

type WAFModeReq struct {
	Mode              string   `json:"mode"`
	Paranoia          int      `json:"paranoia"`
	InboundThreshold  int      `json:"inboundThreshold"`
	OutboundThreshold int      `json:"outboundThreshold"`
	Audit             string   `json:"audit"`
	Packs             []string `json:"packs"`
	DisabledIDs       []int    `json:"disabledIds"`
}

type AVStatus struct {
	Installed       bool   `json:"installed"`
	Version         string `json:"version,omitempty"`
	DaemonActive    bool   `json:"daemonActive"`
	FreshclamActive bool   `json:"freshclamActive"`
	Signatures      string `json:"signatures,omitempty"`
	LastScan        string `json:"lastScan,omitempty"`
}

type AVScanReq struct {
	Path string `json:"path"`
}

type AVScanResp struct {
	OK       bool   `json:"ok"`
	Path     string `json:"path"`
	Infected int    `json:"infected"`
	Summary  string `json:"summary"`
}

type RedisSettings struct {
	Installed       bool   `json:"installed"`
	Version         string `json:"version,omitempty"`
	Bind            string `json:"bind"`
	Port            int    `json:"port"`
	Password        string `json:"password,omitempty"`
	ClearPassword   bool   `json:"clearPassword,omitempty"`
	HasPassword     bool   `json:"hasPassword"`
	ProtectedMode   bool   `json:"protectedMode"`
	MaxMemory       string `json:"maxMemory"`
	MaxMemoryPolicy string `json:"maxMemoryPolicy"`
	AppendOnly      bool   `json:"appendOnly"`
	Timeout         int    `json:"timeout"`
	Databases       int    `json:"databases"`
}

type UserRedisReq struct {
	Username       string `json:"username"`
	Enabled        *bool  `json:"enabled,omitempty"`
	RotatePassword bool   `json:"rotatePassword,omitempty"`
}

type UserRedis struct {
	Username  string `json:"username"`
	Enabled   bool   `json:"enabled"`
	Installed bool   `json:"installed"`
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	RedisUser string `json:"redisUser,omitempty"`
	Password  string `json:"password,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Database  int    `json:"database,omitempty"`
	File      string `json:"file,omitempty"`
	Message   string `json:"message,omitempty"`
}

type ScannerTool struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Installed bool   `json:"installed"`
	Binary    string `json:"binary,omitempty"`
}

type ScannersStatus struct {
	Tools []ScannerTool `json:"tools"`
}

type ScanReq struct {
	Tool   string `json:"tool"`
	Target string `json:"target"`
}

type ScanFinding struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Count    int    `json:"count,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type ScanResp struct {
	OK        bool          `json:"ok"`
	ID        string        `json:"id"`
	Tool      string        `json:"tool"`
	Target    string        `json:"target"`
	Report    string        `json:"report,omitempty"`
	Output    string        `json:"output,omitempty"`
	Title     string        `json:"title,omitempty"`
	Summary   string        `json:"summary,omitempty"`
	Findings  []ScanFinding `json:"findings,omitempty"`
	High      int           `json:"high"`
	Medium    int           `json:"medium"`
	Low       int           `json:"low"`
	Info      int           `json:"info"`
	CreatedAt string        `json:"createdAt,omitempty"`
}

type SystemCPU struct {
	Cores   int     `json:"cores"`
	Model   string  `json:"model,omitempty"`
	Percent float64 `json:"percent"`
}

type SystemMemory struct {
	Total     uint64  `json:"total"`
	Used      uint64  `json:"used"`
	Free      uint64  `json:"free"`
	Available uint64  `json:"available,omitempty"`
	Percent   float64 `json:"percent"`
}

type SystemLoad struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
}

type SystemDisk struct {
	Mount   string  `json:"mount"`
	Device  string  `json:"device"`
	FS      string  `json:"fs"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	Percent float64 `json:"percent"`
}

type SystemNet struct {
	Name    string `json:"name"`
	RxBytes uint64 `json:"rxBytes"`
	TxBytes uint64 `json:"txBytes"`
	RxRate  uint64 `json:"rxRate"`
	TxRate  uint64 `json:"txRate"`
}

type SystemProc struct {
	PID    int     `json:"pid"`
	User   string  `json:"user"`
	Name   string  `json:"name"`
	CPU    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	MemPct float64 `json:"memPct"`
}

type SystemService struct {
	Name   string `json:"name"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
}

type SystemListen struct {
	Proto   string `json:"proto"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	PID     int    `json:"pid"`
	User    string `json:"user"`
	Name    string `json:"name"`
}

type UserUsage struct {
	User         string  `json:"user"`
	Processes    int     `json:"processes"`
	PHPProcesses int     `json:"phpProcesses"`
	Memory       uint64  `json:"memory"`
	MemPct       float64 `json:"memPct"`
	CPU          float64 `json:"cpu"`
	DiskUsed     uint64  `json:"diskUsed"`
}

type LogStatus struct {
	RetentionDays      int    `json:"retentionDays"`
	CloudflarePrefixes int    `json:"cloudflarePrefixes"`
	CloudflareUpdated  string `json:"cloudflareUpdated,omitempty"`
	CloudflareSource   string `json:"cloudflareSource,omitempty"`
	CloudflareOK       bool   `json:"cloudflareOK"`
	RealIPPath         string `json:"realIPPath,omitempty"`
	RealIPModule       bool   `json:"realIPModule"`
	GoAccess           bool   `json:"goaccess"`
	Logrotate          bool   `json:"logrotate"`
	NginxLogs          string `json:"nginxLogs,omitempty"`
}

type LogRetentionReq struct {
	Days int `json:"days"`
}

type GoAccessReq struct {
	Domain  string `json:"domain"`
	Refresh bool   `json:"refresh,omitempty"`
}

type GoAccessInfo struct {
	OK          bool   `json:"ok"`
	Domain      string `json:"domain"`
	Installed   bool   `json:"goaccess"`
	GeneratedAt string `json:"generatedAt,omitempty"`
	Bytes       int64  `json:"bytes,omitempty"`
	Message     string `json:"message,omitempty"`
}

type SiteLogReq struct {
	Username string `json:"username"`
	Domain   string `json:"domain"`
	DocRoot  string `json:"docRoot,omitempty"`
	ID       string `json:"id,omitempty"`
	Bytes    int    `json:"bytes,omitempty"`
}

type SiteLogFile struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Group   string `json:"group"`
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Exists  bool   `json:"exists"`
	Size    int64  `json:"size,omitempty"`
	ModTime string `json:"modTime,omitempty"`
}

type SiteLogEntry struct {
	Time    string `json:"time,omitempty"`
	Env     string `json:"env,omitempty"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Context string `json:"context,omitempty"`
	Status  int    `json:"status,omitempty"`
}

type SiteLogsResp struct {
	OK        bool           `json:"ok"`
	Domain    string         `json:"domain"`
	Files     []SiteLogFile  `json:"files"`
	Current   *SiteLogFile   `json:"current,omitempty"`
	Entries   []SiteLogEntry `json:"entries"`
	Counts    map[string]int `json:"counts,omitempty"`
	Content   string         `json:"content,omitempty"`
	Truncated bool           `json:"truncated"`
}

type SystemStats struct {
	Hostname  string          `json:"hostname"`
	OS        string          `json:"os"`
	Kernel    string          `json:"kernel"`
	Arch      string          `json:"arch"`
	UptimeSec int64           `json:"uptimeSec"`
	Time      string          `json:"time"`
	CPU       SystemCPU       `json:"cpu"`
	Memory    SystemMemory    `json:"memory"`
	Swap      SystemMemory    `json:"swap"`
	Load      SystemLoad      `json:"load"`
	Disks     []SystemDisk    `json:"disks"`
	Network   []SystemNet     `json:"network"`
	Processes int             `json:"processes"`
	Top       []SystemProc    `json:"top"`
	Services  []SystemService `json:"services"`
	Users     []UserUsage     `json:"users"`
	Listen    []SystemListen  `json:"listen"`
}

type ApacheScoreCount struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Count int    `json:"count,omitempty"`
}

type ApacheWorker struct {
	Srv      string  `json:"srv"`
	PID      int     `json:"pid"`
	Acc      string  `json:"acc,omitempty"`
	State    string  `json:"state"`
	Label    string  `json:"label"`
	CPU      float64 `json:"cpu"`
	SS       int     `json:"ss"`
	Req      int     `json:"reqMs"`
	Client   string  `json:"client,omitempty"`
	Protocol string  `json:"protocol,omitempty"`
	VHost    string  `json:"vhost,omitempty"`
	Request  string  `json:"request,omitempty"`
}

type ApacheVHost struct {
	Name string `json:"name"`
	Port int    `json:"port"`
	File string `json:"file,omitempty"`
}

type ApacheStatus struct {
	Installed      bool               `json:"installed"`
	Active         bool               `json:"active"`
	Ready          bool               `json:"ready"`
	Version        string             `json:"version,omitempty"`
	ServerVersion  string             `json:"serverVersion,omitempty"`
	MPM            string             `json:"mpm,omitempty"`
	Built          string             `json:"built,omitempty"`
	CurrentTime    string             `json:"currentTime,omitempty"`
	RestartTime    string             `json:"restartTime,omitempty"`
	ConfigGen      int                `json:"configGen,omitempty"`
	UptimeSec      int64              `json:"uptimeSec"`
	TotalAccesses  int64              `json:"totalAccesses"`
	TotalBytes     int64              `json:"totalBytes"`
	CPULoad        float64            `json:"cpuLoad"`
	ReqPerSec      float64            `json:"reqPerSec"`
	BytesPerSec    float64            `json:"bytesPerSec"`
	BytesPerReq    float64            `json:"bytesPerReq"`
	BusyWorkers    int                `json:"busyWorkers"`
	IdleWorkers    int                `json:"idleWorkers"`
	TotalWorkers   int                `json:"totalWorkers"`
	BusyPercent    float64            `json:"busyPercent"`
	ConnsTotal     int                `json:"connsTotal"`
	ConnsWriting   int                `json:"connsWriting"`
	ConnsKeepAlive int                `json:"connsKeepAlive"`
	ConnsClosing   int                `json:"connsClosing"`
	Load1          float64            `json:"load1"`
	Load5          float64            `json:"load5"`
	Load15         float64            `json:"load15"`
	Scoreboard     string             `json:"scoreboard,omitempty"`
	ScoreCounts    []ApacheScoreCount `json:"scoreCounts,omitempty"`
	ScoreLegend    []ApacheScoreCount `json:"scoreLegend,omitempty"`
	Workers        []ApacheWorker     `json:"workers,omitempty"`
	VHosts         []ApacheVHost      `json:"vhosts,omitempty"`
	FetchedAt      string             `json:"fetchedAt,omitempty"`
	Message        string             `json:"message,omitempty"`
}

type NginxVHost struct {
	Name   string `json:"name"`
	Listen string `json:"listen,omitempty"`
	SSL    bool   `json:"ssl,omitempty"`
	File   string `json:"file,omitempty"`
}

type NginxStatus struct {
	Installed         bool         `json:"installed"`
	Active            bool         `json:"active"`
	Ready             bool         `json:"ready"`
	Version           string       `json:"version,omitempty"`
	UptimeSec         int64        `json:"uptimeSec"`
	ActiveConn        int          `json:"activeConn"`
	Accepts           int64        `json:"accepts"`
	Handled           int64        `json:"handled"`
	Requests          int64        `json:"requests"`
	Reading           int          `json:"reading"`
	Writing           int          `json:"writing"`
	Waiting           int          `json:"waiting"`
	ReqPerSec         float64      `json:"reqPerSec"`
	WorkerProcesses   string       `json:"workerProcesses,omitempty"`
	WorkerConnections int          `json:"workerConnections,omitempty"`
	Sites             []NginxVHost `json:"sites,omitempty"`
	FetchedAt         string       `json:"fetchedAt,omitempty"`
	Message           string       `json:"message,omitempty"`
}

type WebOptimizeItem struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Group     string `json:"group"`
	Value     string `json:"value"`
	Live      string `json:"live,omitempty"`
	Recommend string `json:"recommend,omitempty"`
}

type WebOptimize struct {
	NginxInstalled  bool              `json:"nginxInstalled"`
	ApacheInstalled bool              `json:"apacheInstalled"`
	NginxActive     bool              `json:"nginxActive"`
	ApacheActive    bool              `json:"apacheActive"`
	TotalRAM        int64             `json:"totalRAM"`
	TotalRAMGB      int               `json:"totalRAMGB"`
	SuggestedGB     int               `json:"suggestedGB"`
	RAMGB           int               `json:"ramGB"`
	CPUs            int               `json:"cpus"`
	Settings        []WebOptimizeItem `json:"settings,omitempty"`
	Message         string            `json:"message,omitempty"`
}

type WebOptimizeApply struct {
	RAMGB    int               `json:"ramGB"`
	Settings map[string]string `json:"settings,omitempty"`
}

type FPMProcess struct {
	PID      int    `json:"pid"`
	State    string `json:"state,omitempty"`
	Method   string `json:"method,omitempty"`
	URI      string `json:"uri,omitempty"`
	User     string `json:"user,omitempty"`
	Script   string `json:"script,omitempty"`
	Duration int64  `json:"duration,omitempty"`
}

type FPMPool struct {
	Name               string       `json:"name"`
	Version            string       `json:"version,omitempty"`
	Listen             string       `json:"listen,omitempty"`
	PM                 string       `json:"pm,omitempty"`
	StartSince         int64        `json:"startSince"`
	Accepted           int64        `json:"accepted"`
	ListenQueue        int          `json:"listenQueue"`
	MaxListenQueue     int          `json:"maxListenQueue"`
	Idle               int          `json:"idle"`
	Active             int          `json:"active"`
	Total              int          `json:"total"`
	MaxActive          int          `json:"maxActive"`
	MaxChildrenReached int          `json:"maxChildrenReached"`
	SlowRequests       int64        `json:"slowRequests"`
	Ready              bool         `json:"ready"`
	Message            string       `json:"message,omitempty"`
	Processes          []FPMProcess `json:"processes,omitempty"`
}

type PHPFPMStatus struct {
	Installed   bool      `json:"installed"`
	Active      bool      `json:"active"`
	Ready       bool      `json:"ready"`
	Versions    []string  `json:"versions,omitempty"`
	Pools       []FPMPool `json:"pools,omitempty"`
	Idle        int       `json:"idle"`
	ActiveProcs int       `json:"activeProcs"`
	Total       int       `json:"total"`
	Accepted    int64     `json:"accepted"`
	FetchedAt   string    `json:"fetchedAt,omitempty"`
	Message     string    `json:"message,omitempty"`
}

type DBProcess struct {
	ID      int64  `json:"id"`
	User    string `json:"user,omitempty"`
	Host    string `json:"host,omitempty"`
	DB      string `json:"db,omitempty"`
	Command string `json:"command,omitempty"`
	Time    int    `json:"time"`
	State   string `json:"state,omitempty"`
	Info    string `json:"info,omitempty"`
}

type DBStatus struct {
	Engine             string      `json:"engine"`
	Installed          bool        `json:"installed"`
	Active             bool        `json:"active"`
	Ready              bool        `json:"ready"`
	Version            string      `json:"version,omitempty"`
	Comment            string      `json:"comment,omitempty"`
	UptimeSec          int64       `json:"uptimeSec"`
	ThreadsConnected   int         `json:"threadsConnected"`
	ThreadsRunning     int         `json:"threadsRunning"`
	MaxUsedConnections int         `json:"maxUsedConnections"`
	MaxConnections     int         `json:"maxConnections"`
	Questions          int64       `json:"questions"`
	SlowQueries        int64       `json:"slowQueries"`
	Connections        int64       `json:"connections"`
	AbortedConnects    int64       `json:"abortedConnects"`
	BytesReceived      int64       `json:"bytesReceived"`
	BytesSent          int64       `json:"bytesSent"`
	QPS                float64     `json:"qps"`
	BytesPerSec        float64     `json:"bytesPerSec"`
	OpenTables         int64       `json:"openTables"`
	DataDir            string      `json:"dataDir,omitempty"`
	Processes          []DBProcess `json:"processes,omitempty"`
	FetchedAt          string      `json:"fetchedAt,omitempty"`
	Message            string      `json:"message,omitempty"`
}

type FileSearchHit struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type FileSearchResp struct {
	Hits      []FileSearchHit `json:"hits"`
	Truncated bool            `json:"truncated,omitempty"`
}

type QuotaReq struct {
	Username string `json:"username"`
	LimitMB  int64  `json:"limitMB"`
}

type QuotaInfo struct {
	Username   string `json:"username"`
	UsedBytes  int64  `json:"usedBytes"`
	LimitBytes int64  `json:"limitBytes"`
	LimitMB    int64  `json:"limitMB"`
	Enabled    bool   `json:"enabled"`
	Message    string `json:"message,omitempty"`
}

type BackupReq struct {
	Username  string   `json:"username"`
	Kind      string   `json:"kind"`
	LocalDir  string   `json:"localDir,omitempty"`
	Host      string   `json:"host,omitempty"`
	Port      int      `json:"port,omitempty"`
	User      string   `json:"user,omitempty"`
	Password  string   `json:"password,omitempty"`
	Path      string   `json:"path,omitempty"`
	Endpoint  string   `json:"endpoint,omitempty"`
	Region    string   `json:"region,omitempty"`
	Bucket    string   `json:"bucket,omitempty"`
	Prefix    string   `json:"prefix,omitempty"`
	AccessKey string   `json:"accessKey,omitempty"`
	SecretKey string   `json:"secretKey,omitempty"`
	UseSSL    bool     `json:"useSSL,omitempty"`
	IncludeDB bool     `json:"includeDB,omitempty"`
	Databases []string `json:"databases,omitempty"`
	Restore   string   `json:"restore,omitempty"`
}

type BackupResp struct {
	OK      bool   `json:"ok"`
	Path    string `json:"path,omitempty"`
	Remote  string `json:"remote,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Message string `json:"message,omitempty"`
}

type SysopsReq struct {
	Action       string   `json:"action"`
	Nameservers  []string `json:"nameservers,omitempty"`
	SearchDomain string   `json:"searchDomain,omitempty"`
	Timezone     string   `json:"timezone,omitempty"`
	NTP          *bool    `json:"ntp,omitempty"`
	SwapMB       int      `json:"swapMB,omitempty"`
	Interface    string   `json:"interface,omitempty"`
	Address      string   `json:"address,omitempty"`
	Gateway      string   `json:"gateway,omitempty"`
	Device       string   `json:"device,omitempty"`
	MountPoint   string   `json:"mountPoint,omitempty"`
	FSType       string   `json:"fsType,omitempty"`
	Options      string   `json:"options,omitempty"`
	Persist      bool     `json:"persist,omitempty"`
	Jail         string   `json:"jail,omitempty"`
	IP           string   `json:"ip,omitempty"`
	Src          string   `json:"src,omitempty"`
	Dest         string   `json:"dest,omitempty"`
	Extra        string   `json:"extra,omitempty"`
	MemoryMB     int      `json:"memoryMB,omitempty"`
	Listen       string   `json:"listen,omitempty"`
	Port         int      `json:"port,omitempty"`
}

type SysopsStatus struct {
	Hostname  string           `json:"hostname"`
	Time      string           `json:"time"`
	Timezone  string           `json:"timezone"`
	NTP       bool             `json:"ntp"`
	Timezones []string         `json:"timezones,omitempty"`
	DNS       []string         `json:"dns"`
	Search    string           `json:"search,omitempty"`
	Swap      *SwapInfo        `json:"swap,omitempty"`
	IPs       []NetAddr        `json:"ips,omitempty"`
	Routes    []string         `json:"routes,omitempty"`
	Ifaces    []string         `json:"ifaces,omitempty"`
	Mounts    []MountInfo      `json:"mounts,omitempty"`
	Fstab     []FstabEntry     `json:"fstab,omitempty"`
	Fail2ban  *Fail2banStatus  `json:"fail2ban,omitempty"`
	Threats   *ThreatStatus    `json:"threats,omitempty"`
	FFmpeg    *FFmpegStatus    `json:"ffmpeg,omitempty"`
	Memcached *MemcachedStatus `json:"memcached,omitempty"`
	Quota     bool             `json:"quota"`
	Message   string           `json:"message,omitempty"`
}

type SwapInfo struct {
	TotalMB int    `json:"totalMB"`
	UsedMB  int    `json:"usedMB"`
	File    string `json:"file,omitempty"`
}

type NetAddr struct {
	Iface   string `json:"iface"`
	Address string `json:"address"`
	Family  string `json:"family"`
}

type MountInfo struct {
	Device     string `json:"device"`
	MountPoint string `json:"mountPoint"`
	FSType     string `json:"fsType"`
	Options    string `json:"options"`
	Size       string `json:"size,omitempty"`
	Used       string `json:"used,omitempty"`
	Avail      string `json:"avail,omitempty"`
	UsePct     string `json:"usePct,omitempty"`
}

type FstabEntry struct {
	Device     string `json:"device"`
	MountPoint string `json:"mountPoint"`
	FSType     string `json:"fsType"`
	Options    string `json:"options"`
	Dump       string `json:"dump,omitempty"`
	Pass       string `json:"pass,omitempty"`
	Raw        string `json:"raw,omitempty"`
}

type DiskAnalysis struct {
	Filesystems []MountInfo `json:"filesystems"`
	Trees       []DiskNode  `json:"trees,omitempty"`
	Largest     []DiskNode  `json:"largest,omitempty"`
	Message     string      `json:"message,omitempty"`
}

type DiskNode struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Label string `json:"label,omitempty"`
}

type Fail2banStatus struct {
	Installed bool           `json:"installed"`
	Active    bool           `json:"active"`
	Jails     []Fail2banJail `json:"jails,omitempty"`
	Message   string         `json:"message,omitempty"`
}

type Fail2banJail struct {
	Name   string   `json:"name"`
	Banned []string `json:"banned,omitempty"`
	Failed int      `json:"failed,omitempty"`
	Total  int      `json:"total,omitempty"`
}

type ThreatStatus struct {
	FailedLogins []ThreatEvent `json:"failedLogins,omitempty"`
	Banned       []ThreatEvent `json:"banned,omitempty"`
	TopSources   []ThreatEvent `json:"topSources,omitempty"`
	Message      string        `json:"message,omitempty"`
}

type ThreatEvent struct {
	IP     string `json:"ip"`
	Count  int    `json:"count,omitempty"`
	Detail string `json:"detail,omitempty"`
	When   string `json:"when,omitempty"`
	Source string `json:"source,omitempty"`
}

type FFmpegStatus struct {
	Installed bool     `json:"installed"`
	Version   string   `json:"version,omitempty"`
	Codecs    []string `json:"codecs,omitempty"`
	Message   string   `json:"message,omitempty"`
	Output    string   `json:"output,omitempty"`
}

type MemcachedStatus struct {
	Installed bool              `json:"installed"`
	Active    bool              `json:"active"`
	MemoryMB  int               `json:"memoryMB"`
	Listen    string            `json:"listen"`
	Port      int               `json:"port"`
	Stats     map[string]string `json:"stats,omitempty"`
	Message   string            `json:"message,omitempty"`
}

type PMASignonReq struct {
	DBUser   string `json:"dbUser"`
	Password string `json:"password"`
	Host     string `json:"host,omitempty"`
}

type PMASignonResp struct {
	OK    bool   `json:"ok"`
	Token string `json:"token,omitempty"`
	URL   string `json:"url,omitempty"`
}

type DBPasswordReq struct {
	DBUser   string `json:"dbUser"`
	Password string `json:"password"`
}
