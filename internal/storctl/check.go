package storctl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type CheckReport struct {
	SchemaVersion int          `json:"schema_version"`
	State         *State       `json:"state,omitempty"`
	Checks        []CheckItem  `json:"checks"`
	Summary       CheckSummary `json:"summary"`
}

type CheckItem struct {
	Name    string            `json:"name"`
	Status  string            `json:"status"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

type CheckSummary struct {
	OK   int `json:"ok"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
}

func Check(cfg Config, r *Reporter, runner Runner) error {
	report := collectCheckReport(cfg, runner)
	if cfg.CheckJSON {
		enc := json.NewEncoder(r.out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	emitCheckReport(report, r)
	return nil
}

func collectCheckReport(cfg Config, runner Runner) CheckReport {
	report := CheckReport{SchemaVersion: 1}
	state, err := loadState(cfg.StateDir)
	if err != nil {
		report.add("state", "warn", "state_not_found", fmt.Sprintf("state not found: %s/state.json", cfg.StateDir))
	} else {
		report.State = &state
		report.addDetails("state", "ok", "state_loaded", fmt.Sprintf("%s %s", state.NIC, state.VLAN), map[string]string{
			"schema_version": strconv.Itoa(state.SchemaVersion),
			"nic":            state.NIC,
			"nic_type":       state.NICType,
			"vlan":           state.VLAN,
			"qos_mode":       state.QoSMode,
		})
		if state.Degraded {
			report.add("degraded", "warn", "tcp_fallback_degraded", state.DegradedReason)
		}
	}

	osID, osVersion, err := detectOS()
	if err != nil {
		report.add("os", "warn", "os_unknown", err.Error())
	} else if isOpenEuler(osID) && supportedOpenEuler(osVersion) {
		report.addDetails("os", "ok", "os_supported", fmt.Sprintf("openEuler %s", osVersion), map[string]string{"id": osID, "version": osVersion, "arch": artifactArch()})
	} else {
		report.addDetails("os", "warn", "os_not_tested", fmt.Sprintf("%s %s not tested", osID, osVersion), map[string]string{"id": osID, "version": osVersion, "arch": artifactArch()})
	}

	if runner.Exists("nmcli") {
		report.add("networkmanager", "ok", "nmcli_found", "nmcli found")
	} else {
		report.add("networkmanager", "warn", "nmcli_missing", "nmcli missing")
	}
	if runner.Exists("rdma") {
		if out, err := runner.Run("rdma", "link"); err == nil && strings.TrimSpace(out) != "" {
			report.addDetails("rdma", "ok", "rdma_link_ready", "rdma link ready", map[string]string{"links": compactLines(out)})
		} else {
			report.add("rdma", "warn", "rdma_link_empty", "rdma link empty")
		}
	} else {
		report.add("rdma", "warn", "rdma_command_missing", "rdma command missing")
	}
	if runner.Exists("ibdev2netdev") {
		if out, err := runner.Run("ibdev2netdev"); err == nil && strings.TrimSpace(out) != "" {
			report.add("ibdev2netdev", "ok", "ibdev2netdev_ready", "ibdev2netdev ready")
		} else {
			report.add("ibdev2netdev", "warn", "ibdev2netdev_empty", "ibdev2netdev empty")
		}
	} else {
		report.add("ibdev2netdev", "warn", "ibdev2netdev_missing", "ibdev2netdev missing")
	}
	if runner.Exists("nfsstat") {
		if out, err := runner.Run("nfsstat", "-m"); err == nil {
			if strings.Contains(out, "proto=rdma") {
				report.add("nfs", "ok", "nfs_rdma_found", "proto=rdma")
			} else {
				report.add("nfs", "warn", "nfs_rdma_not_found", "rdma mount not found")
			}
		}
	} else {
		report.add("nfs", "warn", "nfsstat_missing", "nfsstat missing")
	}

	if state.NIC != "" {
		checkDriverState(state, &report, runner)
		checkQoSState(state, &report)
		checkArtifactState(state, &report)
		checkLink(state.NIC, &report, runner)
		checkLink(state.VLAN, &report, runner)
		checkMountState(state, &report, runner)
		if state.RebootRequired {
			report.add("reboot", "warn", "reboot_recommended", "previous driver update")
		}
	}
	return report
}

func (r *CheckReport) add(name, status, code, message string) {
	r.addDetails(name, status, code, message, nil)
}

func (r *CheckReport) addDetails(name, status, code, message string, details map[string]string) {
	r.Checks = append(r.Checks, CheckItem{Name: name, Status: status, Code: code, Message: message, Details: details})
	switch status {
	case "ok":
		r.Summary.OK++
	case "fail":
		r.Summary.Fail++
	default:
		r.Summary.Warn++
	}
}

func compactLines(raw string) string {
	lines := []string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, " | ")
}

func emitCheckReport(report CheckReport, r *Reporter) {
	for _, item := range report.Checks {
		switch item.Status {
		case "ok":
			r.OK("%s %s", item.Name, item.Message)
		case "fail":
			r.Fail(item.Name, item.Message, "")
		default:
			r.Warn("%s %s", item.Name, item.Message)
		}
	}
}

func checkDriverState(state State, report *CheckReport, runner Runner) {
	switch state.NICType {
	case "cx7":
		missing := []string{}
		for _, cmd := range []string{"ibdev2netdev", "mlnx_qos", "cma_roce_tos"} {
			if !runner.Exists(cmd) {
				missing = append(missing, cmd)
			}
		}
		if len(missing) > 0 {
			report.add("driver", "warn", "driver_not_ready", "missing "+strings.Join(missing, ","))
			return
		}
		report.add("driver", "ok", "driver_ready", "cx7 tools ready")
	case "1823":
		if !runner.Exists("hinicadm3") {
			report.add("driver", "warn", "driver_not_ready", "hinicadm3 missing")
			return
		}
		report.add("driver", "ok", "driver_ready", "hinicadm3 ready")
	}
}

func checkQoSState(state State, report *CheckReport) {
	if state.QoSMode != "apply" {
		report.addDetails("qos", "ok", "qos_disabled", "qos disabled", map[string]string{"mode": state.QoSMode})
		return
	}
	if _, err := os.Stat("/usr/local/sbin/storctl-qos.sh"); err != nil {
		report.addDetails("qos", "warn", "qos_persistence_missing", "storctl-qos.sh missing", map[string]string{"mode": state.QoSMode})
		return
	}
	report.addDetails("qos", "ok", "qos_persistence_found", "storctl-qos.sh found", map[string]string{"mode": state.QoSMode})
}

func checkArtifactState(state State, report *CheckReport) {
	if state.ArtifactDir == "" {
		return
	}
	if _, err := readArtifactManifest(filepath.Join(state.ArtifactDir, artifactManifestName)); err != nil {
		report.addDetails("artifacts", "warn", "artifact_manifest_missing", err.Error(), map[string]string{"artifact_dir": state.ArtifactDir})
		return
	}
	report.addDetails("artifacts", "ok", "artifact_manifest_found", artifactManifestName+" found", map[string]string{"artifact_dir": state.ArtifactDir})
}

func checkLink(name string, report *CheckReport, runner Runner) {
	if name == "" {
		return
	}
	if _, err := os.Stat("/sys/class/net/" + name); err != nil {
		report.add("link:"+name, "warn", "link_missing", "missing")
		return
	}
	out, err := runner.Run("ip", "-s", "link", "show", name)
	if err != nil {
		report.add("link:"+name, "warn", "link_unreadable", "unreadable")
		return
	}
	if strings.Contains(out, "DOWN") {
		report.add("link:"+name, "warn", "link_down", "down")
		return
	}
	report.add("link:"+name, "ok", "link_up", "up")
}

func checkMountState(state State, report *CheckReport, runner Runner) {
	if !runner.Exists("findmnt") {
		report.add("findmnt", "warn", "findmnt_missing", "findmnt missing")
		return
	}
	for _, m := range state.Mounts {
		out, err := runner.Run("findmnt", "-n", "--mountpoint", m.MountPoint, "-o", "FSTYPE,OPTIONS")
		if err != nil {
			report.add("mount:"+m.MountPoint, "warn", "mount_missing", "missing")
			continue
		}
		if strings.Contains(out, "nfs") && strings.Contains(out, "proto=rdma") {
			report.add("mount:"+m.MountPoint, "ok", "mount_rdma", "proto=rdma")
		} else {
			report.add("mount:"+m.MountPoint, "warn", "mount_not_rdma", strings.TrimSpace(out))
		}
	}
}

func failf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
