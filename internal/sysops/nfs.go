package sysops

import (
	"fmt"
	"net"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

type nfsExp struct {
	id                     int
	path, client           string
	flags                  []string
	anonUID, anonGID, fsid string
	extra                  []string
}

func ExportOptions() []rpc.NFSOption {
	return []rpc.NFSOption{
		{Key: "rw", Group: "access", Exclusive: true, Label: "Read and write", Detail: "Clients can change files on this share."},
		{Key: "ro", Group: "access", Exclusive: true, Label: "Read only", Detail: "Clients can read files but cannot change them."},
		{Key: "sync", Group: "commit", Exclusive: true, Label: "Sync", Detail: "Confirm a write only after it is stored. Safer if the server loses power."},
		{Key: "async", Group: "commit", Exclusive: true, Label: "Async", Detail: "Confirm a write before it is stored. Faster, and a crash can lose the newest writes."},
		{Key: "root_squash", Group: "identity", Exclusive: true, Label: "Squash root", Detail: "A client root user is treated as nobody on this server."},
		{Key: "no_root_squash", Group: "identity", Exclusive: true, Label: "Keep root", Detail: "A client root user stays root on this server. Use this only for a trusted admin client."},
		{Key: "all_squash", Group: "identity", Exclusive: true, Label: "Squash everyone", Detail: "Every client user is treated as nobody."},
		{Key: "no_subtree_check", Group: "subtree", Exclusive: true, Label: "No subtree check", Detail: "Do not check that a file is still inside this export. This fits a normal directory share."},
		{Key: "subtree_check", Group: "subtree", Exclusive: true, Label: "Subtree check", Detail: "Reject a file that moved outside this export. This costs extra lookups."},
		{Key: "secure", Group: "ports", Exclusive: true, Label: "Privileged ports", Detail: "Accept only clients that connect from a port below 1024."},
		{Key: "insecure", Group: "ports", Exclusive: true, Label: "Any port", Detail: "Also accept clients that connect from a port above 1024. Some macOS and userspace clients need this."},
		{Key: "nohide", Group: "extra", Label: "Show nested mounts", Detail: "Show a filesystem mounted inside this export."},
		{Key: "crossmnt", Group: "extra", Label: "Cross child mounts", Detail: "Let a client enter a child mount that lives on this server."},
		{Key: "no_wdelay", Group: "extra", Label: "No write delay", Detail: "Send writes immediately instead of combining them. Use this when clients open files synchronously."},
	}
}

func MountOptions() []rpc.NFSOption {
	return []rpc.NFSOption{
		{Key: "hard", Group: "retry", Exclusive: true, Label: "Hard", Detail: "Keep retrying if the server stops responding. Programs wait instead of failing."},
		{Key: "soft", Group: "retry", Exclusive: true, Label: "Soft", Detail: "Give up after the retry count. Programs can see an I/O error while the server is down."},
		{Key: "bg", Group: "startup", Exclusive: true, Label: "Background", Detail: "If the first attempt fails, keep trying in the background."},
		{Key: "fg", Group: "startup", Exclusive: true, Label: "Foreground", Detail: "Wait until the mount succeeds or fails before continuing."},
		{Key: "_netdev", Group: "boot", Label: "Wait for network", Detail: "At boot, wait until the network is up before mounting."},
		{Key: "noatime", Group: "safety", Label: "No access time", Detail: "Do not update a file's access time on read."},
		{Key: "nodiratime", Group: "safety", Label: "No directory access time", Detail: "Do not update a directory's access time."},
		{Key: "nosuid", Group: "safety", Label: "Ignore setuid", Detail: "Ignore setuid and setgid bits on this mount."},
		{Key: "nodev", Group: "safety", Label: "Ignore devices", Detail: "Ignore device files on this mount."},
		{Key: "noexec", Group: "safety", Label: "No execute", Detail: "Do not run programs from this mount."},
	}
}

func NFSVersions() []rpc.NFSChoice {
	return []rpc.NFSChoice{
		{Value: "", Label: "Client default", Detail: "Let the client pick the newest version the server offers."},
		{Value: "4.2", Label: "NFSv4.2", Detail: "Newest version. Server-side copy and sparse files work when the server allows them."},
		{Value: "4.1", Label: "NFSv4.1", Detail: "Sessions and a single firewall port. A common choice when 4.2 is not available."},
		{Value: "4", Label: "NFSv4.0", Detail: "One TCP port and a single namespace. Easier to firewall than version 3."},
		{Value: "3", Label: "NFSv3", Detail: "Older clients. Locking uses separate ports, so the firewall needs more than port 2049."},
	}
}

func NFSProtos() []rpc.NFSChoice {
	return []rpc.NFSChoice{
		{Value: "", Label: "Client default", Detail: "The client usually chooses TCP."},
		{Value: "tcp", Label: "TCP", Detail: "Reliable. Use this unless the network is a sealed local link."},
		{Value: "udp", Label: "UDP", Detail: "Less overhead. Packets can be lost on a busy network."},
	}
}

func BuildNFSStatus(exports, fstab, proc string) *rpc.NFSStatus {
	exps := parseExports(exports)
	return &rpc.NFSStatus{
		Exports:       exportViews(exps),
		Mounts:        mergeNFSMounts(fstab, proc),
		ExportOptions: ExportOptions(),
		MountOptions:  MountOptions(),
		Versions:      NFSVersions(),
		Protos:        NFSProtos(),
	}
}

func MutateExports(text, op string, id int, req rpc.NFSExportReq) (string, error) {
	comments, entries := splitExports(text)
	switch op {
	case "add":
		row, err := cleanExport(req, nil)
		if err != nil {
			return "", err
		}
		if exportTaken(entries, 0, row.path, row.client) {
			return "", fmt.Errorf("that client is already on this share")
		}
		if len(entries) >= 500 {
			return "", fmt.Errorf("too many NFS shares")
		}
		entries = append(entries, row)
	case "update":
		prev := findExport(entries, id)
		if prev == nil {
			return "", fmt.Errorf("NFS share not found")
		}
		row, err := cleanExport(req, prev)
		if err != nil {
			return "", err
		}
		if exportTaken(entries, id, row.path, row.client) {
			return "", fmt.Errorf("that client is already on this share")
		}
		*prev = row
		prev.id = id
	case "delete":
		if findExport(entries, id) == nil {
			return "", fmt.Errorf("NFS share not found")
		}
		next := entries[:0]
		for _, e := range entries {
			if e.id != id {
				next = append(next, e)
			}
		}
		entries = next
	default:
		return "", fmt.Errorf("unknown NFS action")
	}
	return renderExports(comments, entries), nil
}

func MutateFstabNFS(text, op string, mountPoint, source, opts string) (string, error) {
	if op == "delete" {
		mountPoint = strings.TrimSpace(mountPoint)
		if mountPoint == "" || mountPoint == "/" {
			return "", fmt.Errorf("invalid mount point")
		}
	} else {
		var err error
		mountPoint, err = cleanLocalPath(mountPoint)
		if err != nil {
			return "", err
		}
	}
	lines := splitLines(text)
	found := false
	var out []string
	for _, line := range lines {
		_, mp, fs, _, ok := fstabFields(line)
		if ok && (fs == "nfs" || fs == "nfs4") && mp == mountPoint {
			found = true
			if op == "delete" {
				continue
			}
			out = append(out, fmt.Sprintf("%s %s nfs %s 0 0", source, mountPoint, opts))
			continue
		}
		out = append(out, line)
	}
	if op == "delete" {
		return joinLines(out), nil
	}
	if !found {
		if op == "update" && source == "" {
			return "", fmt.Errorf("NFS mount not found")
		}
		out = append(out, fmt.Sprintf("%s %s nfs %s 0 0", source, mountPoint, opts))
	}
	return joinLines(out), nil
}

func MountOptionString(req rpc.NFSMountReq) (string, error) {
	source, mp, err := cleanMountReq(req)
	if err != nil {
		return "", err
	}
	_ = source
	_ = mp
	flags, err := cleanFlags(req.Options, MountOptions())
	if err != nil {
		return "", err
	}
	vers, err := choice(req.NFSVers, NFSVersions(), "NFS version")
	if err != nil {
		return "", err
	}
	proto, err := choice(req.Proto, NFSProtos(), "protocol")
	if err != nil {
		return "", err
	}
	var parts []string
	parts = append(parts, flags...)
	if vers != "" {
		parts = append(parts, "nfsvers="+vers)
	}
	if proto != "" {
		parts = append(parts, "proto="+proto)
	}
	if req.Timeo != 0 {
		if req.Timeo < 1 || req.Timeo > 6000 {
			return "", fmt.Errorf("timeout must be from 1 to 6000 tenths of a second")
		}
		parts = append(parts, "timeo="+strconv.Itoa(req.Timeo))
	}
	if req.Retrans != 0 {
		if req.Retrans < 1 || req.Retrans > 100 {
			return "", fmt.Errorf("retries must be from 1 to 100")
		}
		parts = append(parts, "retrans="+strconv.Itoa(req.Retrans))
	}
	for _, pair := range []struct {
		n    int
		name string
	}{{req.RSize, "rsize"}, {req.WSize, "wsize"}} {
		if pair.n == 0 {
			continue
		}
		if pair.n < 1024 || pair.n > 1048576 {
			return "", fmt.Errorf("%s must be from 1024 to 1048576", pair.name)
		}
		parts = append(parts, pair.name+"="+strconv.Itoa(pair.n))
	}
	if len(parts) == 0 {
		parts = []string{"defaults"}
	}
	return strings.Join(parts, ","), nil
}

func mergeNFSMounts(fstab, proc string) []rpc.NFSMount {
	live := map[string]string{}
	for _, line := range splitLines(proc) {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		if f[2] != "nfs" && f[2] != "nfs4" {
			continue
		}
		live[f[1]] = f[3]
	}
	var out []rpc.NFSMount
	seen := map[string]bool{}
	for _, line := range splitLines(fstab) {
		dev, mp, fs, opt, ok := fstabFields(line)
		if !ok || (fs != "nfs" && fs != "nfs4") {
			continue
		}
		row := parseMountOptions(dev, mp, opt)
		row.Persisted = true
		if raw, ok := live[mp]; ok {
			row.Mounted = true
			if row.NFSVers == "" || len(row.Options) == 0 {
				liveRow := parseMountOptions(dev, mp, raw)
				if row.NFSVers == "" {
					row.NFSVers = liveRow.NFSVers
				}
				if row.Proto == "" {
					row.Proto = liveRow.Proto
				}
			}
		}
		out = append(out, row)
		seen[mp] = true
	}
	var extra []string
	for mp := range live {
		if !seen[mp] {
			extra = append(extra, mp)
		}
	}
	sort.Strings(extra)
	for _, mp := range extra {
		src := ""
		for _, line := range splitLines(proc) {
			f := strings.Fields(line)
			if len(f) >= 2 && f[1] == mp {
				src = f[0]
				break
			}
		}
		row := parseMountOptions(src, mp, live[mp])
		row.Mounted = true
		out = append(out, row)
	}
	for i := range out {
		out[i].ID = i + 1
	}
	if out == nil {
		out = []rpc.NFSMount{}
	}
	return out
}

func parseExports(text string) []nfsExp {
	_, entries := splitExports(text)
	return entries
}

func splitExports(text string) (comments []string, entries []nfsExp) {
	next := 1
	for _, line := range joinContinued(text) {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "#") {
			comments = append(comments, trim)
			continue
		}
		path, clients, ok := splitExportLine(trim)
		if !ok {
			comments = append(comments, "# "+trim)
			continue
		}
		for _, client := range clients {
			row := parseClientSpec(path, client)
			row.id = next
			next++
			entries = append(entries, row)
		}
	}
	return comments, entries
}

func splitExportLine(line string) (string, []string, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "/") {
		return "", nil, false
	}
	return fields[0], fields[1:], true
}

func parseClientSpec(dir, spec string) nfsExp {
	row := nfsExp{path: dir}
	name := spec
	opt := ""
	if i := strings.Index(spec, "("); i >= 0 && strings.HasSuffix(spec, ")") {
		name = spec[:i]
		opt = spec[i+1 : len(spec)-1]
	}
	row.client = name
	known := map[string]rpc.NFSOption{}
	for _, o := range ExportOptions() {
		known[o.Key] = o
	}
	for _, part := range strings.Split(opt, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "anonuid=") {
			row.anonUID = strings.TrimPrefix(part, "anonuid=")
			continue
		}
		if strings.HasPrefix(part, "anongid=") {
			row.anonGID = strings.TrimPrefix(part, "anongid=")
			continue
		}
		if strings.HasPrefix(part, "fsid=") {
			row.fsid = strings.TrimPrefix(part, "fsid=")
			continue
		}
		if _, ok := known[part]; ok {
			row.flags = append(row.flags, part)
			continue
		}
		row.extra = append(row.extra, part)
	}
	return row
}

func renderExports(comments []string, entries []nfsExp) string {
	var b strings.Builder
	for _, c := range comments {
		b.WriteString(c)
		b.WriteByte('\n')
	}
	order := []string{}
	grouped := map[string][]nfsExp{}
	for _, e := range entries {
		if _, ok := grouped[e.path]; !ok {
			order = append(order, e.path)
		}
		grouped[e.path] = append(grouped[e.path], e)
	}
	for _, p := range order {
		b.WriteString(p)
		for _, e := range grouped[p] {
			b.WriteByte(' ')
			b.WriteString(formatClient(e))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func formatClient(e nfsExp) string {
	var parts []string
	parts = append(parts, e.flags...)
	if e.anonUID != "" {
		parts = append(parts, "anonuid="+e.anonUID)
	}
	if e.anonGID != "" {
		parts = append(parts, "anongid="+e.anonGID)
	}
	if e.fsid != "" {
		parts = append(parts, "fsid="+e.fsid)
	}
	parts = append(parts, e.extra...)
	if len(parts) == 0 {
		return e.client
	}
	return e.client + "(" + strings.Join(parts, ",") + ")"
}

func exportViews(entries []nfsExp) []rpc.NFSExport {
	out := make([]rpc.NFSExport, 0, len(entries))
	for _, e := range entries {
		out = append(out, rpc.NFSExport{
			ID: e.id, Path: e.path, Client: e.client, Options: append([]string{}, e.flags...),
			AnonUID: e.anonUID, AnonGID: e.anonGID, FSID: e.fsid, Extra: append([]string{}, e.extra...),
		})
	}
	return out
}

func cleanExport(req rpc.NFSExportReq, prev *nfsExp) (nfsExp, error) {
	dir, err := cleanLocalPath(req.Path)
	if err != nil {
		return nfsExp{}, err
	}
	client, err := cleanClient(req.Client)
	if err != nil {
		return nfsExp{}, err
	}
	flags, err := cleanFlags(req.Options, ExportOptions())
	if err != nil {
		return nfsExp{}, err
	}
	if !hasGroup(flags, "access", ExportOptions()) {
		return nfsExp{}, fmt.Errorf("choose read/write or read-only")
	}
	uid, err := cleanID(req.AnonUID, "user id")
	if err != nil {
		return nfsExp{}, err
	}
	gid, err := cleanID(req.AnonGID, "group id")
	if err != nil {
		return nfsExp{}, err
	}
	fsid, err := cleanFSID(req.FSID)
	if err != nil {
		return nfsExp{}, err
	}
	row := nfsExp{path: dir, client: client, flags: flags, anonUID: uid, anonGID: gid, fsid: fsid}
	if prev != nil {
		row.extra = append([]string{}, prev.extra...)
		row.id = prev.id
	}
	return row, nil
}

func cleanMountReq(req rpc.NFSMountReq) (string, string, error) {
	source, err := cleanNFSSource(req.Source)
	if err != nil {
		return "", "", err
	}
	mp, err := cleanLocalPath(req.MountPoint)
	if err != nil {
		return "", "", err
	}
	return source, mp, nil
}

func cleanLocalPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.ContainsAny(raw, " \t\r\n") {
		return "", fmt.Errorf("use an absolute path without spaces")
	}
	c := path.Clean(raw)
	if strings.TrimRight(raw, "/") != c {
		return "", fmt.Errorf("path is not allowed")
	}
	if c == "/" || c == "." {
		return "", fmt.Errorf("that path is not allowed")
	}
	for _, p := range []string{"/proc", "/sys", "/dev", "/run", "/boot", "/etc", "/bin", "/sbin", "/lib", "/lib64", "/root", "/usr/bin", "/usr/sbin"} {
		if c == p || strings.HasPrefix(c, p+"/") {
			return "", fmt.Errorf("that path is not allowed")
		}
	}
	return c, nil
}

func cleanClient(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "*":
		return raw, nil
	case strings.HasPrefix(raw, "@") && hostToken(raw[1:]):
		return raw, nil
	case strings.HasPrefix(raw, "*.") && hostToken(raw[2:]):
		return raw, nil
	}
	if ip, _, err := net.ParseCIDR(raw); err == nil && ip != nil {
		return raw, nil
	}
	if net.ParseIP(raw) != nil {
		return raw, nil
	}
	if hostToken(raw) {
		return raw, nil
	}
	return "", fmt.Errorf("client must be an address, network, hostname, or *")
}

func cleanNFSSource(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	host, remote, ok := splitNFSSource(raw)
	if !ok {
		return "", fmt.Errorf("source must look like server:/share")
	}
	if _, err := cleanClient(host); err != nil || host == "*" || strings.HasPrefix(host, "@") || strings.HasPrefix(host, "*.") {
		return "", fmt.Errorf("server must be an address or hostname")
	}
	if strings.Contains(host, "/") {
		return "", fmt.Errorf("server must be an address or hostname")
	}
	if remote != "/" {
		c := path.Clean(remote)
		if !strings.HasPrefix(remote, "/") || strings.ContainsAny(remote, " \t\r\n") || strings.TrimRight(remote, "/") != c {
			return "", fmt.Errorf("remote path is not allowed")
		}
		remote = c
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + remote, nil
	}
	return host + ":" + remote, nil
}

func splitNFSSource(raw string) (host, remote string, ok bool) {
	if strings.HasPrefix(raw, "[") {
		end := strings.Index(raw, "]:")
		if end <= 1 {
			return "", "", false
		}
		return raw[1:end], raw[end+2:], raw[end+2:] != ""
	}
	i := strings.Index(raw, ":")
	if i <= 0 || i == len(raw)-1 {
		return "", "", false
	}
	return raw[:i], raw[i+1:], true
}

func cleanFlags(flags []string, catalog []rpc.NFSOption) ([]string, error) {
	known := map[string]rpc.NFSOption{}
	order := map[string]int{}
	for i, o := range catalog {
		known[o.Key] = o
		order[o.Key] = i
	}
	seen := map[string]bool{}
	var out []string
	groups := map[string]int{}
	for _, f := range flags {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		opt, ok := known[f]
		if !ok {
			return nil, fmt.Errorf("unknown option %s", f)
		}
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
		if opt.Exclusive && opt.Group != "" {
			groups[opt.Group]++
			if groups[opt.Group] > 1 {
				return nil, fmt.Errorf("choose only one %s option", opt.Group)
			}
		}
	}
	sortBy(out, order)
	return out, nil
}

func sortBy(items []string, order map[string]int) {
	for i := 1; i < len(items); i++ {
		j := i
		for j > 0 && order[items[j]] < order[items[j-1]] {
			items[j], items[j-1] = items[j-1], items[j]
			j--
		}
	}
}

func hasGroup(flags []string, group string, catalog []rpc.NFSOption) bool {
	want := map[string]bool{}
	for _, o := range catalog {
		if o.Group == group {
			want[o.Key] = true
		}
	}
	for _, f := range flags {
		if want[f] {
			return true
		}
	}
	return false
}

func choice(value string, choices []rpc.NFSChoice, label string) (string, error) {
	value = strings.TrimSpace(value)
	for _, c := range choices {
		if c.Value == value {
			return value, nil
		}
	}
	return "", fmt.Errorf("unknown %s", label)
}

func cleanID(raw, label string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	n, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return "", fmt.Errorf("%s must be a number", label)
	}
	return strconv.FormatUint(n, 10), nil
}

func cleanFSID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "root" {
		return raw, nil
	}
	if n, err := strconv.Atoi(raw); err == nil && n >= 0 && n <= 65535 {
		return strconv.Itoa(n), nil
	}
	return "", fmt.Errorf("fsid must be a number or root")
}

func hostToken(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func findExport(entries []nfsExp, id int) *nfsExp {
	for i := range entries {
		if entries[i].id == id {
			return &entries[i]
		}
	}
	return nil
}

func exportTaken(entries []nfsExp, skip int, dir, client string) bool {
	for _, e := range entries {
		if e.id == skip {
			continue
		}
		if e.path == dir && strings.EqualFold(e.client, client) {
			return true
		}
	}
	return false
}

func parseMountOptions(source, mp, opt string) rpc.NFSMount {
	row := rpc.NFSMount{Source: source, MountPoint: mp}
	known := map[string]bool{}
	for _, o := range MountOptions() {
		known[o.Key] = true
	}
	for _, part := range strings.Split(opt, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "defaults" {
			continue
		}
		switch {
		case strings.HasPrefix(part, "nfsvers="), strings.HasPrefix(part, "vers="):
			row.NFSVers = strings.TrimPrefix(strings.TrimPrefix(part, "nfsvers="), "vers=")
		case strings.HasPrefix(part, "proto="):
			row.Proto = strings.TrimPrefix(part, "proto=")
		case strings.HasPrefix(part, "timeo="):
			row.Timeo, _ = strconv.Atoi(strings.TrimPrefix(part, "timeo="))
		case strings.HasPrefix(part, "retrans="):
			row.Retrans, _ = strconv.Atoi(strings.TrimPrefix(part, "retrans="))
		case strings.HasPrefix(part, "rsize="):
			row.RSize, _ = strconv.Atoi(strings.TrimPrefix(part, "rsize="))
		case strings.HasPrefix(part, "wsize="):
			row.WSize, _ = strconv.Atoi(strings.TrimPrefix(part, "wsize="))
		case known[part]:
			row.Options = append(row.Options, part)
		}
	}
	return row
}

func fstabFields(line string) (dev, mp, fs, opt string, ok bool) {
	trim := strings.TrimSpace(line)
	if trim == "" || strings.HasPrefix(trim, "#") {
		return "", "", "", "", false
	}
	f := strings.Fields(trim)
	if len(f) < 4 {
		return "", "", "", "", false
	}
	return f[0], f[1], f[2], f[3], true
}

func joinContinued(text string) []string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	var lines []string
	buf := ""
	for _, line := range strings.Split(text, "\n") {
		trimRight := strings.TrimRight(line, " \t")
		if strings.HasSuffix(trimRight, "\\") {
			buf += strings.TrimSuffix(trimRight, "\\") + " "
			continue
		}
		lines = append(lines, buf+line)
		buf = ""
	}
	if buf != "" {
		lines = append(lines, buf)
	}
	return lines
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
