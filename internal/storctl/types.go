package storctl

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	defaultMTU        = 5500
	defaultRouteTable = 5000
	defaultNFSOptions = "vers=4.1,proto=rdma,port=20049,rsize=1048576,wsize=1048576,hard,noatime"
)

type Config struct {
	NIC             string
	NICType         string
	VLANID          int
	DataCIDR        string
	Gateway         string
	RouteTable      int
	MTU             int
	ArtifactDir     string
	Proxy           string
	NoProxy         string
	UpgradeFirmware bool
	Mounts          []MountSpec
	StateDir        string
}

type MountSpec struct {
	Server     string
	Export     string
	MountPoint string
	Options    string
}

type mountList []MountSpec

func (m *mountList) String() string {
	parts := make([]string, 0, len(*m))
	for _, spec := range *m {
		parts = append(parts, spec.String())
	}
	return strings.Join(parts, ",")
}

func (m *mountList) Set(value string) error {
	spec, err := ParseMountSpec(value)
	if err != nil {
		return err
	}
	*m = append(*m, spec)
	return nil
}

func (m MountSpec) String() string {
	base := fmt.Sprintf("%s:%s:%s", m.Server, m.Export, m.MountPoint)
	if m.Options != "" {
		return base + ":" + m.Options
	}
	return base
}

func ParseMountSpec(raw string) (MountSpec, error) {
	parts := strings.SplitN(raw, ":", 4)
	if len(parts) < 3 {
		return MountSpec{}, fmt.Errorf("mount must be server:/export:/mount[:opts]")
	}
	spec := MountSpec{
		Server:     strings.TrimSpace(parts[0]),
		Export:     strings.TrimSpace(parts[1]),
		MountPoint: strings.TrimSpace(parts[2]),
		Options:    defaultNFSOptions,
	}
	if len(parts) == 4 && strings.TrimSpace(parts[3]) != "" {
		spec.Options = mergeNFSOptions(defaultNFSOptions, strings.TrimSpace(parts[3]))
	}
	if spec.Server == "" || spec.Export == "" || spec.MountPoint == "" {
		return MountSpec{}, fmt.Errorf("mount fields can not be empty")
	}
	if !strings.HasPrefix(spec.Export, "/") {
		return MountSpec{}, fmt.Errorf("mount export must start with /")
	}
	if !strings.HasPrefix(spec.MountPoint, "/") {
		return MountSpec{}, fmt.Errorf("mount point must start with /")
	}
	return spec, nil
}

func mergeNFSOptions(base, extra string) string {
	if extra == "" {
		return base
	}
	baseMap := map[string]string{}
	order := []string{}
	for _, opt := range strings.Split(base, ",") {
		key := strings.SplitN(opt, "=", 2)[0]
		baseMap[key] = opt
		order = append(order, key)
	}
	for _, opt := range strings.Split(extra, ",") {
		opt = strings.TrimSpace(opt)
		if opt == "" {
			continue
		}
		key := strings.SplitN(opt, "=", 2)[0]
		if _, ok := baseMap[key]; !ok {
			order = append(order, key)
		}
		baseMap[key] = opt
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, baseMap[key])
	}
	return strings.Join(out, ",")
}

func parseApply(args []string) (Config, error) {
	var mounts mountList
	cfg := Config{
		NICType:     "auto",
		MTU:         defaultMTU,
		RouteTable:  defaultRouteTable,
		StateDir:    "/var/lib/storctl",
		ArtifactDir: "/root/storage_pkgs",
	}
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.StringVar(&cfg.NIC, "nic", "", "physical NIC name")
	fs.StringVar(&cfg.NICType, "nic-type", "auto", "nic type: auto, cx7, 1823")
	fs.IntVar(&cfg.VLANID, "vlan-id", 0, "VLAN ID")
	fs.StringVar(&cfg.DataCIDR, "data-ip", "", "data IP CIDR, for example 172.27.1.123/18")
	fs.StringVar(&cfg.Gateway, "gateway", "", "VLAN gateway")
	fs.IntVar(&cfg.RouteTable, "route-table", defaultRouteTable, "routing table id")
	fs.IntVar(&cfg.MTU, "mtu", defaultMTU, "MTU")
	fs.StringVar(&cfg.ArtifactDir, "artifact-dir", cfg.ArtifactDir, "local artifact directory")
	fs.StringVar(&cfg.Proxy, "proxy", "", "HTTP/HTTPS proxy for package manager commands")
	fs.StringVar(&cfg.NoProxy, "no-proxy", "", "NO_PROXY value for package manager commands")
	fs.BoolVar(&cfg.UpgradeFirmware, "upgrade-firmware", false, "upgrade NIC firmware")
	fs.StringVar(&cfg.StateDir, "state-dir", cfg.StateDir, "state directory")
	fs.Var(&mounts, "mount", "NFS mount: server:/export:/mount[:opts], repeatable")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	cfg.Mounts = mounts
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	if c.NIC == "" {
		return errors.New("--nic is required")
	}
	switch c.NICType {
	case "auto", "cx7", "1823":
	default:
		return errors.New("--nic-type must be auto, cx7, or 1823")
	}
	if c.VLANID < 1 || c.VLANID > 4094 {
		return errors.New("--vlan-id must be 1..4094")
	}
	if c.DataCIDR == "" {
		return errors.New("--data-ip is required")
	}
	ip, _, err := net.ParseCIDR(c.DataCIDR)
	if err != nil || ip == nil {
		return errors.New("--data-ip must be a valid CIDR")
	}
	if net.ParseIP(c.Gateway) == nil {
		return errors.New("--gateway must be a valid IP")
	}
	if c.RouteTable < 1 {
		return errors.New("--route-table must be positive")
	}
	if c.MTU < 1500 {
		return errors.New("--mtu must be at least 1500")
	}
	if len(c.Mounts) == 0 {
		return errors.New("at least one --mount is required")
	}
	return nil
}

func (c Config) dataIPOnly() string {
	if strings.Contains(c.DataCIDR, "/") {
		return strings.SplitN(c.DataCIDR, "/", 2)[0]
	}
	return c.DataCIDR
}

func (c Config) vlanName() string {
	return fmt.Sprintf("data0.%d", c.VLANID)
}

func (c Config) physicalConName() string {
	return "storctl-" + c.NIC
}

func parseCheck(args []string) (Config, error) {
	cfg := Config{StateDir: "/var/lib/storctl"}
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.StringVar(&cfg.StateDir, "state-dir", cfg.StateDir, "state directory")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	return cfg, nil
}

func parsePositiveInt(raw string) (int, bool) {
	n, err := strconv.Atoi(raw)
	return n, err == nil && n > 0
}
