package storctl

import (
	"fmt"
	"io"
)

func Main(args []string, stdout, stderr io.Writer) int {
	r := NewReporter(stdout, stderr)
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp(stdout)
		return 0
	}
	switch args[0] {
	case "install-driver":
		cfg, err := parseInstallDriver(args[1:])
		if err != nil {
			r.Fail("args", err.Error(), "run: storctl help")
			return 2
		}
		if err := InstallDriver(cfg, r, NewOSRunner(cfg.Proxy, cfg.NoProxy)); err != nil {
			return 1
		}
		return 0
	case "plan":
		cfg, err := parsePlan(args[1:])
		if err != nil {
			r.Fail("args", err.Error(), "run: storctl help")
			return 2
		}
		if err := Plan(cfg, r); err != nil {
			return 1
		}
		return 0
	case "apply":
		cfg, err := parseApply(args[1:])
		if err != nil {
			r.Fail("args", err.Error(), "run: storctl help")
			return 2
		}
		if err := Apply(cfg, r, NewOSRunner(cfg.Proxy, cfg.NoProxy)); err != nil {
			return 1
		}
		return 0
	case "check":
		cfg, err := parseCheck(args[1:])
		if err != nil {
			r.Fail("args", err.Error(), "run: storctl help")
			return 2
		}
		if err := Check(cfg, r, NewOSRunner("", "")); err != nil {
			return 1
		}
		return 0
	default:
		r.Fail("command", fmt.Sprintf("unknown command %q", args[0]), "run: storctl help")
		return 2
	}
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `storctl - join a lab host to NFS-RDMA storage

usage:
  storctl install-driver --nic-type cx7|1823 --artifact-dir DIR
  storctl plan --profile NAME --nic NIC [flags]
  storctl apply --nic NIC --vlan-id ID --data-ip CIDR --gateway IP --mount SERVER:/EXPORT:/MOUNT[:OPTS] [flags]
  storctl apply --profile NAME --nic NIC [flags]
  storctl check
  storctl help

common flags:
  --profile NAME                 load profile from storctl-profiles.json
  --profile-file PATH            override profile file path
  --mgmt-ip IP                   management IP for profile data-ip derivation
  --nic-type auto|cx7|1823      default: auto
  --route-table ID              default: 5000
  --mtu MTU                     default: 5500
  --artifact-dir DIR            default: /root/storage_pkgs
  --proxy URL                   proxy for package commands only
  --no-proxy LIST               no_proxy for package commands only
  --upgrade-firmware            install-driver only; firmware upgrade is off unless this is set
  --allow-repo                  install-driver only; permit artifacts that need a configured dnf repo
  --allow-tcp-fallback          explicitly mount TCP NFS when RDMA is unavailable
  --mount SPEC                  repeatable; default opts are NFS-RDMA

example:
  storctl install-driver --nic-type cx7 --artifact-dir /root/storage_pkgs

  storctl apply --nic enp189s0f0np0 --nic-type auto --vlan-id 3001 \
    --data-ip 172.27.1.123/18 --gateway 172.27.0.1 --route-table 5000 \
    --artifact-dir /root/storage_pkgs \
    --mount 172.27.0.50:/export/a:/mnt/a \
    --mount 172.27.0.51:/export/b:/mnt/b

  storctl plan --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113
`)
}
