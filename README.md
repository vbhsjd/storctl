# storctl

[English documentation](README.en.md)

`storctl` 用于把实验室机器接入 NFS-RDMA 存储。

它会配置存储网卡、NetworkManager VLAN、策略路由、CX7/1823 QoS、NFS-RDMA 挂载、挂载持久化，并写入一个状态文件供后续检查。它既可以单机直接运行，也可以被 Ansible 调用做批量接入。

## 快速开始

单机显式参数模式：

```bash
storctl apply \
  --nic enp194s0f1np1 \
  --nic-type auto \
  --vlan-id 172 \
  --data-ip 172.27.2.146/18 \
  --gateway 172.27.0.1 \
  --route-table 5000 \
  --artifact-dir /root/storage_pkgs \
  --mount 172.27.1.1:/Share:/mnt/share \
  --mount 172.27.1.1:/Weight:/mnt/weight
```

Profile 模式：

```bash
storctl plan --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113
storctl apply --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113
```

检查当前机器状态：

```bash
storctl check
```

## 工作流

```mermaid
flowchart TD
  A["storctl plan/apply"] --> B["加载 profile"]
  B --> C["检测管理 IP"]
  C --> D["推导数据网 IP"]
  D --> E["合并 CLI 覆盖参数"]
  E --> F["检查 OS/NIC/驱动"]
  F --> G["配置 NetworkManager VLAN"]
  G --> H["应用 QoS"]
  H --> I["检查 RDMA 客户端"]
  I --> J["持久化挂载"]
  J --> K["挂载 NFS-RDMA"]
  K --> L["写入 state.json"]
```

`plan` 只输出最终配置，不修改机器。`apply` 执行完整接入流程。

## Profiles

Profile 用来减少每台机器需要传入的参数。`storctl` 按下面顺序查找 profile 文件：

1. `--profile-file /path/to/storctl-profiles.json`
2. `./storctl-profiles.json`
3. `/etc/storctl/profiles.json`

示例：

```json
{
  "profiles": {
    "c4": {
      "vlan_id": 172,
      "gateway": "172.27.0.1",
      "prefix": 18,
      "route_table": 5000,
      "mtu": 5500,
      "artifact_dir": "/root/storage_pkgs",
      "third_octet_map": {
        "17": 4
      },
      "mounts": [
        {"server": "172.27.1.1", "export": "/Share", "mount_point": "/mnt/share"},
        {"server": "172.27.1.1", "export": "/Weight", "mount_point": "/mnt/weight"}
      ]
    }
  }
}
```

数据网 IP 根据管理 IP 推导：

```text
mgmt-ip 80.5.17.113
third_octet_map["17"] = 4
prefix = 18
result = 172.27.4.113/18
```

CLI 参数优先级最高。比如显式传 `--data-ip` 会跳过 IP 推导，重复传 `--mount` 会覆盖 profile 里的挂载配置。

## 批量使用

推荐 Ansible 形态：

```bash
ansible all -m copy -a "src=storctl-linux-arm64 dest=/usr/local/bin/storctl mode=0755"
ansible all -m copy -a "src=storctl-profiles.json dest=/etc/storctl/profiles.json"
ansible all -m shell -a "storctl plan --profile c4 --nic {{ storage_nic }} --mgmt-ip {{ ansible_host }}"
ansible all -m shell -a "storctl apply --profile c4 --nic {{ storage_nic }} --mgmt-ip {{ ansible_host }}"
```

如果有多个集群，每个集群维护一个 profile，并在 inventory 中传入 profile 名称：

```bash
ansible all -m shell -a "storctl apply --profile {{ storage_profile }} --nic {{ storage_nic }} --mgmt-ip {{ ansible_host }}"
```

## 构建

```bash
go test ./...
go build ./cmd/storctl
GOOS=linux GOARCH=arm64 go build -o storctl-linux-arm64 ./cmd/storctl
```

## 驱动包

驱动包从 `--artifact-dir` 指定目录读取；`storctl` 不会主动从公网下载包。

- CX7 支持 `doca-host*.rpm`，随后通过机器已有 `dnf` 源安装 `doca-ofed`。
- CX7 也兼容旧的 `MLNX_OFED_LINUX-*.tgz` 和 `IB_NIC-*.tgz`。
- 1823 支持 `nic_1823.tar.gz` 或 `hinic*.tar.gz`。
- 默认不升级固件，除非显式传 `--upgrade-firmware`。

DOCA-Host 示例：

```bash
wget https://www.mellanox.com/downloads/DOCA/DOCA_v3.3.0/host/doca-host-3.3.0-088000_26.01_openeuler2403.aarch64.rpm
mkdir -p /root/storage_pkgs
cp doca-host-3.3.0-088000_26.01_openeuler2403.aarch64.rpm /root/storage_pkgs/
```

## 排障

`rdma link` 为空：

- 当前机器没有可用 RDMA 设备，NFS-RDMA 还不能工作。
- 检查驱动和模块：
  ```bash
  rdma link
  lsmod | grep -iE 'rdma|roce|ib_|uverbs|xprtrdma|hinic|mlx5'
  modprobe xprtrdma
  ```

挂载是 TCP，不是 RDMA：

- `storctl` 发现 `proto=tcp` 时会尝试重新挂成 RDMA。
- 验证命令：
  ```bash
  findmnt --mountpoint /mnt/share -o TARGET,FSTYPE,SOURCE,OPTIONS
  nfsstat -m
  ```

systemd automount 失败：

- `storctl` 会 fallback 到直接 `mount -t nfs`。
- 查看 unit 日志：
  ```bash
  systemctl status mnt-share.automount --no-pager
  journalctl -u mnt-share.automount -xe
  ```

1823 缺少 ECN sysfs：

- 部分 1823 驱动版本没有 `/sys/class/net/<nic>/ecn/cc_algo`。
- `storctl` 会把它当作可选项跳过，并继续执行 `hinicadm3 qos`。

## 说明

- `storctl` 不实现 DTFS、`cid`、`dn` 或 zone 生成。
- 状态文件写入 `/var/lib/storctl/state.json`。
- 有 systemd 时使用 `.mount/.automount` 持久化挂载；没有 systemd 时写入 `/etc/fstab`。
