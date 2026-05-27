package storctl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	osInfo, err := detectOSInfo()
	if err != nil {
		return Artifact{}, err
	}
	arch := artifactArch()
	return selectArtifactFromManifestForOS(manifest, dir, osInfo, arch, nicType)
}

func selectArtifactFromManifest(manifest ArtifactManifest, dir, osID, osVersion, arch, nicType string) (Artifact, error) {
	info := OSInfo{
		ID:                osID,
		VersionID:         osVersion,
		Version:           osVersion,
		PrettyName:        osVersion,
		NormalizedVersion: normalizeOSVersion(osVersion),
	}
	return selectArtifactFromManifestForOS(manifest, dir, info, arch, nicType)
}

func selectArtifactFromManifestForOS(manifest ArtifactManifest, dir string, osInfo OSInfo, arch, nicType string) (Artifact, error) {
	matches := []Artifact{}
	bestScore := -1
	for _, artifact := range manifest.Artifacts {
		if !strings.EqualFold(artifact.NICType, nicType) {
			continue
		}
		if !strings.EqualFold(artifact.OSID, osInfo.ID) {
			continue
		}
		score, ok := matchOSVersionPrefix(artifact.OSVersionPrefix, osInfo)
		if !ok {
			continue
		}
		if !strings.EqualFold(artifact.Arch, arch) {
			continue
		}
		if score > bestScore {
			bestScore = score
			matches = []Artifact{artifact}
			continue
		}
		if score == bestScore {
			matches = append(matches, artifact)
		}
	}
	if len(matches) == 0 {
		return Artifact{}, fmt.Errorf("no artifact matches os=%s version=%s arch=%s nic_type=%s", osInfo.ID, displayOSVersion(osInfo), arch, nicType)
	}
	file := matches[0].File
	for _, artifact := range matches {
		if artifact.File == "" {
			return Artifact{}, fmt.Errorf("matched artifact has empty file")
		}
		if artifact.File != file {
			return Artifact{}, fmt.Errorf("ambiguous artifacts match os=%s version=%s arch=%s nic_type=%s with same specificity: %s, %s", osInfo.ID, displayOSVersion(osInfo), arch, nicType, file, artifact.File)
		}
	}
	if _, err := os.Stat(hostPath(filepath.Join(dir, file))); err != nil {
		return Artifact{}, fmt.Errorf("matched artifact file missing: %s", filepath.Join(dir, file))
	}
	return matches[0], nil
}

func matchOSVersionPrefix(prefix string, osInfo OSInfo) (int, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return 0, true
	}
	lowPrefix := strings.ToLower(prefix)
	normalizedPrefix := normalizeOSVersion(prefix)
	for _, candidate := range osVersionCandidates(osInfo) {
		lowCandidate := strings.ToLower(candidate)
		if strings.HasPrefix(lowCandidate, lowPrefix) {
			return len(lowPrefix), true
		}
		if normalizedPrefix != "" && strings.HasPrefix(lowCandidate, normalizedPrefix) {
			return len(normalizedPrefix), true
		}
	}
	return 0, false
}

func osVersionCandidates(osInfo OSInfo) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, candidate := range []string{osInfo.VersionID, osInfo.Version, osInfo.PrettyName, osInfo.NormalizedVersion} {
		if normalized := normalizeOSVersion(candidate); normalized != "" {
			candidate = strings.TrimSpace(candidate) + "\n" + normalized
		}
		for _, part := range strings.Split(candidate, "\n") {
			part = strings.TrimSpace(part)
			if part == "" || seen[strings.ToLower(part)] {
				continue
			}
			seen[strings.ToLower(part)] = true
			out = append(out, part)
		}
	}
	return out
}

func displayOSVersion(osInfo OSInfo) string {
	for _, value := range []string{osInfo.Version, osInfo.PrettyName, osInfo.VersionID, osInfo.NormalizedVersion} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "unknown"
}

func readArtifactManifest(path string) (ArtifactManifest, error) {
	data, err := os.ReadFile(hostPath(path))
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
	if simMode() {
		if arch := strings.TrimSpace(os.Getenv("STORCTL_SIM_ARCH")); arch != "" {
			return arch
		}
	}
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
	file, err := os.Open(hostPath(path))
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
	artifactDir := hostPath(cfg.ArtifactDir)
	entries, err := os.ReadDir(artifactDir)
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
		sum, err := sha256File(filepath.Join(artifactDir, entry.Name()))
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
	canonical := strings.NewReplacer(" ", "_", "-", "_").Replace(low)
	if strings.Contains(canonical, "source") || strings.Contains(canonical, "_src") || strings.Contains(canonical, "src_") {
		return "", false, false
	}
	switch {
	case strings.HasPrefix(low, "doca-host") && strings.HasSuffix(low, ".rpm"):
		return "cx7", true, true
	case strings.HasPrefix(low, "mlnx_ofed_linux-") || strings.HasPrefix(low, "ib_nic-"):
		if strings.HasSuffix(low, ".tgz") || strings.HasSuffix(low, ".tar.gz") {
			return "cx7", false, true
		}
	case strings.HasPrefix(canonical, "nic_1823") || strings.HasPrefix(canonical, "hinic") || strings.HasPrefix(canonical, "sdk_linux"):
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
	problems := []string{}
	for _, artifact := range manifest.Artifacts {
		if artifact.File == "" {
			problems = append(problems, "artifact file is required")
			continue
		}
		path := filepath.Join(dir, artifact.File)
		if _, err := os.Stat(hostPath(path)); err != nil {
			problems = append(problems, fmt.Sprintf("artifact file missing: %s", path))
			continue
		}
		if artifact.OSID == "" || artifact.OSVersionPrefix == "" || artifact.Arch == "" || artifact.NICType == "" {
			problems = append(problems, fmt.Sprintf("artifact %s missing os_id/os_version_prefix/arch/nic_type", artifact.File))
		}
		if artifact.NICType != "cx7" && artifact.NICType != "1823" {
			problems = append(problems, fmt.Sprintf("artifact %s has unsupported nic_type %s", artifact.File, artifact.NICType))
		}
		if artifact.SHA256 != "" {
			if err := verifySHA256(path, artifact.SHA256); err != nil {
				problems = append(problems, err.Error())
			}
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
