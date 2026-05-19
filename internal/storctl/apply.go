package storctl

import (
	"fmt"
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
	if err := ensureNICExists(cfg.NIC); err != nil {
		r.Fail("nic "+cfg.NIC, err.Error(), "run: nmcli dev status")
		return err
	}
	r.OK("nic %s found", cfg.NIC)

	nicType, err := resolveNICType(cfg, runner)
	if err != nil {
		r.Fail("nic type", err.Error(), "pass --nic-type cx7 or --nic-type 1823")
		return err
	}
	r.OK("nic type %s", nicType)

	rebootRequired, err := ensureDriver(cfg, nicType, r, runner)
	if err != nil {
		r.Fail("driver "+nicType, err.Error(), "check --artifact-dir and driver package for this OS")
		return err
	}

	if err := configureNetwork(cfg, r, runner); err != nil {
		r.Fail("vlan "+cfg.vlanName(), err.Error(), "run: nmcli con show && ip addr")
		return err
	}
	if err := configureQoS(cfg, nicType, r, runner); err != nil {
		r.Fail("qos "+nicType, err.Error(), "check switch PFC/DSCP and NIC tools")
		return err
	}

	systemd := hasSystemd(runner)
	if err := configureMounts(cfg, systemd, r, runner); err != nil {
		r.Fail("rdma mount", err.Error(), "check server port 20049 and run: rdma link")
		return err
	}

	if err := saveState(cfg, nicType, rebootRequired, systemd); err != nil {
		r.Fail("state", err.Error(), "check /var/lib/storctl permissions")
		return err
	}
	r.OK("state %s/state.json", cfg.StateDir)
	if rebootRequired {
		r.Warn("reboot recommended: driver updated")
	}
	return nil
}

func requireCommand(r Runner, name string) error {
	if !r.Exists(name) {
		return fmt.Errorf("%s not found", name)
	}
	return nil
}
