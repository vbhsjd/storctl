package storctl

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	localProfilePath  = "storctl-profiles.json"
	systemProfilePath = "/etc/storctl/profiles.json"
)

type ProfilesFile struct {
	Profiles map[string]Profile `json:"profiles"`
}

type Profile struct {
	VLANID        int            `json:"vlan_id"`
	Gateway       string         `json:"gateway"`
	Prefix        int            `json:"prefix"`
	RouteTable    int            `json:"route_table"`
	MTU           int            `json:"mtu"`
	ArtifactDir   string         `json:"artifact_dir"`
	ThirdOctetMap map[string]int `json:"third_octet_map"`
	QoS           QoSConfig      `json:"qos"`
	Mounts        []ProfileMount `json:"mounts"`
}

type ProfileMount struct {
	Server     string `json:"server"`
	Export     string `json:"export"`
	MountPoint string `json:"mount_point"`
	Options    string `json:"options"`
}

func resolveConfig(flags configFlags) (Config, error) {
	cfg := flags.Config
	cfg.Mounts = []MountSpec(flags.Mounts)

	if flags.Profile != "" {
		profile, err := loadNamedProfile(flags.ProfileFile, flags.Profile)
		if err != nil {
			return Config{}, err
		}
		cfg = applyProfileDefaults(cfg, profile, flags.seen)
		if cfg.DataCIDR == "" {
			mgmtIP, err := resolveManagementIP(flags.MgmtIP)
			if err != nil {
				return Config{}, err
			}
			dataCIDR, err := deriveDataCIDR(mgmtIP, profile)
			if err != nil {
				return Config{}, err
			}
			cfg.DataCIDR = dataCIDR
		}
	}

	return cfg, nil
}

func applyProfileDefaults(cfg Config, profile Profile, seen map[string]bool) Config {
	if !seen["vlan-id"] && profile.VLANID != 0 {
		cfg.VLANID = profile.VLANID
	}
	if !seen["gateway"] && profile.Gateway != "" {
		cfg.Gateway = profile.Gateway
	}
	if !seen["route-table"] && profile.RouteTable != 0 {
		cfg.RouteTable = profile.RouteTable
	}
	if !seen["mtu"] && profile.MTU != 0 {
		cfg.MTU = profile.MTU
	}
	if !seen["artifact-dir"] && profile.ArtifactDir != "" {
		cfg.ArtifactDir = profile.ArtifactDir
	}
	if profile.QoS.Enabled {
		cfg.QoS = profile.QoS
		if !seen["qos"] {
			cfg.QoSMode = "apply"
		}
	}
	if !seen["mount"] && len(profile.Mounts) > 0 {
		cfg.Mounts = profileMounts(profile.Mounts)
	}
	return cfg
}

func profileMounts(in []ProfileMount) []MountSpec {
	out := make([]MountSpec, 0, len(in))
	for _, m := range in {
		options := defaultNFSOptions
		if strings.TrimSpace(m.Options) != "" {
			options = mergeNFSOptions(defaultNFSOptions, strings.TrimSpace(m.Options))
		}
		out = append(out, MountSpec{
			Server:     strings.TrimSpace(m.Server),
			Export:     strings.TrimSpace(m.Export),
			MountPoint: strings.TrimSpace(m.MountPoint),
			Options:    options,
		})
	}
	return out
}

func loadNamedProfile(path, name string) (Profile, error) {
	profiles, usedPath, err := loadProfiles(path)
	if err != nil {
		return Profile{}, err
	}
	profile, ok := profiles.Profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("profile %q not found in %s", name, usedPath)
	}
	return profile, nil
}

func loadProfiles(path string) (ProfilesFile, string, error) {
	if path != "" {
		profiles, err := readProfiles(path)
		return profiles, path, err
	}
	for _, candidate := range []string{localProfilePath, systemProfilePath} {
		if _, err := os.Stat(candidate); err == nil {
			profiles, err := readProfiles(candidate)
			return profiles, candidate, err
		}
	}
	return ProfilesFile{}, "", fmt.Errorf("profile file not found; checked ./%s and %s", localProfilePath, systemProfilePath)
}

func readProfiles(path string) (ProfilesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProfilesFile{}, err
	}
	var profiles ProfilesFile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return ProfilesFile{}, err
	}
	if len(profiles.Profiles) == 0 {
		return ProfilesFile{}, fmt.Errorf("profile file has no profiles")
	}
	return profiles, nil
}

func ValidateProfiles(path string) error {
	profiles, err := readProfiles(path)
	if err != nil {
		return err
	}
	for name, profile := range profiles.Profiles {
		if profile.VLANID < 1 || profile.VLANID > 4094 {
			return fmt.Errorf("profile %s vlan_id must be 1..4094", name)
		}
		if net.ParseIP(profile.Gateway) == nil {
			return fmt.Errorf("profile %s gateway must be a valid IP", name)
		}
		if profile.Prefix < 0 || profile.Prefix > 32 {
			return fmt.Errorf("profile %s prefix must be 0..32", name)
		}
		if profile.RouteTable < 0 {
			return fmt.Errorf("profile %s route_table must not be negative", name)
		}
		if profile.MTU != 0 && profile.MTU < 1500 {
			return fmt.Errorf("profile %s mtu must be at least 1500", name)
		}
		for key, value := range profile.ThirdOctetMap {
			if _, err := strconv.Atoi(key); err != nil {
				return fmt.Errorf("profile %s third_octet_map key %q must be an integer", name, key)
			}
			if value < 0 || value > 255 {
				return fmt.Errorf("profile %s third_octet_map value for %s must be 0..255", name, key)
			}
		}
		if len(profile.Mounts) == 0 {
			return fmt.Errorf("profile %s must define at least one mount", name)
		}
		for _, mount := range profile.Mounts {
			if mount.Server == "" || !strings.HasPrefix(mount.Export, "/") || !strings.HasPrefix(mount.MountPoint, "/") {
				return fmt.Errorf("profile %s has invalid mount", name)
			}
		}
	}
	return nil
}

func resolveManagementIP(explicit string) (net.IP, error) {
	if explicit != "" {
		ip := net.ParseIP(explicit)
		if ip == nil || ip.To4() == nil {
			return nil, fmt.Errorf("--mgmt-ip must be an IPv4 address")
		}
		return ip.To4(), nil
	}
	ips, err := candidateManagementIPs()
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("can not infer management IP; pass --mgmt-ip")
	}
	if len(ips) > 1 {
		parts := make([]string, 0, len(ips))
		for _, ip := range ips {
			parts = append(parts, ip.String())
		}
		return nil, fmt.Errorf("multiple management IP candidates: %s; pass --mgmt-ip", strings.Join(parts, ","))
	}
	return ips[0], nil
}

func candidateManagementIPs() ([]net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isIgnoredInterface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipFromAddr(addr)
			if ip == nil || !isManagementCandidate(ip) {
				continue
			}
			out = append(out, ip)
		}
	}
	return out, nil
}

func ipFromAddr(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP.To4()
	case *net.IPAddr:
		return v.IP.To4()
	default:
		return nil
	}
}

func isIgnoredInterface(name string) bool {
	for _, prefix := range []string{"lo", "docker", "virbr", "veth", "cni", "flannel", "kube", "data"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isManagementCandidate(ip net.IP) bool {
	if ip == nil || ip.To4() == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	octets := ip.To4()
	if octets[0] == 172 && octets[1] == 27 {
		return false
	}
	if octets[0] == 172 && octets[1] == 17 {
		return false
	}
	if octets[0] == 192 && octets[1] == 168 && octets[2] == 122 {
		return false
	}
	return true
}

func deriveDataCIDR(mgmtIP net.IP, profile Profile) (string, error) {
	if mgmtIP == nil || mgmtIP.To4() == nil {
		return "", fmt.Errorf("management IP is not IPv4")
	}
	prefix := profile.Prefix
	if prefix == 0 {
		prefix = 18
	}
	third := int(mgmtIP.To4()[2])
	fourth := int(mgmtIP.To4()[3])
	mapped, ok := profile.ThirdOctetMap[strconv.Itoa(third)]
	if !ok {
		return "", fmt.Errorf("no third_octet_map entry for management third octet %d", third)
	}
	return fmt.Sprintf("172.27.%d.%d/%d", mapped, fourth, prefix), nil
}
