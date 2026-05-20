package storctl

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	var out, stderr bytes.Buffer
	code := Main([]string{"help"}, &out, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(out.String(), "storctl apply") {
		t.Fatalf("help missing apply usage: %s", out.String())
	}
}

func TestApplyArgValidation(t *testing.T) {
	var out, stderr bytes.Buffer
	code := Main([]string{"apply", "--nic", "eth0"}, &out, &stderr)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "FAIL args") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestVersionJSON(t *testing.T) {
	var out, stderr bytes.Buffer
	code := Main([]string{"version", "--json"}, &out, &stderr)
	if code != 0 {
		t.Fatalf("code = %d stderr=%s", code, stderr.String())
	}
	var info VersionInfo
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Version == "" || info.Commit == "" || info.BuiltAt == "" || info.Go == "" {
		t.Fatalf("incomplete version info: %+v", info)
	}
}
