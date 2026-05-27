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

func ensureDriverReady(cfg Config, nicType string, r *Reporter, runner Runner) error {
	switch nicType {
	case "cx7":
		return ensureCX7DriverReady(r, runner)
	case "1823":
		return ensure1823DriverReady(r, runner)
	default:
		return fmt.Errorf("unsupported nic type %s", nicType)
	}
}

func ensureCX7DriverReady(r *Reporter, runner Runner) error {
	ready := runner.Exists("ibdev2netdev") && runner.Exists("mlnx_qos") && runner.Exists("cma_roce_tos")
	if ready {
		r.OK("driver mlx5 tools ready")
		return nil
	}
	return fmt.Errorf("mlx5 tools not ready: need ibdev2netdev, mlnx_qos, and cma_roce_tos")
}

func ensure1823DriverReady(r *Reporter, runner Runner) error {
	if !runner.Exists("hinicadm3") {
		return fmt.Errorf("hinicadm3 not found")
	}
	if !runner.Exists("rdma") {
		return fmt.Errorf("rdma command not found")
	}
	out, err := runner.Run("rdma", "link")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Errorf("rdma link is empty; 1823 RDMA driver is not ready")
	}
	r.OK("driver hinic tools ready")
	return nil
}

type InstallDriverConfig struct {
	NICType         string
	ArtifactDir     string
	Proxy           string
	NoProxy         string
	UpgradeFirmware bool
	AllowRepo       bool
}

func InstallDriver(cfg InstallDriverConfig, r *Reporter, runner Runner) error {
	if err := requireRoot(); err != nil {
		r.Fail("permission", err.Error(), "run storctl as root")
		return err
	}
	artifact, err := selectArtifact(cfg.ArtifactDir, cfg.NICType)
	if err != nil {
		r.Fail("artifact", err.Error(), "prepare "+cfg.ArtifactDir+"/storctl-artifacts.json and matching driver package")
		return err
	}
	r.OK("artifact %s", artifact.File)
	if artifact.RequiresRepo && !cfg.AllowRepo {
		err := fmt.Errorf("%s requires a configured dnf repo; rerun with --allow-repo only when repo access is available", artifact.File)
		r.Fail("artifact repo", err.Error(), "prefer a true offline tgz/rpm bundle for lab use")
		return err
	}
	pkg := filepath.Join(cfg.ArtifactDir, artifact.File)
	if artifact.SHA256 != "" {
		if err := verifySHA256(pkg, artifact.SHA256); err != nil {
			r.Fail("artifact sha256", err.Error(), "replace the artifact or update storctl-artifacts.json")
			return err
		}
		r.OK("artifact sha256 verified")
	}
	var rebootRequired bool
	switch cfg.NICType {
	case "cx7":
		rebootRequired, err = installCX7Artifact(pkg, artifact, r, runner)
	case "1823":
		rebootRequired, err = install1823Artifact(pkg, cfg.UpgradeFirmware, r, runner)
	default:
		err = fmt.Errorf("--nic-type must be cx7 or 1823")
	}
	if err != nil {
		r.Fail("driver install", err.Error(), "check artifact package and OS/driver matrix")
		return err
	}
	if rebootRequired {
		r.Warn("reboot recommended: driver updated")
	}
	return nil
}

func installCX7Artifact(pkg string, artifact Artifact, r *Reporter, runner Runner) (bool, error) {
	if strings.HasSuffix(pkg, ".rpm") || strings.Contains(filepath.Base(pkg), "doca-host") {
		if !artifact.RequiresRepo {
			return false, fmt.Errorf("rpm repo installer artifacts must set requires_repo=true")
		}
		return installDOCAHost(pkg, r, runner)
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
	work := hostPath("/tmp/storctl-mlnx")
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
	if runner.Exists("dracut") {
		if _, err := runner.Run("dracut", "-f"); err != nil {
			return true, err
		}
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

func install1823Artifact(pkg string, upgradeFirmware bool, r *Reporter, runner Runner) (bool, error) {
	work := hostPath("/tmp/storctl-1823")
	if _, err := runner.Run("rm", "-rf", work); err != nil {
		return false, err
	}
	if _, err := runner.Run("mkdir", "-p", work); err != nil {
		return false, err
	}
	if _, err := runner.Run("tar", "xf", pkg, "-C", work); err != nil {
		return false, err
	}
	if upgradeFirmware {
		if _, err := runner.Sh(fmt.Sprintf("install_sh=$(find %s -maxdepth 3 -type f -name install.sh -print -quit); [ -n \"$install_sh\" ] || { echo 'install.sh not found'; exit 1; }; cd \"$(dirname \"$install_sh\")\" && bash install.sh roce", work)); err != nil {
			return false, err
		}
		r.OK("firmware 1823 upgraded")
	} else {
		cmd := fmt.Sprintf("install_sh=$(find %s -maxdepth 3 -type f -name install.sh -print -quit); [ -n \"$install_sh\" ] || { echo 'install.sh not found'; exit 1; }; dir=$(dirname \"$install_sh\"); cd \"$dir\" && if ! rpm -qa | grep -q rdma; then echo 'rdma-core not installed'; exit 1; fi && tool_rpm=$(ls tool/hinicadm3-*.rpm 2>/dev/null | head -n1 || true) && if [ -n \"$tool_rpm\" ]; then rpm -Uvh --force nic/*hisdk3*.rpm nic/*hinic3*.rpm roce/*hiroce3*.rpm \"$tool_rpm\"; else rpm -Uvh --force nic/*hisdk3*.rpm nic/*hinic3*.rpm roce/*hiroce3*.rpm && install -m 0755 tool/hinicadm3 /usr/bin/hinicadm3; fi && if command -v dracut >/dev/null 2>&1; then dracut -f; fi", work)
		if _, err := runner.Sh(cmd); err != nil {
			return false, err
		}
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
