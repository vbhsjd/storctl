# storctl 详细教程

这篇教程面向“我要把一台机器接入存储”的场景。批量接入请看 `storctl-compose`。

## 1. 你需要先理解边界

`storctl` 是单机命令，只负责当前这台机器：

- 检查 OS、网卡、驱动、RDMA、挂载状态。
- 配置存储物理网卡 MTU、VLAN 网卡、策略路由。
- 按需配置 QoS。
- 配置并挂载 NFS-RDMA。
- 写入 `/var/lib/storctl/state.json`，便于后续 `check`。

它不负责：

- 自动选择网卡。
- SSH 到其他机器。
- 分发驱动包。
- 维护 yum/dnf 仓库。
- 替你判断交换机侧 PFC/ECN 是否正确。

所以正确姿势是：单台先跑通，再用 `storctl-compose`/Ansible 批量调用。

## 2. 准备 storctl

下载 release 里的 `storctl-linux-arm64`，复制到目标机器：

```bash
install -m 0755 storctl-linux-arm64 /usr/local/bin/storctl
storctl version
```

目标机器需要 root 执行会修改系统的命令：

```bash
storctl plan     # 不修改机器，不需要 root
storctl check    # 不修改机器，不需要 root
storctl facts    # 不修改机器，不需要 root
storctl apply    # 修改机器，需要 root
storctl install-driver  # 安装驱动，需要 root
```

## 3. 确认网卡

`--nic` 必须是存储物理网卡，不能写 SSH 管理口。

常用检查：

```bash
ip -br addr
ip -br link
ibdev2netdev
hinicadm3 info
ethtool <nic> | grep -i speed
```

经验判断：

- CX7 通常能在 `ibdev2netdev` 里看到 `mlx5_x port 1 ==> <nic>`。
- 1823 通常能在 `hinicadm3 info` 里看到 NIC 和 PCIe Function。
- SSH 管理口一般有管理网 IP，例如 `80.5.x.x`。
- 存储网卡会被配置 VLAN，例如 `data0.172@enp23s0f1`。

如果传了 `--mgmt-ip`，`apply` 会在修改 NetworkManager 前检查 `--nic` 是否正好持有这个管理 IP。命中会提前失败，避免把 SSH 管理口改坏。

## 4. 选择显式参数还是 profile

单机临时测试可以用显式参数：

```bash
storctl apply \
  --nic enp23s0f1 \
  --nic-type auto \
  --vlan-id 172 \
  --data-ip 172.27.4.113/18 \
  --gateway 172.27.0.1 \
  --route-table 5000 \
  --artifact-dir /root/storage_pkgs \
  --mount 172.27.1.1:/Share:/mnt/share \
  --mount 172.27.1.1:/Weight:/mnt/weight
```

长期使用推荐 profile。把集群固定项写入 `/etc/storctl/profiles.json`：

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
        "17": 4,
        "21": 3
      },
      "mounts": [
        {"server": "172.27.1.1", "export": "/Share", "mount_point": "/mnt/share"},
        {"server": "172.27.1.1", "export": "/Weight", "mount_point": "/mnt/weight"}
      ]
    }
  }
}
```

然后执行：

```bash
storctl plan --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113
storctl apply --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113
```

`--mgmt-ip` 只用于推导数据网 IP。例如：

```text
mgmt-ip = 80.5.17.113
third_octet_map["17"] = 4
prefix = 18
data-ip = 172.27.4.113/18
```

如果你显式传 `--data-ip`，就不会使用这个推导逻辑。

## 5. 离线驱动目录

`storctl apply` 默认只检查驱动是否就绪，不会联网安装驱动。

离线驱动目录建议这样准备：

```text
/root/storage_pkgs/
  storctl-artifacts.json
  SDK_LINUX-xxx-openEuler22.03-aarch64.tar.gz
  MLNX_OFED_LINUX-xxx-openEuler24.03-aarch64.tgz
```

`storctl-artifacts.json` 示例：

```json
{
  "artifacts": [
    {
      "os_id": "openEuler",
      "os_version_prefix": "22.03",
      "arch": "aarch64",
      "nic_type": "1823",
      "file": "SDK_LINUX-xxx-openEuler22.03-aarch64.tar.gz",
      "sha256": "replace-with-real-sha256",
      "requires_repo": false
    }
  ]
}
```

安装驱动：

```bash
storctl validate-artifacts --artifact-dir /root/storage_pkgs
storctl install-driver --nic-type 1823 --artifact-dir /root/storage_pkgs
```

CX7 同理：

```bash
storctl install-driver --nic-type cx7 --artifact-dir /root/storage_pkgs
```

如果驱动包需要系统 repo，manifest 必须显式写 `requires_repo: true`，并且安装时要显式允许：

```bash
storctl install-driver --nic-type cx7 --artifact-dir /root/storage_pkgs --allow-repo
```

## 6. 推荐执行顺序

第一步，采集事实：

```bash
storctl facts
storctl facts --json
```

第二步，校验输入：

```bash
storctl validate-profile --profile-file /etc/storctl/profiles.json
storctl validate-artifacts --artifact-dir /root/storage_pkgs
```

第三步，预览计划：

```bash
storctl plan \
  --profile-file /etc/storctl/profiles.json \
  --profile c4 \
  --nic enp23s0f1 \
  --nic-type auto \
  --mgmt-ip 80.5.17.113
```

第四步，确认驱动：

```bash
storctl install-driver --nic-type 1823 --artifact-dir /root/storage_pkgs
```

第五步，执行接入：

```bash
storctl apply \
  --profile-file /etc/storctl/profiles.json \
  --profile c4 \
  --nic enp23s0f1 \
  --nic-type auto \
  --mgmt-ip 80.5.17.113
```

第六步，检查结果：

```bash
storctl check
storctl check --json
mount | grep -E '/mnt/share|/mnt/weight'
rdma link
```

成功的 RDMA 挂载应该能看到类似：

```text
172.27.1.1:/Share on /mnt/share type nfs4 (...,vers=4.1,proto=rdma,port=20049,...)
```

## 7. QoS 怎么用

QoS 默认关闭：

```text
SKIP qos disabled
```

只有确认交换机和存储侧策略后，再显式启用：

```bash
storctl apply ... --qos apply
```

或在 profile 里写：

```json
{
  "qos": {
    "enabled": true
  }
}
```

如果你还没有确认 PFC/ECN/DSCP，不建议打开 QoS。RDMA 网络里 QoS 参数不匹配时，问题可能比不配更难排。

## 8. TCP fallback

默认目标是 NFS-RDMA。RDMA 不通时，`apply` 会失败，不会偷偷改成 TCP。

如果业务必须先可用，可以显式允许 TCP 降级：

```bash
storctl apply ... --allow-tcp-fallback
```

这种情况下：

- RDMA driver/link 尚未 ready 时，也会继续配置网络并尝试 TCP NFS。
- 挂载会使用 TCP NFS。
- `/var/lib/storctl/state.json` 会记录 degraded 状态。
- `storctl check` 会输出 `WARN degraded tcp-fallback`。

这适合临时救急，不适合作为性能验收通过标准。

## 9. 常见失败

### FAIL nic

含义：网卡不存在，或者你传了管理口。

下一步：

```bash
ip -br addr
ip -br link
ibdev2netdev
hinicadm3 info
```

确认 `--nic` 是 200G 存储物理口，不是 SSH 管理口。

### FAIL driver

含义：驱动或工具不完整。

下一步：

```bash
storctl facts --json
storctl validate-artifacts --artifact-dir /root/storage_pkgs
storctl install-driver --nic-type <cx7|1823> --artifact-dir /root/storage_pkgs
```

### rdma link 为空

含义：当前机器没有可用 RDMA link。

下一步：

```bash
rdma link
lsmod | grep -iE 'rdma|roce|mlx5|hinic'
ibdev2netdev
hinicadm3 info
```

先解决驱动/RDMA 设备，再看 NFS-RDMA。

### mount.nfs: Protocol error

含义：客户端尝试 RDMA NFS，但服务端或网络路径不接受。

下一步：

```bash
rdma link
ping 172.27.1.1
showmount -e 172.27.1.1
mount -t nfs -o vers=4.1,proto=rdma,port=20049 172.27.1.1:/Share /mnt/share
```

同时确认服务端打开 NFS-RDMA 端口 `20049`。

### 挂成 TCP 了

检查：

```bash
findmnt -T /mnt/share -o FSTYPE,OPTIONS
nfsstat -m
```

如果看到 `proto=tcp`，说明不是 RDMA。默认情况下 `storctl apply` 会把非 RDMA 的同路径挂载卸载后重挂 RDMA。

## 10. 验收清单

一台机器算接入成功，至少满足：

- `storctl check` 没有 `FAIL`。
- `ip -br addr` 里有 `data0.<vlan>` 和正确 `172.27.x.y/18`。
- `ip rule`/`ip route show table 5000` 有对应策略路由。
- `rdma link` 非空。
- `mount` 或 `findmnt` 显示挂载为 `proto=rdma`。
- 重启后 VLAN 和挂载仍然存在。
