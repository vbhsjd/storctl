package storctl

import (
	"fmt"
	"strings"
)

func configureQoS(cfg Config, nicType string, r *Reporter, runner Runner) error {
	var commands []string
	switch nicType {
	case "cx7":
		ibdev, err := ibdevForNIC(cfg.NIC, runner)
		if err != nil {
			return err
		}
		commands = cx7QoSCommands(cfg.NIC, ibdev)
	case "1823":
		commands = hinicQoSCommands(cfg.NIC)
	default:
		return fmt.Errorf("unsupported nic type %s", nicType)
	}
	for _, cmd := range commands {
		if _, err := runner.Sh(cmd); err != nil {
			return err
		}
	}
	if err := persistQoS(commands, hasSystemd(runner), r, runner); err != nil {
		return err
	}
	r.OK("qos %s applied", nicType)
	return nil
}

func ibdevForNIC(nic string, runner Runner) (string, error) {
	if !runner.Exists("ibdev2netdev") {
		return "", fmt.Errorf("ibdev2netdev not found")
	}
	out, err := runner.Run("ibdev2netdev")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, nic) {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				return fields[0], nil
			}
		}
	}
	return "", fmt.Errorf("no ibdev maps to %s", nic)
}

func cx7QoSCommands(nic, ibdev string) []string {
	return []string{
		fmt.Sprintf("mlnx_qos -i %s --pfc 0,0,0,0,1,0,0,0 --trust dscp", shellQuote(nic)),
		fmt.Sprintf("cma_roce_tos -d %s -t 128", shellQuote(ibdev)),
		fmt.Sprintf("mlnx_qos -i %s --prio_tc 1,0,0,0,4,0,0,0", shellQuote(nic)),
		fmt.Sprintf("mlnx_qos -i %s --tsa ets,ets,ets,ets,ets,ets,ets,ets --tcbw 10,0,0,0,90,0,0,0", shellQuote(nic)),
	}
}

func hinicQoSCommands(nic string) []string {
	q := shellQuote(nic)
	return []string{
		fmt.Sprintf("echo dcqcn > /sys/class/net/%s/ecn/cc_algo", q),
		fmt.Sprintf("hinicadm3 qos -i %s -t dcb -e 1", q),
		fmt.Sprintf("hinicadm3 qos -i %s -t pfc -e 1 -f 0,0,0,0,1,0,0,0", q),
		fmt.Sprintf("hinicadm3 qos -i %s --dev_trust dscp", q),
		fmt.Sprintf("hinicadm3 qos -i %s --port_trust dscp", q),
		fmt.Sprintf("hinicadm3 qos -i %s -t ets -c 0,1,2,3,4,5,6,7 -w 10,0,0,0,90,0,0,0", q),
		"sed -i '/net.ipv4.conf.all.arp_ignore/d' /etc/sysctl.conf",
		"echo 'net.ipv4.conf.all.arp_ignore=1' >> /etc/sysctl.conf",
		"sysctl -w net.ipv4.conf.all.arp_ignore=1",
	}
}

func persistQoS(commands []string, systemd bool, r *Reporter, runner Runner) error {
	script := "#!/bin/sh\nset -eu\n" + strings.Join(commands, "\n") + "\n"
	changed, err := writeFileChanged("/usr/local/sbin/storctl-qos.sh", []byte(script), 0755)
	if err != nil {
		return err
	}
	if systemd {
		unit := `[Unit]
Description=storctl QoS restore
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/storctl-qos.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`
		if _, err := writeFileChanged("/etc/systemd/system/storctl-qos.service", []byte(unit), 0644); err != nil {
			return err
		}
		if _, err := runner.Run("systemctl", "daemon-reload"); err != nil {
			return err
		}
		if _, err := runner.Run("systemctl", "enable", "storctl-qos.service"); err != nil {
			return err
		}
		r.OK("qos persistence systemd")
		return nil
	}
	if changed {
		r.Warn("qos persistence script written: /usr/local/sbin/storctl-qos.sh")
	} else {
		r.Skip("qos persistence script unchanged")
	}
	r.Warn("systemd not detected: run storctl-qos.sh from boot scripts")
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
