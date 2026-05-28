package storctl

import (
	"fmt"
	"strings"
)

func Apply(cfg Config, r *Reporter, runner Runner) error {
	if err := requireRoot(); err != nil {
		r.Fail("permission", err.Error(), "run storctl as root")
		return err
	}
	osID, osVersion, err := detectOS()
	if err != nil {
		r.Warn("os unknown: %v", err)
	} else if isOpenEuler(osID) && supportedOpenEuler(osVersion) {
		r.OK("os openEuler %s", osVersion)
	} else if isOpenEuler(osID) {
		r.Warn("os openEuler %s not in tested 22-24 range", osVersion)
	} else {
		r.Warn("os %s %s not tested", osID, osVersion)
	}

	if err := requireCommand(runner, "nmcli"); err != nil {
		r.Fail("networkmanager", err.Error(), "install/enable NetworkManager and retry")
		return err
	}
	if err := ensureNetworkManagerStarted(runner, r); err != nil {
		r.Fail("networkmanager", err.Error(), "start NetworkManager and retry")
		return err
	}
	if err := ensureNICExists(cfg.NIC); err != nil {
		r.Fail("nic "+cfg.NIC, err.Error(), "run: nmcli dev status")
		return err
	}
	r.OK("nic %s found", cfg.NIC)
	if err := guardManagementNIC(cfg, runner); err != nil {
		r.Fail("nic "+cfg.NIC, err.Error(), "pass the 200G storage NIC, not the SSH management NIC")
		return err
	}

	nicType, err := resolveNICType(cfg, runner)
	if err != nil {
		r.Fail("nic type", err.Error(), "pass --nic-type cx7 or --nic-type 1823")
		return err
	}
	r.OK("nic type %s", nicType)

	driverReady, err := ensureApplyDriverReady(cfg, nicType, r, runner)
	if err != nil {
		r.Fail("driver "+nicType, err.Error(), "run: storctl install-driver --nic-type "+nicType+" --artifact-dir "+cfg.ArtifactDir)
		return err
	}

	if err := configureNetwork(cfg, r, runner); err != nil {
		r.Fail("vlan "+cfg.vlanName(), err.Error(), "run: nmcli con show && ip addr")
		return err
	}
	if !driverReady && cfg.QoSMode == "apply" {
		r.Skip("qos driver not ready")
	} else if err := configureQoS(cfg, nicType, r, runner); err != nil {
		r.Fail("qos "+nicType, err.Error(), "check switch PFC/DSCP and NIC tools")
		return err
	}

	systemd := hasSystemd(runner)
	mountResult, err := configureMounts(cfg, false, r, runner)
	if err != nil {
		r.Fail("rdma mount", err.Error(), "check server port 20049 and run: rdma link")
		return err
	}

	if err := saveState(cfg, nicType, false, systemd, mountResult.Degraded, mountResult.DegradedReason); err != nil {
		r.Fail("state", err.Error(), "check /var/lib/storctl permissions")
		return err
	}
	r.OK("state %s/state.json", cfg.StateDir)
	if mountResult.Degraded {
		r.Warn("degraded tcp-fallback: %s", mountResult.DegradedReason)
	}
	return nil
}

func ensureNetworkManagerStarted(runner Runner, r *Reporter) error {
	if networkManagerRunning(runner) {
		r.OK("networkmanager running")
		return nil
	}
	r.Warn("networkmanager not running, trying to start")
	var lastErr error
	if runner.Exists("systemctl") {
		if _, err := runner.Run("systemctl", "start", "NetworkManager"); err != nil {
			lastErr = err
		}
		if networkManagerRunning(runner) {
			r.OK("networkmanager started")
			return nil
		}
	}
	if runner.Exists("service") {
		if _, err := runner.Run("service", "NetworkManager", "start"); err != nil {
			lastErr = err
		}
		if networkManagerRunning(runner) {
			r.OK("networkmanager started")
			return nil
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("NetworkManager is not running")
}

func networkManagerRunning(runner Runner) bool {
	out, err := runner.Run("nmcli", "-t", "-f", "RUNNING", "general")
	return err == nil && strings.Contains(strings.ToLower(out), "running")
}

func requireCommand(r Runner, name string) error {
	if !r.Exists(name) {
		return fmt.Errorf("%s not found", name)
	}
	return nil
}

func ensureApplyDriverReady(cfg Config, nicType string, r *Reporter, runner Runner) (bool, error) {
	if err := ensureDriverReady(cfg, nicType, r, runner); err != nil {
		if !cfg.AllowTCPFallback {
			return false, err
		}
		r.Warn("driver %s not ready, using explicit tcp fallback: %v", nicType, err)
		return false, nil
	}
	return true, nil
}
