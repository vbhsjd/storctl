package storctl

import (
	"fmt"
	"net"
	"strings"
)

func guardManagementNIC(cfg Config, runner Runner) error {
	mgmtIP := strings.TrimSpace(cfg.MgmtIP)
	if mgmtIP == "" {
		return nil
	}
	parsed := net.ParseIP(mgmtIP)
	if parsed == nil || parsed.To4() == nil {
		return nil
	}
	out, err := runner.Run("ip", "-o", "-4", "addr", "show", "dev", cfg.NIC)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i, field := range fields {
			if field != "inet" || i+1 >= len(fields) {
				continue
			}
			ip := strings.SplitN(fields[i+1], "/", 2)[0]
			if ip == parsed.String() {
				return fmt.Errorf("--nic owns --mgmt-ip %s; this looks like the SSH management interface", parsed.String())
			}
		}
	}
	return nil
}
