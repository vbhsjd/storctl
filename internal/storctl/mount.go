package storctl

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type MountResult struct {
	Degraded       bool
	DegradedReason string
}

func configureMounts(cfg Config, systemd bool, r *Reporter, runner Runner) (MountResult, error) {
	if err := requireRDMAClient(runner); err != nil {
		if !cfg.AllowTCPFallback {
			return MountResult{}, err
		}
		reason := err.Error()
		r.Warn("rdma unavailable, using explicit tcp fallback: %s", reason)
		if err := configureTCPFallbackMounts(cfg, systemd, r, runner, reason); err != nil {
			return MountResult{}, err
		}
		return MountResult{Degraded: true, DegradedReason: reason}, nil
	}
	result := MountResult{}
	for _, m := range cfg.Mounts {
		if err := os.MkdirAll(m.MountPoint, 0755); err != nil {
			return MountResult{}, err
		}
		if systemd {
			if err := persistSystemdMount(m, r, runner); err != nil {
				return MountResult{}, err
			}
		} else {
			if err := persistFstabMount(m, r); err != nil {
				return MountResult{}, err
			}
		}
		if err := mountNow(m, systemd, r, runner); err != nil {
			if !cfg.AllowTCPFallback {
				return MountResult{}, err
			}
			reason := err.Error()
			r.Warn("rdma mount failed, using explicit tcp fallback for %s", m.MountPoint)
			if err := configureTCPFallbackMount(m, systemd, r, runner); err != nil {
				return MountResult{}, err
			}
			result.Degraded = true
			result.DegradedReason = appendDegradedReason(result.DegradedReason, reason)
			continue
		}
		if err := verifyRDMAMount(m, runner); err != nil {
			if !cfg.AllowTCPFallback {
				return MountResult{}, err
			}
			reason := err.Error()
			r.Warn("rdma verify failed, using explicit tcp fallback for %s", m.MountPoint)
			if err := configureTCPFallbackMount(m, systemd, r, runner); err != nil {
				return MountResult{}, err
			}
			result.Degraded = true
			result.DegradedReason = appendDegradedReason(result.DegradedReason, reason)
			continue
		}
		r.OK("mount %s proto=rdma", m.MountPoint)
	}
	return result, nil
}

func configureTCPFallbackMounts(cfg Config, systemd bool, r *Reporter, runner Runner, reason string) error {
	for _, m := range cfg.Mounts {
		if err := os.MkdirAll(m.MountPoint, 0755); err != nil {
			return err
		}
		if err := configureTCPFallbackMount(m, systemd, r, runner); err != nil {
			return err
		}
	}
	return nil
}

func configureTCPFallbackMount(m MountSpec, systemd bool, r *Reporter, runner Runner) error {
	tcpMount := m
	tcpMount.Options = tcpFallbackOptions(m.Options)
	if systemd {
		if err := persistSystemdMount(tcpMount, r, runner); err != nil {
			return err
		}
	} else {
		if err := persistFstabMount(tcpMount, r); err != nil {
			return err
		}
	}
	if isMountPoint(tcpMount.MountPoint, runner) {
		if ok, _ := mountIsTCP(tcpMount, runner); !ok {
			if err := unmount(tcpMount.MountPoint, runner); err != nil {
				return err
			}
		}
	}
	if !isMountPoint(tcpMount.MountPoint, runner) {
		if _, err := runner.Run("mount", "-t", "nfs", "-o", tcpMount.Options, tcpMount.Server+":"+tcpMount.Export, tcpMount.MountPoint); err != nil {
			return err
		}
	}
	r.Warn("mount %s proto=tcp degraded", tcpMount.MountPoint)
	return nil
}

func persistSystemdMount(m MountSpec, r *Reporter, runner Runner) error {
	unitName := systemdMountUnitName(m.MountPoint)
	unitPath := filepath.Join("/etc/systemd/system", unitName)
	autoPath := strings.TrimSuffix(unitPath, ".mount") + ".automount"
	unit := fmt.Sprintf(`[Unit]
Description=storctl NFS-RDMA mount %s
After=network-online.target storctl-qos.service
Wants=network-online.target

[Mount]
What=%s:%s
Where=%s
Type=nfs
Options=%s
TimeoutSec=60

[Install]
WantedBy=multi-user.target
`, m.Server, m.Server, m.Export, m.MountPoint, m.Options)
	automount := fmt.Sprintf(`[Unit]
Description=storctl automount %s
After=network-online.target

[Automount]
Where=%s
TimeoutIdleSec=300

[Install]
WantedBy=multi-user.target
`, m.MountPoint, m.MountPoint)
	if _, err := writeFileChanged(unitPath, []byte(unit), 0644); err != nil {
		return err
	}
	if _, err := writeFileChanged(autoPath, []byte(automount), 0644); err != nil {
		return err
	}
	if _, err := runner.Run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if _, err := runner.Run("systemctl", "enable", filepath.Base(autoPath)); err != nil {
		return err
	}
	r.OK("mount persistence %s", filepath.Base(autoPath))
	return nil
}

func persistFstabMount(m MountSpec, r *Reporter) error {
	line := fmt.Sprintf("%s:%s %s nfs %s 0 0", m.Server, m.Export, m.MountPoint, m.Options)
	path := "/etc/fstab"
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := []string{}
	found := false
	for _, old := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(old) == "" {
			continue
		}
		fields := strings.Fields(old)
		if len(fields) >= 2 && fields[1] == m.MountPoint {
			if old == line {
				found = true
				lines = append(lines, old)
			}
			continue
		}
		lines = append(lines, old)
	}
	if !found {
		lines = append(lines, line)
	}
	out := strings.Join(lines, "\n") + "\n"
	changed, err := writeFileChanged(path, []byte(out), 0644)
	if err != nil {
		return err
	}
	if changed {
		r.OK("mount persistence fstab %s", m.MountPoint)
	} else {
		r.Skip("mount persistence fstab %s", m.MountPoint)
	}
	return nil
}

func mountNow(m MountSpec, systemd bool, r *Reporter, runner Runner) error {
	if systemd {
		unit := strings.TrimSuffix(systemdMountUnitName(m.MountPoint), ".mount") + ".automount"
		if _, err := runner.Run("systemctl", "start", unit); err != nil {
			r.Warn("automount start failed for %s: %v", m.MountPoint, err)
			r.Warn("trying direct nfs mount for %s", m.MountPoint)
		} else if isMountPoint(m.MountPoint, runner) {
			if ok, _ := mountIsRDMA(m, runner); ok {
				return nil
			}
			r.Warn("mount %s exists but is not rdma; remounting", m.MountPoint)
			if err := unmount(m.MountPoint, runner); err != nil {
				return err
			}
		}
	}
	if isMountPoint(m.MountPoint, runner) {
		if ok, _ := mountIsRDMA(m, runner); ok {
			return nil
		}
		r.Warn("mount %s exists but is not rdma; remounting", m.MountPoint)
		if err := unmount(m.MountPoint, runner); err != nil {
			return err
		}
	}
	_, err := runner.Run("mount", "-t", "nfs", "-o", m.Options, m.Server+":"+m.Export, m.MountPoint)
	return err
}

func verifyRDMAMount(m MountSpec, runner Runner) error {
	details := []string{}
	if ok, detail := mountIsRDMA(m, runner); ok {
		return nil
	} else if detail != "" {
		details = append(details, detail)
	}
	if runner.Exists("nfsstat") {
		out, err := runner.Run("nfsstat", "-m")
		if err == nil && strings.Contains(out, m.MountPoint) && strings.Contains(out, "proto=rdma") {
			return nil
		}
		if strings.TrimSpace(out) != "" {
			details = append(details, "nfsstat: "+strings.TrimSpace(out))
		}
	}
	if len(details) > 0 {
		return fmt.Errorf("nfsstat/findmnt does not show %s as proto=rdma; %s", m.MountPoint, strings.Join(details, " | "))
	}
	return fmt.Errorf("nfsstat/findmnt does not show %s as proto=rdma", m.MountPoint)
}

func mountIsRDMA(m MountSpec, runner Runner) (bool, string) {
	if runner.Exists("findmnt") {
		out, err := runner.Run("findmnt", "-n", "--mountpoint", m.MountPoint, "-o", "FSTYPE,OPTIONS")
		if err == nil && strings.Contains(out, "nfs") && strings.Contains(out, "proto=rdma") {
			return true, ""
		}
		if strings.TrimSpace(out) != "" {
			return false, "findmnt: " + strings.TrimSpace(out)
		}
	}
	return false, ""
}

func mountIsTCP(m MountSpec, runner Runner) (bool, string) {
	if runner.Exists("findmnt") {
		out, err := runner.Run("findmnt", "-n", "--mountpoint", m.MountPoint, "-o", "FSTYPE,OPTIONS")
		if err == nil && strings.Contains(out, "nfs") && strings.Contains(out, "proto=tcp") {
			return true, ""
		}
		if strings.TrimSpace(out) != "" {
			return false, "findmnt: " + strings.TrimSpace(out)
		}
	}
	return false, ""
}

func tcpFallbackOptions(_ string) string {
	return defaultTCPOptions
}

func appendDegradedReason(existing, next string) string {
	if existing == "" {
		return next
	}
	if strings.Contains(existing, next) {
		return existing
	}
	return existing + "; " + next
}

func isMountPoint(path string, runner Runner) bool {
	if runner.Exists("findmnt") {
		_, err := runner.Run("findmnt", "-n", "--mountpoint", path)
		return err == nil
	}
	_, err := runner.Run("mountpoint", "-q", path)
	return err == nil
}

func unmount(path string, runner Runner) error {
	_, err := runner.Run("umount", path)
	return err
}

func requireRDMAClient(runner Runner) error {
	if runner.Exists("modprobe") {
		_, _ = runner.Run("modprobe", "xprtrdma")
	}
	if !runner.Exists("rdma") {
		return fmt.Errorf("rdma command not found")
	}
	out, err := runner.Run("rdma", "link")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Errorf("rdma link is empty; no RDMA device is available for NFS-RDMA")
	}
	return nil
}

func systemdMountUnitName(mountPoint string) string {
	clean := filepath.Clean(mountPoint)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." {
		return "-.mount"
	}
	re := regexp.MustCompile(`[^A-Za-z0-9_.-]+`)
	clean = re.ReplaceAllString(clean, "-")
	clean = strings.ReplaceAll(clean, "/", "-")
	return clean + ".mount"
}
