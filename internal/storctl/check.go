package storctl

import (
	"fmt"
	"os"
	"strings"
)

func Check(cfg Config, r *Reporter, runner Runner) error {
	state, err := loadState(cfg.StateDir)
	if err != nil {
		r.Warn("state not found: %s/state.json", cfg.StateDir)
	} else {
		r.OK("state %s %s", state.NIC, state.VLAN)
		if state.Degraded {
			r.Warn("degraded tcp-fallback: %s", state.DegradedReason)
		}
	}

	osID, osVersion, err := detectOS()
	if err != nil {
		r.Warn("os unknown: %v", err)
	} else if isOpenEuler(osID) && supportedOpenEuler(osVersion) {
		r.OK("os openEuler %s", osVersion)
	} else {
		r.Warn("os %s %s not tested", osID, osVersion)
	}

	if runner.Exists("nmcli") {
		r.OK("networkmanager nmcli found")
	} else {
		r.Warn("networkmanager nmcli missing")
	}
	if runner.Exists("rdma") {
		if out, err := runner.Run("rdma", "link"); err == nil && strings.TrimSpace(out) != "" {
			r.OK("rdma link ready")
		} else {
			r.Warn("rdma link empty")
		}
	} else {
		r.Warn("rdma command missing")
	}
	if runner.Exists("ibdev2netdev") {
		if out, err := runner.Run("ibdev2netdev"); err == nil && strings.TrimSpace(out) != "" {
			r.OK("ibdev2netdev ready")
		} else {
			r.Warn("ibdev2netdev empty")
		}
	} else {
		r.Warn("ibdev2netdev missing")
	}
	if runner.Exists("nfsstat") {
		if out, err := runner.Run("nfsstat", "-m"); err == nil {
			if strings.Contains(out, "proto=rdma") {
				r.OK("nfs proto=rdma")
			} else {
				r.Warn("nfs rdma mount not found")
			}
		}
	} else {
		r.Warn("nfsstat missing")
	}

	if state.NIC != "" {
		checkLink(state.NIC, r, runner)
		checkLink(state.VLAN, r, runner)
		checkMountState(state, r, runner)
		if state.RebootRequired {
			r.Warn("reboot recommended: previous driver update")
		}
	}
	return nil
}

func checkLink(name string, r *Reporter, runner Runner) {
	if name == "" {
		return
	}
	if _, err := os.Stat("/sys/class/net/" + name); err != nil {
		r.Warn("link %s missing", name)
		return
	}
	out, err := runner.Run("ip", "-s", "link", "show", name)
	if err != nil {
		r.Warn("link %s unreadable", name)
		return
	}
	if strings.Contains(out, "DOWN") {
		r.Warn("link %s down", name)
		return
	}
	r.OK("link %s up", name)
}

func checkMountState(state State, r *Reporter, runner Runner) {
	if !runner.Exists("findmnt") {
		r.Warn("findmnt missing")
		return
	}
	for _, m := range state.Mounts {
		out, err := runner.Run("findmnt", "-n", "--mountpoint", m.MountPoint, "-o", "FSTYPE,OPTIONS")
		if err != nil {
			r.Warn("mount %s missing", m.MountPoint)
			continue
		}
		if strings.Contains(out, "nfs") && strings.Contains(out, "proto=rdma") {
			r.OK("mount %s proto=rdma", m.MountPoint)
		} else {
			r.Warn("mount %s not rdma: %s", m.MountPoint, strings.TrimSpace(out))
		}
	}
}

func failf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
