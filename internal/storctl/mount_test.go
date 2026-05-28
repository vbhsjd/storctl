package storctl

import (
	"os"
	"path/filepath"
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

func TestConfigureMountsDoesNotFallbackToTCPByDefault(t *testing.T) {
	r := &fakeRunner{
		exists: map[string]bool{"rdma": true},
		outputs: map[string]string{
			"rdma link": "",
		},
	}
	cfg := Config{
		Mounts: []MountSpec{{
			Server:     "172.27.1.1",
			Export:     "/Share",
			MountPoint: t.TempDir(),
			Options:    defaultNFSOptions,
		}},
	}
	var out, stderr strings.Builder
	_, err := configureMounts(cfg, false, NewReporter(&out, &stderr), r)
	if err == nil || !strings.Contains(err.Error(), "rdma link is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out.String(), "proto=tcp") || strings.Contains(stderr.String(), "proto=tcp") {
		t.Fatalf("unexpected tcp fallback output: stdout=%q stderr=%q", out.String(), stderr.String())
	}
}

func TestTCPFallbackOptionsUseTCP(t *testing.T) {
	got := tcpFallbackOptions(defaultNFSOptions)
	if !containsAll(got, "vers=3", "proto=tcp", "nolock", "nconnect=8") || strings.Contains(got, "proto=rdma") {
		t.Fatalf("unexpected tcp options: %s", got)
	}
}

func TestEnsureNetworkManagerStartedSkipsWhenAlreadyRunning(t *testing.T) {
	r := &fakeRunner{
		exists: map[string]bool{"systemctl": true},
		outputs: map[string]string{
			"nmcli -t -f RUNNING general": "running\n",
		},
	}
	var out, stderr strings.Builder
	if err := ensureNetworkManagerStarted(r, NewReporter(&out, &stderr)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("unexpected calls: %+v", r.calls)
	}
}

func TestEnsureNetworkManagerStartedUsesSystemctl(t *testing.T) {
	r := &fakeRunner{
		exists: map[string]bool{"systemctl": true},
		outputs: map[string]string{
			"nmcli -t -f RUNNING general":    "running\n",
			"systemctl start NetworkManager": "",
		},
		errors: map[string]error{
			"nmcli -t -f RUNNING general": failf("NetworkManager is not running"),
		},
		failOnce: map[string]bool{
			"nmcli -t -f RUNNING general": true,
		},
	}
	var out, stderr strings.Builder
	if err := ensureNetworkManagerStarted(r, NewReporter(&out, &stderr)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsAll(strings.Join(r.calls, "\n"), "systemctl start NetworkManager") {
		t.Fatalf("NetworkManager start was not attempted: %+v", r.calls)
	}
}

func TestCleanupLegacySystemdMountRemovesUnits(t *testing.T) {
	root := t.TempDir()
	t.Setenv(simRootEnv, root)
	systemdDir := filepath.Join(root, "etc/systemd/system")
	if err := os.MkdirAll(systemdDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mnt-share.mount", "mnt-share.automount"} {
		if err := os.WriteFile(filepath.Join(systemdDir, name), []byte("legacy"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	r := &fakeRunner{
		exists: map[string]bool{"systemctl": true},
		outputs: map[string]string{
			"systemctl disable --now mnt-share.automount": "",
			"systemctl disable --now mnt-share.mount":     "",
			"systemctl daemon-reload":                     "",
		},
	}
	var out, stderr strings.Builder
	cleanupLegacySystemdMount(MountSpec{MountPoint: "/mnt/share"}, NewReporter(&out, &stderr), r)
	for _, name := range []string{"mnt-share.mount", "mnt-share.automount"} {
		if _, err := os.Stat(filepath.Join(systemdDir, name)); !os.IsNotExist(err) {
			t.Fatalf("legacy unit %s still exists: %v", name, err)
		}
	}
	if !containsAll(strings.Join(r.calls, "\n"), "systemctl disable --now mnt-share.automount", "systemctl daemon-reload") {
		t.Fatalf("unexpected systemctl calls: %+v", r.calls)
	}
}

type fakeRunner struct {
	exists   map[string]bool
	outputs  map[string]string
	errors   map[string]error
	failOnce map[string]bool
	calls    []string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	key := name
	for _, arg := range args {
		key += " " + arg
	}
	f.calls = append(f.calls, key)
	if err, ok := f.errors[key]; ok {
		if f.failOnce[key] {
			delete(f.errors, key)
			delete(f.failOnce, key)
		}
		return "", err
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
