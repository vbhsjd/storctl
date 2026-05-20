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
	"sort"
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
	got, err := sha256File(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, strings.TrimSpace(expected)) {
		return fmt.Errorf("%s sha256 mismatch: got %s want %s", path, got, expected)
	}
	return nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type ManifestGenerateConfig struct {
	ArtifactDir     string
	OSID            string
	OSVersionPrefix string
	Arch            string
}

func GenerateManifest(cfg ManifestGenerateConfig, out io.Writer, errw io.Writer) error {
	entries, err := os.ReadDir(cfg.ArtifactDir)
	if err != nil {
		return err
	}
	manifest := ArtifactManifest{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == artifactManifestName {
			continue
		}
		nicType, requiresRepo, ok := classifyArtifact(entry.Name())
		if !ok {
			fmt.Fprintf(errw, "WARN artifact ignored %s\n", entry.Name())
			continue
		}
		sum, err := sha256File(filepath.Join(cfg.ArtifactDir, entry.Name()))
		if err != nil {
			return err
		}
		manifest.Artifacts = append(manifest.Artifacts, Artifact{
			OSID:            cfg.OSID,
			OSVersionPrefix: cfg.OSVersionPrefix,
			Arch:            cfg.Arch,
			NICType:         nicType,
			File:            entry.Name(),
			SHA256:          sum,
			RequiresRepo:    requiresRepo,
		})
	}
	sort.Slice(manifest.Artifacts, func(i, j int) bool {
		return manifest.Artifacts[i].File < manifest.Artifacts[j].File
	})
	if len(manifest.Artifacts) == 0 {
		return fmt.Errorf("no supported artifacts found in %s", cfg.ArtifactDir)
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(manifest)
}

func classifyArtifact(name string) (nicType string, requiresRepo bool, ok bool) {
	low := strings.ToLower(name)
	switch {
	case strings.HasPrefix(low, "doca-host") && strings.HasSuffix(low, ".rpm"):
		return "cx7", true, true
	case strings.HasPrefix(low, "mlnx_ofed_linux-") || strings.HasPrefix(low, "ib_nic-"):
		if strings.HasSuffix(low, ".tgz") || strings.HasSuffix(low, ".tar.gz") {
			return "cx7", false, true
		}
	case strings.HasPrefix(low, "nic_1823") || strings.HasPrefix(low, "hinic"):
		if strings.HasSuffix(low, ".tgz") || strings.HasSuffix(low, ".tar.gz") {
			return "1823", false, true
		}
	}
	return "", false, false
}

func ValidateArtifacts(dir string) error {
	manifest, err := readArtifactManifest(filepath.Join(dir, artifactManifestName))
	if err != nil {
		return err
	}
	for _, artifact := range manifest.Artifacts {
		if artifact.File == "" {
			return fmt.Errorf("artifact file is required")
		}
		path := filepath.Join(dir, artifact.File)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("artifact file missing: %s", path)
		}
		if artifact.OSID == "" || artifact.OSVersionPrefix == "" || artifact.Arch == "" || artifact.NICType == "" {
			return fmt.Errorf("artifact %s missing os_id/os_version_prefix/arch/nic_type", artifact.File)
		}
		if artifact.NICType != "cx7" && artifact.NICType != "1823" {
			return fmt.Errorf("artifact %s has unsupported nic_type %s", artifact.File, artifact.NICType)
		}
		if artifact.SHA256 != "" {
			if err := verifySHA256(path, artifact.SHA256); err != nil {
				return err
			}
		}
	}
	return nil
}
