package storctl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactManifestSelectsMatchingArtifact(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cx7.tgz"), []byte("driver"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := ArtifactManifest{Artifacts: []Artifact{{
		OSID:            "openEuler",
		OSVersionPrefix: "22.03",
		Arch:            "aarch64",
		NICType:         "cx7",
		File:            "cx7.tgz",
	}}}
	got, err := selectArtifactFromManifest(manifest, dir, "openEuler", "22.03", "aarch64", "cx7")
	if err != nil {
		t.Fatal(err)
	}
	if got.File != "cx7.tgz" {
		t.Fatalf("File = %q", got.File)
	}
}

func TestArtifactManifestSelectsMostSpecificSPMatch(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"generic.tgz", "sp4.tgz"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("driver"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := ArtifactManifest{Artifacts: []Artifact{
		{OSID: "openEuler", OSVersionPrefix: "22.03", Arch: "aarch64", NICType: "1823", File: "generic.tgz"},
		{OSID: "openEuler", OSVersionPrefix: "22.03-LTS-SP4", Arch: "aarch64", NICType: "1823", File: "sp4.tgz"},
	}}
	got, err := selectArtifactFromManifestForOS(manifest, dir, OSInfo{
		ID:                "openEuler",
		VersionID:         "22.03",
		Version:           "22.03 (LTS-SP4)",
		PrettyName:        "openEuler 22.03 (LTS-SP4)",
		NormalizedVersion: "22.03-lts-sp4",
	}, "aarch64", "1823")
	if err != nil {
		t.Fatal(err)
	}
	if got.File != "sp4.tgz" {
		t.Fatalf("File = %q, want sp4.tgz", got.File)
	}
}

func TestArtifactManifestFallsBackToBroadVersionPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "generic.tgz"), []byte("driver"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := ArtifactManifest{Artifacts: []Artifact{
		{OSID: "openEuler", OSVersionPrefix: "22.03", Arch: "aarch64", NICType: "1823", File: "generic.tgz"},
	}}
	got, err := selectArtifactFromManifestForOS(manifest, dir, OSInfo{
		ID:                "openEuler",
		VersionID:         "22.03",
		Version:           "22.03 (LTS-SP4)",
		PrettyName:        "openEuler 22.03 (LTS-SP4)",
		NormalizedVersion: "22.03-lts-sp4",
	}, "aarch64", "1823")
	if err != nil {
		t.Fatal(err)
	}
	if got.File != "generic.tgz" {
		t.Fatalf("File = %q, want generic.tgz", got.File)
	}
}

func TestArtifactManifestReportsAmbiguousSameSpecificity(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.tgz", "b.tgz"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("driver"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := ArtifactManifest{Artifacts: []Artifact{
		{OSID: "openEuler", OSVersionPrefix: "22.03-LTS-SP4", Arch: "aarch64", NICType: "1823", File: "a.tgz"},
		{OSID: "openEuler", OSVersionPrefix: "22.03-lts-sp4", Arch: "aarch64", NICType: "1823", File: "b.tgz"},
	}}
	_, err := selectArtifactFromManifestForOS(manifest, dir, OSInfo{
		ID:                "openEuler",
		VersionID:         "22.03",
		Version:           "22.03 (LTS-SP4)",
		PrettyName:        "openEuler 22.03 (LTS-SP4)",
		NormalizedVersion: "22.03-lts-sp4",
	}, "aarch64", "1823")
	if err == nil || !strings.Contains(err.Error(), "ambiguous artifacts") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestArtifactManifestReportsNoMatch(t *testing.T) {
	manifest := ArtifactManifest{Artifacts: []Artifact{{
		OSID:            "openEuler",
		OSVersionPrefix: "24.03",
		Arch:            "aarch64",
		NICType:         "cx7",
		File:            "cx7.tgz",
	}}}
	_, err := selectArtifactFromManifest(manifest, t.TempDir(), "openEuler", "22.03", "aarch64", "cx7")
	if err == nil || !strings.Contains(err.Error(), "no artifact matches") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifySHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "driver.tgz")
	data := []byte("driver")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if err := verifySHA256(path, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256(path, strings.Repeat("0", 64)); err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestGenerateManifest(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"MLNX_OFED_LINUX-test.tgz": "cx7",
		"nic_1823-test.tar.gz":     "1823",
		"SDK_LINUX-test.tar.gz":    "1823-sdk",
		"doca-host-test.rpm":       "doca",
		"ignore.txt":               "ignore",
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var out, stderr bytes.Buffer
	err := GenerateManifest(ManifestGenerateConfig{
		ArtifactDir:     dir,
		OSID:            "openEuler",
		OSVersionPrefix: "22.03",
		Arch:            "aarch64",
	}, &out, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "WARN artifact ignored ignore.txt") {
		t.Fatalf("missing ignored warning: %s", stderr.String())
	}
	var manifest ArtifactManifest
	if err := json.Unmarshal(out.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Artifacts) != 4 {
		t.Fatalf("Artifacts len = %d", len(manifest.Artifacts))
	}
	foundRepo := false
	for _, artifact := range manifest.Artifacts {
		if artifact.File == "doca-host-test.rpm" && artifact.RequiresRepo && artifact.NICType == "cx7" {
			foundRepo = true
		}
		if artifact.SHA256 == "" {
			t.Fatalf("missing sha256 for %+v", artifact)
		}
	}
	if !foundRepo {
		t.Fatalf("doca-host artifact not marked requires_repo: %+v", manifest.Artifacts)
	}
}

func TestValidateArtifactsReportsMultipleProblems(t *testing.T) {
	dir := t.TempDir()
	manifest := ArtifactManifest{Artifacts: []Artifact{
		{OSID: "openEuler", OSVersionPrefix: "22.03", Arch: "aarch64", NICType: "bad", File: "missing.tgz"},
		{OSID: "openEuler", OSVersionPrefix: "22.03", Arch: "aarch64", NICType: "cx7", File: "cx7.tgz", SHA256: strings.Repeat("0", 64)},
	}}
	if err := os.WriteFile(filepath.Join(dir, "cx7.tgz"), []byte("driver"), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, artifactManifestName), data, 0644); err != nil {
		t.Fatal(err)
	}
	err = ValidateArtifacts(dir)
	if err == nil || !containsAll(err.Error(), "missing.tgz", "sha256 mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
