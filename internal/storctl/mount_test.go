package storctl

import "testing"

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
