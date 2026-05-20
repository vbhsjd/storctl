# Changelog

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
