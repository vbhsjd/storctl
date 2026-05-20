package storctl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const artifactManifestName = "storctl-artifacts.json"

type ArtifactManifest struct {
	Artifacts []Artifact `json:"artifacts"`
}

type Artifact struct {
	OSID            string `json:"os_id"`
	OSVersionPrefix string `json:"os_version_prefix"`
	Arch            string `json:"arch"`
	NICType         string `json:"nic_type"`
	File            string `json:"file"`
	SHA256          string `json:"sha256"`
	RequiresRepo    bool   `json:"requires_repo"`
}

func selectArtifact(dir, nicType string) (Artifact, error) {
	manifest, err := readArtifactManifest(filepath.Join(dir, artifactManifestName))
	if err != nil {
		return Artifact{}, err
	}
	osID, osVersion, err := detectOS()
	if err != nil {
		return Artifact{}, err
	}
	arch := artifactArch()
	return selectArtifactFromManifest(manifest, dir, osID, osVersion, arch, nicType)
}

func selectArtifactFromManifest(manifest ArtifactManifest, dir, osID, osVersion, arch, nicType string) (Artifact, error) {
	for _, artifact := range manifest.Artifacts {
		if !strings.EqualFold(artifact.NICType, nicType) {
			continue
		}
		if !strings.EqualFold(artifact.OSID, osID) {
			continue
		}
		if artifact.OSVersionPrefix != "" && !strings.HasPrefix(osVersion, artifact.OSVersionPrefix) {
			continue
		}
		if !strings.EqualFold(artifact.Arch, arch) {
			continue
		}
		if artifact.File == "" {
			return Artifact{}, fmt.Errorf("matched artifact has empty file")
		}
		if _, err := os.Stat(filepath.Join(dir, artifact.File)); err != nil {
			return Artifact{}, fmt.Errorf("matched artifact file missing: %s", filepath.Join(dir, artifact.File))
		}
		return artifact, nil
	}
	return Artifact{}, fmt.Errorf("no artifact matches os=%s version=%s arch=%s nic_type=%s", osID, osVersion, arch, nicType)
}

func readArtifactManifest(path string) (ArtifactManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ArtifactManifest{}, fmt.Errorf("read %s: %w", path, err)
	}
	var manifest ArtifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ArtifactManifest{}, err
	}
	if len(manifest.Artifacts) == 0 {
		return ArtifactManifest{}, fmt.Errorf("%s has no artifacts", path)
	}
	return manifest, nil
}

func artifactArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64"
	case "amd64":
		return "x86_64"
	default:
		return runtime.GOARCH
	}
}

func verifySHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(expected)) {
		return fmt.Errorf("%s sha256 mismatch: got %s want %s", path, got, expected)
	}
	return nil
}
