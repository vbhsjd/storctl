package storctl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveNICType(cfg Config, runner Runner) (string, error) {
	if cfg.NICType != "auto" {
		return cfg.NICType, nil
	}
	if runner.Exists("ibdev2netdev") {
		if out, err := runner.Run("ibdev2netdev"); err == nil && strings.Contains(out, cfg.NIC) {
			return "cx7", nil
		}
	}
	if runner.Exists("lspci") {
		out, _ := runner.Run("lspci")
		low := strings.ToLower(out)
		if strings.Contains(low, "connectx-7") || strings.Contains(low, "mellanox") {
			return "cx7", nil
		}
		if strings.Contains(low, "huawei") && strings.Contains(low, "0222") {
			return "1823", nil
		}
	}
	return "", fmt.Errorf("can not auto-detect NIC type")
}

func ensureDriver(cfg Config, nicType string, r *Reporter, runner Runner) (bool, error) {
	switch nicType {
	case "cx7":
		return ensureCX7Driver(cfg, r, runner)
	case "1823":
		return ensure1823Driver(cfg, r, runner)
	default:
		return false, fmt.Errorf("unsupported nic type %s", nicType)
	}
}

func ensureCX7Driver(cfg Config, r *Reporter, runner Runner) (bool, error) {
	ready := runner.Exists("ibdev2netdev") && runner.Exists("mlnx_qos") && runner.Exists("cma_roce_tos")
	if ready {
		r.OK("driver mlx5 tools ready")
		return false, nil
	}
	if pkg, err := findArtifact(cfg.ArtifactDir, "doca-host*.rpm"); err == nil {
		return installDOCAHost(pkg, r, runner)
	}
	pkg, err := findArtifact(cfg.ArtifactDir, "MLNX_OFED_LINUX-*.tgz", "IB_NIC-*.tgz")
	if err != nil {
		return false, err
	}
	return installMLNXOFED(pkg, r, runner)
}

func installDOCAHost(pkg string, r *Reporter, runner Runner) (bool, error) {
	if !runner.Exists("rpm") {
		return false, fmt.Errorf("rpm not found")
	}
	if !runner.Exists("dnf") {
		return false, fmt.Errorf("dnf not found")
	}
	if _, err := runner.Run("rpm", "-Uvh", pkg); err != nil {
		return false, err
	}
	if _, err := runner.Run("dnf", "clean", "all"); err != nil {
		return false, err
	}
	if _, err := runner.Run("dnf", "-y", "install", "doca-ofed"); err != nil {
		return false, err
	}
	if runner.Exists("dracut") {
		if _, err := runner.Run("dracut", "-f"); err != nil {
			return true, err
		}
	}
	restartOpenIBD(runner)
	r.OK("driver doca-ofed installed")
	return true, nil
}

func installMLNXOFED(pkg string, r *Reporter, runner Runner) (bool, error) {
	work := "/tmp/storctl-mlnx"
	if _, err := runner.Run("rm", "-rf", work); err != nil {
		return false, err
	}
	if _, err := runner.Run("mkdir", "-p", work); err != nil {
		return false, err
	}
	if _, err := runner.Run("tar", "xf", pkg, "-C", work); err != nil {
		return false, err
	}
	cmd := fmt.Sprintf("cd %s/* && chmod +x mlnxofedinstall && ./mlnxofedinstall --with-nfsrdma --without-fw-update --add-kernel-support --skip-repo --force", work)
	if _, err := runner.Sh(cmd); err != nil {
		return false, err
	}
	if _, err := runner.Run("dracut", "-f"); err != nil {
		return true, err
	}
	restartOpenIBD(runner)
	r.OK("driver mlx5 installed")
	return true, nil
}

func restartOpenIBD(runner Runner) {
	if runner.Exists("systemctl") {
		_, _ = runner.Run("systemctl", "restart", "openibd")
		return
	}
	_, _ = runner.Run("/etc/init.d/openibd", "restart")
}

func ensure1823Driver(cfg Config, r *Reporter, runner Runner) (bool, error) {
	if runner.Exists("hinicadm3") {
		r.OK("driver hinic tools ready")
		if cfg.UpgradeFirmware {
			if err := upgrade1823Firmware(runner); err != nil {
				return false, err
			}
			r.OK("firmware 1823 upgraded")
			return true, nil
		}
		return false, nil
	}
	pkg, err := findArtifact(cfg.ArtifactDir, "nic_1823.tar.gz", "hinic*.tar.gz")
	if err != nil {
		return false, err
	}
	work := "/tmp/storctl-1823"
	if _, err := runner.Run("rm", "-rf", work); err != nil {
		return false, err
	}
	if _, err := runner.Run("mkdir", "-p", work); err != nil {
		return false, err
	}
	if _, err := runner.Run("tar", "xf", pkg, "-C", work); err != nil {
		return false, err
	}
	if _, err := runner.Sh(fmt.Sprintf("cd %s/* && rpm -Uvh *.rpm", work)); err != nil {
		return false, err
	}
	if cfg.UpgradeFirmware {
		if err := upgrade1823Firmware(runner); err != nil {
			return true, err
		}
		r.OK("firmware 1823 upgraded")
	}
	r.OK("driver hinic installed")
	return true, nil
}

func upgrade1823Firmware(runner Runner) error {
	if !runner.Exists("hinicadm3") {
		return fmt.Errorf("hinicadm3 not found")
	}
	out, err := runner.Run("hinicadm3", "info")
	if err != nil {
		return err
	}
	lines := strings.Split(out, "\n")
	updated := 0
	for _, line := range lines {
		if !strings.Contains(line, "2X200G") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if !strings.HasPrefix(name, "hinic") {
			continue
		}
		if _, err := runner.Run("hinicadm3", "updatefw", "-i", name, "-f", "Hinic3_flash.bin", "-x", "0", "-or"); err != nil {
			return err
		}
		updated++
	}
	if updated == 0 {
		return fmt.Errorf("no 2X200G hinic device found")
	}
	return nil
}

func findArtifact(dir string, patterns ...string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("artifact dir is empty")
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("artifact dir not found: %s", dir)
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		if len(matches) > 0 {
			return matches[0], nil
		}
	}
	return "", fmt.Errorf("artifact not found in %s", dir)
}
