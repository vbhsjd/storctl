package storctl

import (
	"strings"
	"testing"
)

func TestInstall1823ArtifactUsesSDKInstallScript(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"rm -rf /tmp/storctl-1823":   "",
			"mkdir -p /tmp/storctl-1823": "",
			"tar xf /root/storage_pkgs/SDK_LINUX-test.tar.gz -C /tmp/storctl-1823": "",
			"/bin/sh -c install_sh=$(find /tmp/storctl-1823 -maxdepth 3 -type f -name install.sh -print -quit); [ -n \"$install_sh\" ] || { echo 'install.sh not found'; exit 1; }; cd \"$(dirname \"$install_sh\")\" && bash install.sh roce": "",
		},
	}
	var out, stderr strings.Builder
	reboot, err := install1823Artifact("/root/storage_pkgs/SDK_LINUX-test.tar.gz", true, NewReporter(&out, &stderr), runner)
	if err != nil {
		t.Fatal(err)
	}
	if !reboot {
		t.Fatal("expected reboot required after 1823 SDK install")
	}
	if !strings.Contains(out.String(), "OK driver hinic installed") {
		t.Fatalf("missing install output: %s", out.String())
	}
	found := false
	for _, call := range runner.calls {
		if strings.Contains(call, "bash install.sh roce") {
			found = true
		}
	}
	if !found {
		t.Fatalf("install.sh roce was not called: %#v", runner.calls)
	}
}

func TestInstall1823ArtifactSkipsFirmwareByDefault(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"rm -rf /tmp/storctl-1823":   "",
			"mkdir -p /tmp/storctl-1823": "",
			"tar xf /root/storage_pkgs/SDK_LINUX-test.tar.gz -C /tmp/storctl-1823": "",
			"/bin/sh -c install_sh=$(find /tmp/storctl-1823 -maxdepth 3 -type f -name install.sh -print -quit); [ -n \"$install_sh\" ] || { echo 'install.sh not found'; exit 1; }; dir=$(dirname \"$install_sh\"); cd \"$dir\" && if ! rpm -qa | grep -q rdma; then echo 'rdma-core not installed'; exit 1; fi && tool_rpm=$(ls tool/hinicadm3-*.rpm 2>/dev/null | head -n1 || true) && if [ -n \"$tool_rpm\" ]; then rpm -Uvh --force nic/*hisdk3*.rpm nic/*hinic3*.rpm roce/*hiroce3*.rpm \"$tool_rpm\"; else rpm -Uvh --force nic/*hisdk3*.rpm nic/*hinic3*.rpm roce/*hiroce3*.rpm && install -m 0755 tool/hinicadm3 /usr/bin/hinicadm3; fi && if command -v dracut >/dev/null 2>&1; then dracut -f; fi": "",
		},
	}
	var out, stderr strings.Builder
	reboot, err := install1823Artifact("/root/storage_pkgs/SDK_LINUX-test.tar.gz", false, NewReporter(&out, &stderr), runner)
	if err != nil {
		t.Fatal(err)
	}
	if !reboot {
		t.Fatal("expected reboot required after 1823 SDK install")
	}
	for _, call := range runner.calls {
		if strings.Contains(call, "bash install.sh roce") {
			t.Fatalf("default install should not run firmware-capable install.sh: %#v", runner.calls)
		}
	}
	if !strings.Contains(out.String(), "OK driver hinic installed") {
		t.Fatalf("missing install output: %s", out.String())
	}
}
