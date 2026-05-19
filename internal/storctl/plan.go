package storctl

import (
	"fmt"
)

func Plan(cfg Config, r *Reporter) error {
	r.OK("nic %s", cfg.NIC)
	r.OK("nic type %s", cfg.NICType)
	r.OK("vlan %s", cfg.vlanName())
	r.OK("data-ip %s", cfg.DataCIDR)
	r.OK("gateway %s", cfg.Gateway)
	r.OK("route-table %d", cfg.RouteTable)
	r.OK("mtu %d", cfg.MTU)
	r.OK("artifact-dir %s", cfg.ArtifactDir)
	for _, m := range cfg.Mounts {
		r.OK("mount %s:%s -> %s", m.Server, m.Export, m.MountPoint)
	}
	fmt.Fprintln(r.out, "OK plan no changes applied")
	return nil
}
