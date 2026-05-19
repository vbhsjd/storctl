package storctl

import (
	"strings"
	"testing"
)

func TestSystemdMountUnitName(t *testing.T) {
	cases := map[string]string{
		"/mnt/a":       "mnt-a.mount",
		"/mnt/storage": "mnt-storage.mount",
		"/":            "-.mount",
	}
	for in, want := range cases {
		if got := systemdMountUnitName(in); got != want {
			t.Fatalf("systemdMountUnitName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVerifyRDMAMountIncludesDiagnostics(t *testing.T) {
	r := &fakeRunner{
		exists: map[string]bool{"findmnt": true, "nfsstat": true},
		outputs: map[string]string{
			"findmnt -n --mountpoint /mnt/share -o FSTYPE,OPTIONS": "nfs4 rw,vers=4.1,proto=tcp\n",
			"nfsstat -m": "/mnt/share from 172.27.1.1:/Share\n Flags: rw,proto=tcp\n",
		},
	}
	err := verifyRDMAMount(MountSpec{MountPoint: "/mnt/share"}, r)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !containsAll(got, "findmnt:", "nfsstat:", "proto=tcp") {
		t.Fatalf("missing diagnostics: %s", got)
	}
}

func TestRequireRDMAClientRejectsEmptyRDMALink(t *testing.T) {
	r := &fakeRunner{
		exists: map[string]bool{"rdma": true, "modprobe": true},
		outputs: map[string]string{
			"modprobe xprtrdma": "",
			"rdma link":         "",
		},
	}
	err := requireRDMAClient(r)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "rdma link is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireRDMAClientAcceptsRDMALink(t *testing.T) {
	r := &fakeRunner{
		exists: map[string]bool{"rdma": true},
		outputs: map[string]string{
			"rdma link": "link mlx5_1/1 state ACTIVE physical_state LINK_UP netdev enp194s0f1np1\n",
		},
	}
	if err := requireRDMAClient(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

type fakeRunner struct {
	exists  map[string]bool
	outputs map[string]string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	key := name
	for _, arg := range args {
		key += " " + arg
	}
	if out, ok := f.outputs[key]; ok {
		return out, nil
	}
	return "", failf("not found: %s", key)
}

func (f *fakeRunner) Sh(command string) (string, error) {
	return f.Run("/bin/sh", "-c", command)
}

func (f *fakeRunner) Exists(name string) bool {
	return f.exists[name]
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
