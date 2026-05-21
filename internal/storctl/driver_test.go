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
	reboot, err := install1823Artifact("/root/storage_pkgs/SDK_LINUX-test.tar.gz", false, NewReporter(&out, &stderr), runner)
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
