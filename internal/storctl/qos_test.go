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
