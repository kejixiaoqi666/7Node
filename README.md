# xboard-node

## Rust 重构（实验）

已把交付包中的 Rust 模块接成独立的 `xboard-node-rust` 单节点程序。**当前默认路径的控制层和协议数据层均为 Rust**：REST 同步、WS 重同步提示、VLESS TCP、VLESS/Trojan 文件 TLS、TCP 转发、用户热更新及失败恢复包含在同一个二进制中，无需 Go 服务端或 Go 工具链。控制/数据两个 Rust 进程用于隔离故障。

仅用户变更可以原子替换认证表，既有连接继续使用认证时的稳定身份；删除用户阻止新连接，已认证连接可继续。监听/TLS 等变更仍完整重启。按用户统计实际转发载荷，原生计数周期存档，冻结快照和控制器队列先落盘再确认；结果不确定的上报批次暂停重发。正常退出先结束载荷任务，再采集最后金额。尚未落盘的强杀尾部、生产账单对账、限速、设备控制、REALITY 等其他协议和多节点继续迁移。

详见 [原生计数持久化与停止协议](docs/RUST_NATIVE_DURABILITY_ZH.md)、[流量统计与可靠上报](docs/RUST_TRAFFIC_ZH.md)、[Rust 重构进度](docs/RUST_MIGRATION_ZH.md) 和 [Rust 交接入口](HANDOFF_RUST.md)。默认配置在 `examples/runtime-rust.json`，不填写 `singbox_executable` 即使用内置 Rust 数据层。原 Go 源码、安装方式和此前外部 Go 过渡方案保留在仓库中；下文原版功能表不代表 Rust 已全部实现。新 Rust 代码沿用 MPL-2.0。

2026-10-02 的原生持久化候选 nd3 已通过 Windows 104 项、Linux ARM64 113 项测试及 fmt/严格 clippy；五个新增恢复/停止案例、七个原流量案例，以及前序协议/热更新/故障/连接/FD 回归均通过。同一最终 Rust ELF 为 4,462,520 字节，需 glibc 2.39+ 与 libgcc_s，较 acct4 增加 128 KiB；详细数据和范围见 [本版本机器可读结果](benchmarks/results/rust-native-durable-20261002.json)。默认存档间隔一秒，不能消除尚未提交的强杀尾部；进程崩溃测试不等于断电或真实面板计费验收。

Node backend for [Xboard](https://github.com/cedar2025/Xboard). Supports `sing-box` / `xray-core` dual kernels.

> **Disclaimer**: This project is for educational and learning purposes only.

## Features

- Protocols: V2Ray family, Trojan, Shadowsocks, Hysteria2, TUIC, AnyTLS
- Sync: WebSocket push + REST polling dual channel
- User controls: speed limit, device limit, alive-IP tracking, hot update
- Deploy modes: node mode, machine mode, standalone mode
- Multi-instance: single process binding multiple panels / nodes

## 本仓库定制版安装

已提供交互式 `install-custom.sh`，无需 VPS 编译。请先阅读 [Debian 13 一键安装及实测记录](docs/DEPLOYMENT_ZH.md)。下方上游安装方式不包含本仓库 AnyTLS + REALITY 修改。2026-09-29 已修复主站 WebSocket 地址/代理和设备列表序列化问题，连接、推送及自动重连已实测；REST 保留，完整同步边界验收仍待完成。

## VPS 网络调优

[一键 BBR 与高吞吐 TCP 配置](docs/NETWORK_TUNING_ZH.md)：独立于节点安装，可重复执行并回退。

## Install

### Docker

```bash
docker run -d --restart=always --network=host \
  -e apiHost=https://panel.com -e apiKey=TOKEN -e nodeID=1 \
  ghcr.io/cedar2025/xboard-node:latest
```

### Docker Compose

```bash
git clone -b compose --depth 1 https://github.com/cedar2025/xboard-node.git
cd xboard-node
vim config/config.yml   # set panel.url / token / node_id
docker compose up -d
```

### Installer (Linux systemd)

```bash
# Node mode
curl -fsSL https://raw.githubusercontent.com/cedar2025/xboard-node/dev/install.sh | \
  sudo bash -s -- --mode node --panel https://panel.example.com --token TOKEN --node-id 1

# Machine mode
curl -fsSL https://raw.githubusercontent.com/cedar2025/xboard-node/dev/install.sh | \
  sudo bash -s -- --mode machine --panel https://panel.example.com --token TOKEN --machine-id 1
```

## xbctl

Run `xbctl` after installation for help. Common commands:

```bash
xbctl list                          # list all instances
xbctl status                        # running status
xbctl bind add-node --panel URL --token TOKEN --node-id 1
xbctl bind add-machine --panel URL --token TOKEN --machine-id 1
xbctl bind remove-node --panel URL --node-id 1
xbctl service restart
```

## Configuration

Legacy single-panel config is fully compatible. Appending bindings auto-migrates to `instances` format. See `config.yml.example`.

## Extensions

2026-10-01：完成标准 TUIC v5 + TLS 的实机、订阅及 UDP 转发验证，修复首次安装时
未写入证书路径的问题。见 [TUIC 部署验证](docs/TUIC_DEPLOYMENT_ZH.md)。

2026-10-01：修复出站、DNS 和路由依赖的自动更新。已有标准安装可使用
`upgrade-custom.sh`，详见 [热更新实测与旧版升级](docs/HOT_RELOAD_ZH.md)。

2026-09-30：定制 sing-box 面板路由/DNS 修复已通过两个真实节点测试，安装器已更新发行包。
见 [路由验证与回退记录](docs/ROUTING_VERIFICATION_ZH.md)。

- Custom routes: [docs-custom-routes.md](docs-custom-routes.md)
- Custom outbounds: [docs-custom-outbounds.md](docs-custom-outbounds.md)
- DNS providers (ACME DNS-01): [docs-dns-providers.md](docs-dns-providers.md)

## License

MPL-2.0.
