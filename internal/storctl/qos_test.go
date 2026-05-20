package storctl

import (
	"strings"
	"testing"
)

func TestCX7QoSCommandsNoTrailingTSACComma(t *testing.T) {
	cmds := cx7QoSCommands("enp194s0f1np1", "mlx5_0")
	joined := strings.Join(cmds, "\n")
	if strings.Contains(joined, "ets, --tcbw") {
		t.Fatalf("tsa command has trailing comma: %s", joined)
	}
	if !strings.Contains(joined, "--tsa ets,ets,ets,ets,ets,ets,ets,ets --tcbw") {
		t.Fatalf("tsa command missing expected form: %s", joined)
	}
}

func TestHinicQoSCommandsTolerateMissingECN(t *testing.T) {
	cmds := hinicQoSCommands("enp23s0f1")
	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, "if [ -e '/sys/class/net/enp23s0f1/ecn/cc_algo' ]; then echo dcqcn") {
		t.Fatalf("ecn command should be conditional: %s", joined)
	}
}

func TestEnsureDriverReadyDoesNotInstall(t *testing.T) {
	runner := &fakeRunner{exists: map[string]bool{}}
	var out, stderr strings.Builder
	err := ensureDriverReady(Config{ArtifactDir: "/nope"}, "cx7", NewReporter(&out, &stderr), runner)
	if err == nil || !strings.Contains(err.Error(), "mlx5 tools not ready") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsure1823DriverReadyRequiresRDMALink(t *testing.T) {
	runner := &fakeRunner{
		exists: map[string]bool{"hinicadm3": true, "rdma": true},
		outputs: map[string]string{
			"rdma link": "",
		},
	}
	var out, stderr strings.Builder
	err := ensureDriverReady(Config{}, "1823", NewReporter(&out, &stderr), runner)
	if err == nil || !strings.Contains(err.Error(), "rdma link is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCX7RepoInstallerRequiresManifestOptIn(t *testing.T) {
	runner := &fakeRunner{exists: map[string]bool{}}
	var out, stderr strings.Builder
	_, err := installCX7Artifact("/tmp/doca-host.rpm", Artifact{RequiresRepo: false}, NewReporter(&out, &stderr), runner)
	if err == nil || !strings.Contains(err.Error(), "requires_repo=true") {
		t.Fatalf("unexpected error: %v", err)
	}
}
