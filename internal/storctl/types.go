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
	defaultTCPOptions = "vers=3,proto=tcp,nolock,nconnect=8,hard,noatime"
)

type Config struct {
	NIC              string
	NICType          string
	MgmtIP           string
	VLANID           int
	DataCIDR         string
	Gateway          string
	RouteTable       int
	MTU              int
	ArtifactDir      string
	Proxy            string
	NoProxy          string
	UpgradeFirmware  bool
	AllowTCPFallback bool
	QoSMode          string
	QoS              QoSConfig
	CheckJSON        bool
	Mounts           []MountSpec
	StateDir         string
}

type QoSConfig struct {
	Enabled bool       `json:"enabled"`
	CX7     CX7QoS     `json:"cx7"`
	NIC1823 NIC1823QoS `json:"nic_1823"`
}

type CX7QoS struct {
	PFC    string `json:"pfc"`
	ToS    int    `json:"tos"`
	PrioTC string `json:"prio_tc"`
	TSA    string `json:"tsa"`
	TCBW   string `json:"tcbw"`
}

type NIC1823QoS struct {
	ECNAlgo    string `json:"ecn_algo"`
	PFC        string `json:"pfc"`
	Trust      string `json:"trust"`
	ETSClasses string `json:"ets_classes"`
	ETSWeights string `json:"ets_weights"`
}

type MountSpec struct {
	Server     string
	Export     string
	MountPoint string
	Options    string
}

type parseMode string

const (
	parseModeApply parseMode = "apply"
	parseModePlan  parseMode = "plan"
)

type configFlags struct {
	Config
	Profile     string
	ProfileFile string
	MgmtIP      string
	Mounts      mountList
	seen        map[string]bool
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
	return parseConfig(args, parseModeApply)
}

func parsePlan(args []string) (Config, error) {
	return parseConfig(args, parseModePlan)
}

func parseConfig(args []string, mode parseMode) (Config, error) {
	flags, err := parseConfigFlags(string(mode), args)
	if err != nil {
		return Config{}, err
	}
	cfg, err := resolveConfig(flags)
	if err != nil {
		return Config{}, err
	}
	return cfg, cfg.Validate()
}

func parseConfigFlags(name string, args []string) (configFlags, error) {
	var mounts mountList
	flags := configFlags{
		Config: Config{
			NICType:     "auto",
			QoSMode:     "off",
			MTU:         defaultMTU,
			RouteTable:  defaultRouteTable,
			StateDir:    "/var/lib/storctl",
			ArtifactDir: "/root/storage_pkgs",
		},
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&flags.Profile, "profile", "", "profile name")
	fs.StringVar(&flags.ProfileFile, "profile-file", "", "profile JSON path")
	fs.StringVar(&flags.MgmtIP, "mgmt-ip", "", "management IP used to derive data IP")
	fs.StringVar(&flags.NIC, "nic", "", "physical NIC name")
	fs.StringVar(&flags.NICType, "nic-type", "auto", "nic type: auto, cx7, 1823")
	fs.IntVar(&flags.VLANID, "vlan-id", 0, "VLAN ID")
	fs.StringVar(&flags.DataCIDR, "data-ip", "", "data IP CIDR, for example 172.27.1.123/18")
	fs.StringVar(&flags.Gateway, "gateway", "", "VLAN gateway")
	fs.IntVar(&flags.RouteTable, "route-table", defaultRouteTable, "routing table id")
	fs.IntVar(&flags.MTU, "mtu", defaultMTU, "MTU")
	fs.StringVar(&flags.ArtifactDir, "artifact-dir", flags.ArtifactDir, "local artifact directory")
	fs.StringVar(&flags.Proxy, "proxy", "", "HTTP/HTTPS proxy for package manager commands")
	fs.StringVar(&flags.NoProxy, "no-proxy", "", "NO_PROXY value for package manager commands")
	fs.BoolVar(&flags.UpgradeFirmware, "upgrade-firmware", false, "upgrade NIC firmware")
	fs.BoolVar(&flags.AllowTCPFallback, "allow-tcp-fallback", false, "mount TCP NFS when NFS-RDMA is unavailable")
	fs.StringVar(&flags.QoSMode, "qos", "off", "QoS mode: off or apply")
	fs.StringVar(&flags.StateDir, "state-dir", flags.StateDir, "state directory")
	fs.Var(&mounts, "mount", "NFS mount: server:/export:/mount[:opts], repeatable")
	if err := fs.Parse(args); err != nil {
		return configFlags{}, err
	}
	flags.Mounts = mounts
	flags.seen = map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		flags.seen[f.Name] = true
	})
	return flags, nil
}

func (c Config) Validate() error {
	if c.NIC == "" {
		return errors.New("--nic is required")
	}
	if c.NIC == "auto" {
		return errors.New("--nic must be an explicit interface name; auto selection is intentionally unsupported")
	}
	switch c.NICType {
	case "auto", "cx7", "1823":
	default:
		return errors.New("--nic-type must be auto, cx7, or 1823")
	}
	switch c.QoSMode {
	case "off", "apply":
	default:
		return errors.New("--qos must be off or apply")
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
	for _, m := range c.Mounts {
		if m.Server == "" || m.Export == "" || m.MountPoint == "" {
			return errors.New("mount fields can not be empty")
		}
		if !strings.HasPrefix(m.Export, "/") {
			return errors.New("mount export must start with /")
		}
		if !strings.HasPrefix(m.MountPoint, "/") {
			return errors.New("mount point must start with /")
		}
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
	fs.BoolVar(&cfg.CheckJSON, "json", false, "output stable JSON")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	return cfg, nil
}

func parseReconcileMounts(args []string) (Config, error) {
	var profileName, profileFile string
	var mounts mountList
	cfg := Config{AllowTCPFallback: false}
	fs := flag.NewFlagSet("reconcile-mounts", flag.ContinueOnError)
	fs.StringVar(&profileName, "profile", "", "profile name")
	fs.StringVar(&profileFile, "profile-file", "", "profile JSON path")
	fs.BoolVar(&cfg.AllowTCPFallback, "allow-tcp-fallback", false, "preserve TCP NFS fallback when current mount uses proto=tcp")
	fs.Var(&mounts, "mount", "NFS mount: server:/export:/mount[:opts], repeatable")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	if profileName != "" {
		profile, err := loadNamedProfile(profileFile, profileName)
		if err != nil {
			return Config{}, err
		}
		cfg.Mounts = profileMounts(profile.Mounts)
	}
	if len(mounts) > 0 {
		cfg.Mounts = []MountSpec(mounts)
	}
	if len(cfg.Mounts) == 0 {
		return Config{}, errors.New("--profile or at least one --mount is required")
	}
	for _, m := range cfg.Mounts {
		if m.Server == "" || m.Export == "" || m.MountPoint == "" {
			return Config{}, errors.New("mount fields can not be empty")
		}
		if !strings.HasPrefix(m.Export, "/") {
			return Config{}, errors.New("mount export must start with /")
		}
		if !strings.HasPrefix(m.MountPoint, "/") {
			return Config{}, errors.New("mount point must start with /")
		}
	}
	return cfg, nil
}

func parseInstallDriver(args []string) (InstallDriverConfig, error) {
	cfg := InstallDriverConfig{ArtifactDir: "/root/storage_pkgs"}
	fs := flag.NewFlagSet("install-driver", flag.ContinueOnError)
	fs.StringVar(&cfg.NICType, "nic-type", "", "nic type: cx7 or 1823")
	fs.StringVar(&cfg.ArtifactDir, "artifact-dir", cfg.ArtifactDir, "local artifact directory")
	fs.StringVar(&cfg.Proxy, "proxy", "", "HTTP/HTTPS proxy for package manager commands")
	fs.StringVar(&cfg.NoProxy, "no-proxy", "", "NO_PROXY value for package manager commands")
	fs.BoolVar(&cfg.UpgradeFirmware, "upgrade-firmware", false, "upgrade NIC firmware")
	fs.BoolVar(&cfg.AllowRepo, "allow-repo", false, "allow artifacts that require a configured dnf repo")
	if err := fs.Parse(args); err != nil {
		return InstallDriverConfig{}, err
	}
	if cfg.NICType != "cx7" && cfg.NICType != "1823" {
		return InstallDriverConfig{}, errors.New("--nic-type must be cx7 or 1823")
	}
	if cfg.ArtifactDir == "" {
		return InstallDriverConfig{}, errors.New("--artifact-dir is required")
	}
	return cfg, nil
}

func parseGenerateManifest(args []string) (ManifestGenerateConfig, error) {
	cfg := ManifestGenerateConfig{ArtifactDir: "/root/storage_pkgs", Arch: artifactArch()}
	fs := flag.NewFlagSet("generate-manifest", flag.ContinueOnError)
	fs.StringVar(&cfg.ArtifactDir, "artifact-dir", cfg.ArtifactDir, "local artifact directory")
	fs.StringVar(&cfg.OSID, "os-id", "", "OS ID, for example openEuler")
	fs.StringVar(&cfg.OSVersionPrefix, "os-version-prefix", "", "OS version prefix, for example 22.03-LTS-SP4")
	fs.StringVar(&cfg.Arch, "arch", cfg.Arch, "architecture, for example aarch64")
	if err := fs.Parse(args); err != nil {
		return ManifestGenerateConfig{}, err
	}
	if fs.NArg() != 0 {
		return ManifestGenerateConfig{}, fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	if cfg.ArtifactDir == "" || cfg.OSID == "" || cfg.OSVersionPrefix == "" || cfg.Arch == "" {
		return ManifestGenerateConfig{}, errors.New("--artifact-dir, --os-id, --os-version-prefix, and --arch are required")
	}
	return cfg, nil
}

func parseValidateProfile(args []string) (string, error) {
	var path string
	fs := flag.NewFlagSet("validate-profile", flag.ContinueOnError)
	fs.StringVar(&path, "profile-file", "", "profile JSON path")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 0 {
		return "", fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	if path == "" {
		return "", errors.New("--profile-file is required")
	}
	return path, nil
}

func parseValidateArtifacts(args []string) (string, error) {
	dir := "/root/storage_pkgs"
	fs := flag.NewFlagSet("validate-artifacts", flag.ContinueOnError)
	fs.StringVar(&dir, "artifact-dir", dir, "local artifact directory")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 0 {
		return "", fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	if dir == "" {
		return "", errors.New("--artifact-dir is required")
	}
	return dir, nil
}

func parseVersion(args []string) (bool, error) {
	var jsonOut bool
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.BoolVar(&jsonOut, "json", false, "output stable JSON")
	if err := fs.Parse(args); err != nil {
		return false, err
	}
	if fs.NArg() != 0 {
		return false, fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	return jsonOut, nil
}

func parseFacts(args []string) (bool, error) {
	var jsonOut bool
	fs := flag.NewFlagSet("facts", flag.ContinueOnError)
	fs.BoolVar(&jsonOut, "json", false, "output stable JSON")
	if err := fs.Parse(args); err != nil {
		return false, err
	}
	if fs.NArg() != 0 {
		return false, fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	return jsonOut, nil
}

func parsePositiveInt(raw string) (int, bool) {
	n, err := strconv.Atoi(raw)
	return n, err == nil && n > 0
}
