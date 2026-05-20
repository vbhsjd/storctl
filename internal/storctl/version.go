package storctl

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
)

var (
	Version = "dev"
	Commit  = "unknown"
	BuiltAt = "unknown"
)

type VersionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
	Go      string `json:"go"`
}

func currentVersion() VersionInfo {
	return VersionInfo{
		Version: Version,
		Commit:  Commit,
		BuiltAt: BuiltAt,
		Go:      runtime.Version(),
	}
}

func PrintVersion(jsonOut bool, out io.Writer) error {
	info := currentVersion()
	if jsonOut {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}
	fmt.Fprintf(out, "storctl %s\ncommit %s\nbuilt_at %s\ngo %s\n", info.Version, info.Commit, info.BuiltAt, info.Go)
	return nil
}
