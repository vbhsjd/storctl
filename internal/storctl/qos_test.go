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
