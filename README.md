# storctl

`storctl` is a small Go CLI for joining lab hosts to NFS-RDMA storage.

It configures one data NIC, a VLAN interface through NetworkManager, routing
rules, CX7/1823 QoS, NFS-RDMA mounts, and mount persistence. It is designed to
be copied to a host and run directly, or called by Ansible.

## Build

```bash
go build ./cmd/storctl
```

For the intended target:

```bash
GOOS=linux GOARCH=arm64 go build -o storctl-linux-arm64 ./cmd/storctl
```

## Usage

```bash
storctl apply \
  --nic enp189s0f0np0 \
  --nic-type auto \
  --vlan-id 3001 \
  --data-ip 172.27.1.123/18 \
  --gateway 172.27.0.1 \
  --route-table 5000 \
  --mtu 5500 \
  --artifact-dir /root/storage_pkgs \
  --mount 172.27.0.50:/export/a:/mnt/a \
  --mount 172.27.0.51:/export/b:/mnt/b
```

Check current state:

```bash
storctl check
```

Short help:

```bash
storctl help
```

## Notes

- It does not implement DTFS, `cid`, `dn`, or zone generation.
- It installs or upgrades drivers by default when required.
- Firmware upgrade is disabled unless `--upgrade-firmware` is set.
- Artifacts are read from `--artifact-dir`; the tool does not fetch packages
  from the public internet.
- CX7 driver install supports both `doca-host*.rpm` followed by
  `dnf -y install doca-ofed`, and legacy `MLNX_OFED_LINUX-*.tgz` /
  `IB_NIC-*.tgz`.
- With systemd, mounts use `.mount/.automount` units. Without systemd, mounts
  are persisted in `/etc/fstab`.

For DOCA-Host, put the host RPM in the artifact directory:

```bash
wget https://www.mellanox.com/downloads/DOCA/DOCA_v3.3.0/host/doca-host-3.3.0-088000_26.01_openeuler2403.aarch64.rpm
mkdir -p /root/storage_pkgs
cp doca-host-3.3.0-088000_26.01_openeuler2403.aarch64.rpm /root/storage_pkgs/
```
