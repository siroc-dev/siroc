package sysops

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const sirocNetplanName = "99-siroc.yaml"

const sirocNetplanHeader = "# Siroc network addresses. This file is kept across reboot.\n"

// netplanPlan is a set of /etc/netplan file changes. Names are base names.
type netplanPlan struct {
	Update map[string]string
	Delete []string
}

type sirocNetplan struct {
	Network sirocNetwork `yaml:"network"`
}

// netplanDeviceSections are the device groups that can hold an address.
// A later file must use the same group. Writing a bond under ethernets makes
// netplan reject the file because the device type changed.
var netplanDeviceSections = []string{
	"ethernets", "bonds", "bridges", "vlans", "tunnels", "vrfs", "dummy-devices",
}

type sirocNetwork struct {
	Version      int                     `yaml:"version"`
	Ethernets    map[string]*sirocDevice `yaml:"ethernets,omitempty"`
	Bonds        map[string]*sirocDevice `yaml:"bonds,omitempty"`
	Bridges      map[string]*sirocDevice `yaml:"bridges,omitempty"`
	VLANs        map[string]*sirocDevice `yaml:"vlans,omitempty"`
	Tunnels      map[string]*sirocDevice `yaml:"tunnels,omitempty"`
	VRFs         map[string]*sirocDevice `yaml:"vrfs,omitempty"`
	DummyDevices map[string]*sirocDevice `yaml:"dummy-devices,omitempty"`
}

type sirocDevice struct {
	ID             int          `yaml:"id,omitempty"`
	Link           string       `yaml:"link,omitempty"`
	Addresses      []string     `yaml:"addresses"`
	Routes         []sirocRoute `yaml:"routes,omitempty"`
	DHCP4Overrides *sirocDHCP4  `yaml:"dhcp4-overrides,omitempty"`
}

type sirocDHCP4 struct {
	UseRoutes *bool `yaml:"use-routes"`
}

type sirocRoute struct {
	To     string `yaml:"to"`
	Via    string `yaml:"via,omitempty"`
	OnLink bool   `yaml:"on-link,omitempty"`
	Metric *int   `yaml:"metric,omitempty"`
	Table  int    `yaml:"table,omitempty"`
}

type npDev struct {
	dhcp4     bool
	addresses []string
	routes    []sirocRoute
}

func normalizeNetAddr(addr, gateway string) (cidr, gw string, err error) {
	cidr, err = canonicalCIDR(addr)
	if err != nil {
		return "", "", fmt.Errorf("address must be CIDR, e.g. 192.168.1.10/24")
	}
	gateway = strings.TrimSpace(gateway)
	if gateway == "" {
		return cidr, "", nil
	}
	ip := net.ParseIP(gateway)
	if ip == nil || ip.IsUnspecified() {
		return "", "", fmt.Errorf("invalid gateway")
	}
	gw = ip.String()
	host, _, _ := net.ParseCIDR(cidr)
	if host.Equal(ip) {
		return "", "", fmt.Errorf("gateway must be a different address")
	}
	if (host.To4() == nil) != (ip.To4() == nil) {
		return "", "", fmt.Errorf("gateway must be the same IP version as the address")
	}
	return cidr, gw, nil
}

func canonicalCIDR(addr string) (string, error) {
	ip, network, err := net.ParseCIDR(strings.TrimSpace(addr))
	if err != nil {
		return "", err
	}
	ones, _ := network.Mask.Size()
	return fmt.Sprintf("%s/%d", ip.String(), ones), nil
}

func osReleaseID(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") {
			return strings.Trim(strings.TrimPrefix(line, "ID="), `"'`)
		}
	}
	return ""
}

// planNetplanAdd records one address, and an optional default route, in the
// Siroc netplan file. That file is applied last, so its address list replaces
// the list from an earlier file such as cloud-init. The earlier file is left
// in place except for gateway4 and gateway6, which would otherwise add a
// second default route.
func planNetplanAdd(files map[string]string, section, dev, parent string, vlanID int, cidr, gw string) (netplanPlan, error) {
	section = deviceSection(files, dev, vlanID, section)
	devs, err := mergeDocs(files)
	if err != nil {
		return netplanPlan{}, err
	}
	merged := devs[section][dev]
	var have []string
	var routes []sirocRoute
	dhcp4 := false
	if merged != nil {
		have = merged.addresses
		routes = merged.routes
		dhcp4 = merged.dhcp4
	}
	// A previous file may have saved this device under the wrong group.
	// Keep those addresses, then drop the wrong group so the type matches.
	for _, other := range netplanDeviceSections {
		if other == section || devs[other] == nil || devs[other][dev] == nil {
			continue
		}
		have = append(have, devs[other][dev].addresses...)
	}
	list := canonList(have)
	if containsAddr(list, cidr) {
		if gw == "" {
			return netplanPlan{}, fmt.Errorf("address already exists")
		}
	} else {
		list = append(list, cidr)
	}
	overlay, err := parseOverlay(files[sirocNetplanName])
	if err != nil {
		return netplanPlan{}, err
	}
	dropOtherSections(&overlay, section, dev)
	devices := overlayDevices(&overlay, section)
	cur := devices[dev]
	if cur == nil {
		cur = &sirocDevice{}
		devices[dev] = cur
	}
	cur.Addresses = list
	if vlanID > 0 {
		cur.ID = vlanID
		cur.Link = parent
	}
	plan := netplanPlan{Update: map[string]string{}}
	if gw != "" {
		cur.Routes = replaceDefault(append([]sirocRoute{}, routes...), gw)
		if dhcp4 {
			off := false
			cur.DHCP4Overrides = &sirocDHCP4{UseRoutes: &off}
		}
		for name, content := range files {
			if name == sirocNetplanName {
				continue
			}
			next, changed, err := stripGatewayKeys(content, section, dev)
			if err != nil {
				return netplanPlan{}, err
			}
			if changed {
				plan.Update[name] = next
			}
		}
	}
	body, err := marshalOverlay(overlay)
	if err != nil {
		return netplanPlan{}, err
	}
	plan.Update[sirocNetplanName] = body
	return plan, nil
}

// planNetplanDel removes a static address from every netplan file. The last
// address on a VLAN also removes that VLAN, so the interface does not stay
// behind with no address.
func planNetplanDel(files map[string]string, iface, cidr string) (netplanPlan, error) {
	devs, err := mergeDocs(files)
	if err != nil {
		return netplanPlan{}, err
	}
	section := deviceSection(files, iface, 0, "")
	if section == "" || devs[section] == nil || devs[section][iface] == nil || !containsAddr(canonList(devs[section][iface].addresses), cidr) {
		section = ""
		for _, candidate := range netplanDeviceSections {
			if devs[candidate][iface] != nil && containsAddr(canonList(devs[candidate][iface].addresses), cidr) {
				section = candidate
				break
			}
		}
	}
	if section == "" {
		return netplanPlan{}, nil
	}
	plan := netplanPlan{Update: map[string]string{}}
	changedFiles := map[string]string{}
	for name, content := range files {
		next, changed, err := removeCIDR(content, section, iface, cidr)
		if err != nil {
			return netplanPlan{}, err
		}
		if changed {
			changedFiles[name] = next
		} else {
			changedFiles[name] = content
		}
	}
	if content, ok := changedFiles[sirocNetplanName]; ok {
		for _, other := range netplanDeviceSections {
			if other == section {
				continue
			}
			next, changed, err := removeNetplanDevice(content, other, iface)
			if err != nil {
				return netplanPlan{}, err
			}
			if changed {
				content = next
				changedFiles[sirocNetplanName] = next
			}
		}
	}
	if section == "vlans" && !vlanHasAddress(changedFiles, iface) {
		for name, content := range changedFiles {
			next, changed, err := removeNetplanDevice(content, "vlans", iface)
			if err != nil {
				return netplanPlan{}, err
			}
			if changed {
				changedFiles[name] = next
			}
		}
	}
	for name, content := range files {
		next := changedFiles[name]
		if next == content {
			continue
		}
		if name == sirocNetplanName && netplanBlank(next) {
			plan.Delete = append(plan.Delete, name)
			continue
		}
		plan.Update[name] = next
	}
	sort.Strings(plan.Delete)
	return plan, nil
}

func deviceSection(files map[string]string, dev string, vlanID int, fallback string) string {
	if vlanID > 0 {
		return "vlans"
	}
	if section := definedSection(files, dev, true); section != "" {
		return section
	}
	if section := definedSection(files, dev, false); section != "" {
		return section
	}
	if fallback != "" {
		return fallback
	}
	return "ethernets"
}

// definedSection reports the device group in netplan files. skipSiroc ignores
// 99-siroc.yaml so a bond saved earlier as an ethernet does not hide the real type.
func definedSection(files map[string]string, dev string, skipSiroc bool) string {
	names := make([]string, 0, len(files))
	for name := range files {
		if skipSiroc && name == sirocNetplanName {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	found := ""
	for _, name := range names {
		var doc map[string]any
		if yaml.Unmarshal([]byte(files[name]), &doc) != nil {
			continue
		}
		network, _ := doc["network"].(map[string]any)
		if network == nil {
			continue
		}
		for _, section := range netplanDeviceSections {
			sec, _ := network[section].(map[string]any)
			if _, ok := sec[dev]; ok {
				found = section
			}
		}
	}
	return found
}

func overlayDevices(doc *sirocNetplan, section string) map[string]*sirocDevice {
	switch section {
	case "bonds":
		if doc.Network.Bonds == nil {
			doc.Network.Bonds = map[string]*sirocDevice{}
		}
		return doc.Network.Bonds
	case "bridges":
		if doc.Network.Bridges == nil {
			doc.Network.Bridges = map[string]*sirocDevice{}
		}
		return doc.Network.Bridges
	case "vlans":
		if doc.Network.VLANs == nil {
			doc.Network.VLANs = map[string]*sirocDevice{}
		}
		return doc.Network.VLANs
	case "tunnels":
		if doc.Network.Tunnels == nil {
			doc.Network.Tunnels = map[string]*sirocDevice{}
		}
		return doc.Network.Tunnels
	case "vrfs":
		if doc.Network.VRFs == nil {
			doc.Network.VRFs = map[string]*sirocDevice{}
		}
		return doc.Network.VRFs
	case "dummy-devices":
		if doc.Network.DummyDevices == nil {
			doc.Network.DummyDevices = map[string]*sirocDevice{}
		}
		return doc.Network.DummyDevices
	default:
		if doc.Network.Ethernets == nil {
			doc.Network.Ethernets = map[string]*sirocDevice{}
		}
		return doc.Network.Ethernets
	}
}

func dropOtherSections(doc *sirocNetplan, keep, name string) {
	if keep != "ethernets" {
		delete(doc.Network.Ethernets, name)
	}
	if keep != "bonds" {
		delete(doc.Network.Bonds, name)
	}
	if keep != "bridges" {
		delete(doc.Network.Bridges, name)
	}
	if keep != "vlans" {
		delete(doc.Network.VLANs, name)
	}
	if keep != "tunnels" {
		delete(doc.Network.Tunnels, name)
	}
	if keep != "vrfs" {
		delete(doc.Network.VRFs, name)
	}
	if keep != "dummy-devices" {
		delete(doc.Network.DummyDevices, name)
	}
}

func parseOverlay(content string) (sirocNetplan, error) {
	doc := sirocNetplan{Network: sirocNetwork{Version: 2}}
	if strings.TrimSpace(content) == "" {
		return doc, nil
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return sirocNetplan{}, fmt.Errorf("%s: %w", sirocNetplanName, err)
	}
	if doc.Network.Version == 0 {
		doc.Network.Version = 2
	}
	return doc, nil
}

func marshalOverlay(doc sirocNetplan) (string, error) {
	if len(doc.Network.Ethernets) == 0 {
		doc.Network.Ethernets = nil
	}
	if len(doc.Network.Bonds) == 0 {
		doc.Network.Bonds = nil
	}
	if len(doc.Network.Bridges) == 0 {
		doc.Network.Bridges = nil
	}
	if len(doc.Network.VLANs) == 0 {
		doc.Network.VLANs = nil
	}
	if len(doc.Network.Tunnels) == 0 {
		doc.Network.Tunnels = nil
	}
	if len(doc.Network.VRFs) == 0 {
		doc.Network.VRFs = nil
	}
	if len(doc.Network.DummyDevices) == 0 {
		doc.Network.DummyDevices = nil
	}
	doc.Network.Version = 2
	var buf bytes.Buffer
	buf.WriteString(sirocNetplanHeader)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err := enc.Encode(doc)
	_ = enc.Close()
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func mergeDocs(files map[string]string) (map[string]map[string]*npDev, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	out := map[string]map[string]*npDev{}
	for _, section := range netplanDeviceSections {
		out[section] = map[string]*npDev{}
	}
	for _, name := range names {
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(files[name]), &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		network, _ := doc["network"].(map[string]any)
		if network == nil {
			continue
		}
		for _, section := range netplanDeviceSections {
			absorbDevices(out[section], network[section])
		}
	}
	return out, nil
}

func absorbDevices(dst map[string]*npDev, raw any) {
	sec, ok := raw.(map[string]any)
	if !ok {
		return
	}
	for name, val := range sec {
		dev, ok := val.(map[string]any)
		if !ok {
			continue
		}
		cur := dst[name]
		if cur == nil {
			cur = &npDev{}
			dst[name] = cur
		}
		if b, ok := dev["dhcp4"].(bool); ok {
			cur.dhcp4 = b
		}
		if list, ok := stringList(dev["addresses"]); ok {
			cur.addresses = list
		}
		if routes, ok := parseRoutes(dev["routes"]); ok {
			cur.routes = routes
		}
	}
}

func stringList(v any) ([]string, bool) {
	switch t := v.(type) {
	case nil:
		return nil, false
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil, false
		}
		return []string{s}, true
	case []any:
		var out []string
		for _, item := range t {
			s := strings.TrimSpace(fmt.Sprint(item))
			if s != "" && s != "<nil>" {
				out = append(out, s)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func parseRoutes(v any) ([]sirocRoute, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	var out []sirocRoute
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		to, _ := m["to"].(string)
		if to == "" {
			continue
		}
		route := sirocRoute{To: to}
		if via, ok := m["via"].(string); ok {
			route.Via = via
		}
		if on, ok := m["on-link"].(bool); ok {
			route.OnLink = on
		}
		if n, ok := asInt(m["metric"]); ok {
			route.Metric = &n
		}
		if n, ok := asInt(m["table"]); ok {
			route.Table = n
		}
		out = append(out, route)
	}
	return out, true
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case uint64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func canonList(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		c, err := canonicalCIDR(s)
		if err != nil {
			c = strings.TrimSpace(s)
		}
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

func containsAddr(list []string, cidr string) bool {
	for _, s := range list {
		if s == cidr {
			return true
		}
	}
	return false
}

func replaceDefault(routes []sirocRoute, via string) []sirocRoute {
	v4 := net.ParseIP(via).To4() != nil
	out := make([]sirocRoute, 0, len(routes)+1)
	for _, route := range routes {
		if v4 && v4Default(route) {
			continue
		}
		if !v4 && v6Default(route) {
			continue
		}
		out = append(out, route)
	}
	to := "default"
	if !v4 {
		to = "::/0"
	}
	out = append(out, sirocRoute{To: to, Via: via})
	return out
}

func v4Default(route sirocRoute) bool {
	if route.To == "0.0.0.0/0" {
		return true
	}
	if route.To != "default" {
		return false
	}
	ip := net.ParseIP(route.Via)
	return ip == nil || ip.To4() != nil
}

func v6Default(route sirocRoute) bool {
	if route.To == "::/0" {
		return true
	}
	if route.To != "default" {
		return false
	}
	ip := net.ParseIP(route.Via)
	return ip != nil && ip.To4() == nil
}

func vlanHasAddress(files map[string]string, iface string) bool {
	devs, err := mergeDocs(files)
	if err != nil || devs["vlans"][iface] == nil {
		return false
	}
	return len(canonList(devs["vlans"][iface].addresses)) > 0
}

func netplanBlank(content string) bool {
	var doc map[string]any
	if yaml.Unmarshal([]byte(content), &doc) != nil {
		return false
	}
	network, _ := doc["network"].(map[string]any)
	if network == nil {
		return true
	}
	for _, key := range []string{"ethernets", "vlans", "wifis", "bridges", "bonds"} {
		if m, ok := network[key].(map[string]any); ok && len(m) > 0 {
			return false
		}
	}
	return true
}

func stripGatewayKeys(content, section, name string) (string, bool, error) {
	return editDevice(content, section, name, func(dev *yaml.Node) bool {
		gone4 := mappingDel(dev, "gateway4")
		gone6 := mappingDel(dev, "gateway6")
		return gone4 || gone6
	})
}

func removeCIDR(content, section, name, cidr string) (string, bool, error) {
	return editDevice(content, section, name, func(dev *yaml.Node) bool {
		seq := mappingGet(dev, "addresses")
		if seq == nil || seq.Kind != yaml.SequenceNode {
			return false
		}
		next := make([]*yaml.Node, 0, len(seq.Content))
		gone := false
		for _, item := range seq.Content {
			got, err := canonicalCIDR(item.Value)
			if err == nil && got == cidr {
				gone = true
				continue
			}
			next = append(next, item)
		}
		if !gone {
			return false
		}
		seq.Content = next
		return true
	})
}

func removeNetplanDevice(content, section, name string) (string, bool, error) {
	var doc yaml.Node
	if strings.TrimSpace(content) == "" {
		return content, false, nil
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return "", false, err
	}
	root := documentRoot(&doc)
	network := mappingGet(root, "network")
	sec := mappingGet(network, section)
	if !mappingDel(sec, name) {
		return content, false, nil
	}
	if sec != nil && len(sec.Content) == 0 {
		mappingDel(network, section)
	}
	text, err := encodeNode(&doc)
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

func editDevice(content, section, name string, edit func(*yaml.Node) bool) (string, bool, error) {
	if strings.TrimSpace(content) == "" {
		return content, false, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return "", false, err
	}
	root := documentRoot(&doc)
	dev := mappingGet(mappingGet(mappingGet(root, "network"), section), name)
	if dev == nil || !edit(dev) {
		return content, false, nil
	}
	text, err := encodeNode(&doc)
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

func mappingGet(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func mappingDel(n *yaml.Node, key string) bool {
	if n == nil || n.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == key {
			n.Content = append(n.Content[:i], n.Content[i+2:]...)
			return true
		}
	}
	return false
}

func encodeNode(doc *yaml.Node) (string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err := enc.Encode(doc)
	_ = enc.Close()
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
