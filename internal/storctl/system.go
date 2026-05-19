package storctl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type State struct {
	UpdatedAt       time.Time   `json:"updated_at"`
	NIC             string      `json:"nic"`
	NICType         string      `json:"nic_type"`
	VLAN            string      `json:"vlan"`
	DataCIDR        string      `json:"data_cidr"`
	Gateway         string      `json:"gateway"`
	RouteTable      int         `json:"route_table"`
	MTU             int         `json:"mtu"`
	Mounts          []MountSpec `json:"mounts"`
	RebootRequired  bool        `json:"reboot_required"`
	SystemdDetected bool        `json:"systemd_detected"`
}

func requireRoot() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("root is required")
	}
	return nil
}

func detectOS() (string, string, error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", "", err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[key] = strings.Trim(value, `"`)
	}
	return values["ID"], values["VERSION_ID"], nil
}

func supportedOpenEuler(version string) bool {
	return strings.HasPrefix(version, "22") || strings.HasPrefix(version, "23") || strings.HasPrefix(version, "24")
}

func isOpenEuler(id string) bool {
	return strings.EqualFold(id, "openeuler")
}

func hasSystemd(r Runner) bool {
	if !r.Exists("systemctl") {
		return false
	}
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

func backupIfExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	backup := fmt.Sprintf("%s.bak.%s", path, time.Now().Format("20060102150405"))
	return os.WriteFile(backup, data, 0644)
}

func writeFileChanged(path string, data []byte, mode os.FileMode) (bool, error) {
	if old, err := os.ReadFile(path); err == nil && string(old) == string(data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}
	if err := backupIfExists(path); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return false, err
	}
	return true, nil
}

func saveState(cfg Config, nicType string, rebootRequired, systemd bool) error {
	state := State{
		UpdatedAt:       time.Now(),
		NIC:             cfg.NIC,
		NICType:         nicType,
		VLAN:            cfg.vlanName(),
		DataCIDR:        cfg.DataCIDR,
		Gateway:         cfg.Gateway,
		RouteTable:      cfg.RouteTable,
		MTU:             cfg.MTU,
		Mounts:          cfg.Mounts,
		RebootRequired:  rebootRequired,
		SystemdDetected: systemd,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.StateDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cfg.StateDir, "state.json"), append(data, '\n'), 0644)
}

func loadState(stateDir string) (State, error) {
	data, err := os.ReadFile(filepath.Join(stateDir, "state.json"))
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}
