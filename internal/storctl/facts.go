package storctl

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

type FactsReport struct {
	SchemaVersion int             `json:"schema_version"`
	OS            OSFacts         `json:"os"`
	Systemd       bool            `json:"systemd"`
	Commands      map[string]bool `json:"commands"`
	ManagementIPs []string        `json:"management_ips"`
	Interfaces    []InterfaceFact `json:"interfaces"`
	RDMA          RDMAFacts       `json:"rdma"`
	IBDevMappings []string        `json:"ibdev_mappings,omitempty"`
}

type OSFacts struct {
	ID                string `json:"id"`
	Version           string `json:"version"`
	VersionID         string `json:"version_id,omitempty"`
	VersionText       string `json:"version_text,omitempty"`
	PrettyName        string `json:"pretty_name,omitempty"`
	NormalizedVersion string `json:"normalized_version,omitempty"`
	Arch              string `json:"arch"`
}

type InterfaceFact struct {
	Name    string   `json:"name"`
	Up      bool     `json:"up"`
	Ignored bool     `json:"ignored"`
	Addrs   []string `json:"addrs,omitempty"`
}

type RDMAFacts struct {
	CommandFound bool     `json:"command_found"`
	Available    bool     `json:"available"`
	Links        []string `json:"links,omitempty"`
}

func Facts(jsonOut bool, out io.Writer, runner Runner) error {
	report := collectFacts(runner)
	if jsonOut {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	fmt.Fprintf(out, "OK os %s %s arch=%s\n", report.OS.ID, report.OS.Version, report.OS.Arch)
	fmt.Fprintf(out, "OK systemd %t\n", report.Systemd)
	for _, ip := range report.ManagementIPs {
		fmt.Fprintf(out, "OK mgmt-ip %s\n", ip)
	}
	for _, iface := range report.Interfaces {
		if iface.Ignored {
			fmt.Fprintf(out, "SKIP nic %s ignored\n", iface.Name)
			continue
		}
		fmt.Fprintf(out, "OK nic %s up=%t addrs=%s\n", iface.Name, iface.Up, strings.Join(iface.Addrs, ","))
	}
	if report.RDMA.Available {
		fmt.Fprintf(out, "OK rdma links=%s\n", strings.Join(report.RDMA.Links, " | "))
	} else {
		fmt.Fprintln(out, "WARN rdma unavailable")
	}
	return nil
}

func collectFacts(runner Runner) FactsReport {
	osInfo, _ := detectOSInfo()
	mgmtIPs, _ := candidateManagementIPs()
	report := FactsReport{
		SchemaVersion: 1,
		OS: OSFacts{
			ID:                osInfo.ID,
			Version:           osInfo.VersionID,
			VersionID:         osInfo.VersionID,
			VersionText:       osInfo.Version,
			PrettyName:        osInfo.PrettyName,
			NormalizedVersion: osInfo.NormalizedVersion,
			Arch:              artifactArch(),
		},
		Systemd:  hasSystemd(runner),
		Commands: map[string]bool{},
	}
	for _, cmd := range []string{"nmcli", "rdma", "ibdev2netdev", "mlnx_qos", "cma_roce_tos", "hinicadm3", "findmnt", "nfsstat", "systemctl"} {
		report.Commands[cmd] = runner.Exists(cmd)
	}
	for _, ip := range mgmtIPs {
		report.ManagementIPs = append(report.ManagementIPs, ip.String())
	}
	report.Interfaces = collectInterfaceFacts()
	report.RDMA.CommandFound = runner.Exists("rdma")
	if report.RDMA.CommandFound {
		if out, err := runner.Run("rdma", "link"); err == nil {
			report.RDMA.Links = nonEmptyLines(out)
			report.RDMA.Available = len(report.RDMA.Links) > 0
		}
	}
	if runner.Exists("ibdev2netdev") {
		if out, err := runner.Run("ibdev2netdev"); err == nil {
			report.IBDevMappings = nonEmptyLines(out)
		}
	}
	return report
}

func collectInterfaceFacts() []InterfaceFact {
	if simMode() {
		return collectSimInterfaceFacts()
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	out := make([]InterfaceFact, 0, len(ifaces))
	for _, iface := range ifaces {
		fact := InterfaceFact{
			Name:    iface.Name,
			Up:      iface.Flags&net.FlagUp != 0,
			Ignored: isIgnoredInterface(iface.Name),
		}
		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				fact.Addrs = append(fact.Addrs, addr.String())
			}
		}
		out = append(out, fact)
	}
	return out
}

func collectSimInterfaceFacts() []InterfaceFact {
	entries, err := os.ReadDir(hostPath("/sys/class/net"))
	if err != nil {
		return nil
	}
	out := make([]InterfaceFact, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		state, _ := os.ReadFile(hostPath("/sys/class/net/" + name + "/operstate"))
		fact := InterfaceFact{
			Name:    name,
			Up:      strings.TrimSpace(string(state)) == "up",
			Ignored: isIgnoredInterface(name),
		}
		if addrs, err := os.ReadFile(hostPath("/sys/class/net/" + name + "/ipv4_addrs")); err == nil {
			for _, line := range nonEmptyLines(string(addrs)) {
				fact.Addrs = append(fact.Addrs, line)
			}
		}
		out = append(out, fact)
	}
	return out
}

func simManagementIPs() []net.IP {
	var out []net.IP
	for _, iface := range collectSimInterfaceFacts() {
		if !iface.Up || iface.Ignored {
			continue
		}
		for _, raw := range iface.Addrs {
			ip, _, err := net.ParseCIDR(raw)
			if err != nil {
				ip = net.ParseIP(raw)
			}
			if ip != nil && isManagementCandidate(ip.To4()) {
				out = append(out, ip.To4())
			}
		}
	}
	return out
}

func nonEmptyLines(raw string) []string {
	lines := []string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
