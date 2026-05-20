# storctl 离线驱动矩阵草案

这个文件是团队 wiki 的初稿。建议把最终版本放到内部 wiki，由人维护 OS、网卡、驱动包和注意事项；`storctl` 只读取 `/root/storage_pkgs/storctl-artifacts.json` 做机器校验。

## 推荐取舍

- 默认不维护跨实验室公网仓库。每个实验室通过 Ansible/scp 预分发 `storage_pkgs/`。
- 优先选择真离线包：CX7 用 `MLNX_OFED_LINUX-*.tgz` 或 `IB_NIC-*.tgz`，1823 用 `nic_1823*.tar.gz` 或 `hinic*.tar.gz`。
- `doca-host*.rpm` 是 repo installer，不是真离线包。只有实验室已经准备好 dnf repo 时，才在 manifest 里标记 `requires_repo: true`，并运行 `storctl install-driver --allow-repo`。
- `apply` 不安装驱动。驱动没就绪时失败，并提示先运行 `storctl install-driver`。
- `--nic` 必须由 inventory 或人工显式指定。`storctl` 不做 `--nic auto`，避免在双口 200G 机器上猜错业务口。

## 目录结构

```text
/root/storage_pkgs/
  storctl-artifacts.json
  MLNX_OFED_LINUX-5.8-1.1.2.1-openeuler22.03-aarch64.tgz
  nic_1823-openeuler22.03-aarch64.tar.gz
```

生成 sha256：

```bash
cd /root/storage_pkgs
sha256sum *.tgz *.tar.gz *.rpm
```

## Manifest 字段

```json
{
  "artifacts": [
    {
      "os_id": "openEuler",
      "os_version_prefix": "22.03",
      "arch": "aarch64",
      "nic_type": "cx7",
      "file": "MLNX_OFED_LINUX-5.8-1.1.2.1-openeuler22.03-aarch64.tgz",
      "sha256": "replace-with-sha256",
      "requires_repo": false
    }
  ]
}
```

- `os_id` 来自 `/etc/os-release` 的 `ID`。
- `os_version_prefix` 是前缀匹配，比如 `22.03` 可匹配 `22.03-LTS-SP4`。
- `arch` 在 Linux aarch64 机器上应写 `aarch64`。
- `nic_type` 当前支持 `cx7` 和 `1823`。
- `requires_repo` 为 `true` 表示安装时可能调用 dnf repo，默认会被拒绝，除非显式传 `--allow-repo`。

## 当前矩阵

| OS | Arch | NIC | Artifact | Repo required | 状态 |
| --- | --- | --- | --- | --- | --- |
| openEuler 22.03 | aarch64 | CX7 | `MLNX_OFED_LINUX-*.tgz` 或 `IB_NIC-*.tgz` | 否 | 推荐 |
| openEuler 22.03 | aarch64 | 1823 | `nic_1823*.tar.gz` 或 `hinic*.tar.gz` | 否 | 推荐 |
| openEuler 23.x | aarch64 | CX7 | 待填 | 否 | 待验证 |
| openEuler 23.x | aarch64 | 1823 | 待填 | 否 | 待验证 |
| openEuler 24.03 | aarch64 | CX7 | `doca-host*.rpm` 或对应 MLNX/IB 离线包 | 视包而定 | 推荐优先找真离线包 |
| openEuler 24.03 | aarch64 | 1823 | 待填 | 否 | 待验证 |

## 批量接入流程

```bash
ansible all -m copy -a "src=storctl-linux-arm64 dest=/usr/local/bin/storctl mode=0755"
ansible all -m copy -a "src=storage_pkgs/ dest=/root/storage_pkgs/"
ansible all -m copy -a "src=storctl-profiles.json dest=/etc/storctl/profiles.json"
ansible all -m shell -a "storctl install-driver --nic-type {{ nic_type }} --artifact-dir /root/storage_pkgs"
ansible all -m shell -a "storctl plan --profile {{ storage_profile }} --nic {{ storage_nic }} --mgmt-ip {{ ansible_host }}"
ansible all -m shell -a "storctl apply --profile {{ storage_profile }} --nic {{ storage_nic }} --mgmt-ip {{ ansible_host }}"
```

## TCP 降级策略

默认不降级。RDMA 不可用时，`storctl apply` 失败并提示检查 `rdma link`、驱动和服务端 `20049` 端口。

临时保业务时可以显式传：

```bash
storctl apply ... --allow-tcp-fallback
```

这会持久化 TCP NFS，并在 `/var/lib/storctl/state.json` 写入 `degraded: true`。后续 `storctl check` 会继续提示降级状态。
