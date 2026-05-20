package storctl

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCheckReportJSONCodes(t *testing.T) {
	runner := &fakeRunner{
		exists: map[string]bool{"rdma": true},
		outputs: map[string]string{
			"rdma link": "",
		},
	}
	report := collectCheckReport(Config{StateDir: t.TempDir()}, runner)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "rdma_link_empty") {
		t.Fatalf("missing rdma code: %s", got)
	}
	if report.Summary.Warn == 0 {
		t.Fatalf("expected warnings: %+v", report.Summary)
	}
}
