# Changelog

## v0.5.7

### Added

- Added `storctl reconcile-mounts`, a small root-only command that reconciles
  mount persistence without changing NICs, VLANs, QoS, or current mounts.

### Changed

- `reconcile-mounts` removes legacy systemd `.mount/.automount` units for the
  configured mount points and writes `/etc/fstab`.
- When `--allow-tcp-fallback` is passed and the current mount is already
  `proto=tcp`, `reconcile-mounts` preserves TCP fallback options in fstab.

## v0.5.6

### Changed

- `apply` now checks whether NetworkManager is running and attempts to start it
  with `systemctl start NetworkManager`, falling back to
  `service NetworkManager start` when available.
- Mount persistence now always uses `/etc/fstab`; `storctl` no longer creates
  systemd `.mount/.automount` units for NFS mounts.
- When writing fstab persistence, `storctl` removes legacy systemd mount units
  for the same mount point if they were created by earlier versions.

## v0.5.4

### Changed

- NetworkManager VLAN setup now repairs stale VLAN links whose parent NIC has
  drifted, for example `data0.3001@eth4` when the selected storage NIC is
  `eth3`.
- `apply` now sets the parent NIC MTU before the VLAN MTU and rebuilds the VLAN
  link once if the existing link rejects the MTU.
- TCP fallback now defaults to NFSv3 TCP options
  `vers=3,proto=tcp,nolock,nconnect=8,hard,noatime`, matching lab storage
  servers that reject NFSv4.1 TCP.
- 1823 SDK installation defaults to driver RPM installation and only runs the
  vendor `install.sh roce` firmware-capable path when `--upgrade-firmware` is
  explicit.

## v0.5.3

### Added

- Added `STORCTL_SIM_ROOT` support for high-fidelity local integration tests
  without touching real `/etc`, `/sys`, `/run`, or `/var` paths.
- Simulation mode now bypasses root checks and routes shell snippets through a
  fake shell command, so QoS and installer tests can be run safely on a laptop.
- Added `STORCTL_SIM_ARCH` for simulated artifact matching on non-aarch64 test
  hosts.

## v0.5.2

### Changed

- Artifact selection now understands openEuler SP versions from `VERSION`,
  `VERSION_ID`, and `PRETTY_NAME`, and chooses the most specific
  `os_version_prefix` match.

## v0.5.1

### Added

- `apply` now fails before NetworkManager changes when `--nic` owns the
  `--mgmt-ip`, preventing accidental SSH management NIC reconfiguration.
- Added a detailed Chinese single-host tutorial under `docs/tutorial.md`.
- 1823 driver installation now uses the SDK installer entrypoint
  `bash install.sh roce` after extracting the artifact.
- `apply --allow-tcp-fallback` now continues past RDMA driver readiness
  failures and records the mount as degraded TCP fallback.

## v0.5.0

### Added

- `storctl facts` and `storctl facts --json` for collecting host facts without changing the host.
- Richer `check --json` details for state, OS, RDMA links, driver readiness, QoS mode, artifacts, links, and mounts.
- Strict profile validation that rejects unknown JSON fields.
- Artifact validation now reports multiple manifest/file/checksum problems at once.

### Changed

- `state.json` records `artifact_dir` for later check visibility.

## v0.4.0

### Changed

- QoS is disabled by default. Use `--qos apply` or profile `qos.enabled=true` to configure CX7/1823 QoS.
- `state.json` now includes `schema_version: 1` and remains compatible with legacy state files.
- Release archives now include docs, examples, license, and changelog.

### Added

- `storctl check --json` for stable machine-readable Ansible collection.
- `storctl version` and `storctl version --json`.
- `storctl generate-manifest` for local artifact manifest generation.
- `storctl validate-profile` and `storctl validate-artifacts`.
- Apache-2.0 license.

### Notes

- `storctl` remains a single-host tool. Batch orchestration, SSH, inventory, and artifact distribution stay outside the CLI.

## v0.3.1

### Changed

- Documented explicit NIC selection and rejected `--nic auto`.

## v0.3.0

### Added

- Offline driver artifact workflow with `storctl-artifacts.json`.
- Explicit `install-driver` command.
- Explicit TCP fallback with degraded state tracking.

## v0.2.0

### Added

- Profile-driven `plan` and `apply` workflow.
- Management-IP based data-IP derivation.
