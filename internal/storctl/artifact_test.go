package storctl

import (
	"crypto/sha256"
	"encoding/hex"
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
