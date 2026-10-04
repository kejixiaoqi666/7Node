# Rust 接续入口

本轮基于 `xbord-node-v3-rust-a6255b9.tar.gz` 续接，并保留 GitHub Go 基线 `7893713344d301b07978c2c1e84d848181b20e39` 的后续修复。原始附件与 Go 源码均未被改写。

主要入口是 [原生计数持久化与停止协议](docs/RUST_NATIVE_DURABILITY_ZH.md)、[流量统计与持久队列](docs/RUST_TRAFFIC_ZH.md) 与 [迁移清单](docs/RUST_MIGRATION_ZH.md)。Rust workspace 包含 `node-runtime` 与 `node-native`，二进制名 `xboard-node-rust`。配置示例在 `examples/runtime-rust.json`。

当前默认单节点路径的控制层、VLESS/Trojan TCP、文件 TLS、认证快照、转发与用户控制均为 Rust。不填写 `singbox_executable` 时自动用自己的 Rust 二进制启动数据子进程，仅用户更新自动使用原子认证表替换；无需 Go 服务端或工具链。未实现的路由、限速和设备控制继续显式拒绝。原 Go 源码和外部内核保留为历史兼容方案，全部原版功能尚未迁完。

本地恢复先读 `.Codex/context-checkpoint.md` 与 `.Codex/project-ops/state.json`；验证原始日志保存在 `.Codex/evidence/`。干净源码包不携带 `.git`、`.Codex`、工具下载、构建缓存或真实配置/凭据。

验证命令：

```bash
cargo fmt --all -- --check
cargo test --workspace --locked
cargo clippy --workspace --all-targets --all-features --locked -- -D warnings
cargo build -p node-runtime --release --locked
```

旧外部方式的真实客户端测试需显式提供 `SINGBOX_TEST_BINARY` 后执行；它不证明新 Rust 服务端。新路径的协议/TLS/热更新/故障验收由 `benchmarks/native_users_lab.py --builtin` 与 `benchmarks/rust_fd_lab.py` 单独完成，原生场景检查运行数据层是同一个 Rust ELF。

```bash
cargo test -p node-runtime --test real_singbox --locked -- --ignored --nocapture
```

## 当前原生持久化片段的验证

最终 nd3 候选绑定 60 个输入，清单 SHA256 为 `9d9a55cf57b9187e9b92fae71e22282f84552a134af766cbf3b0b4217a980aa2`。Linux ARM64 GNU 程序为 4,462,520 字节，SHA256 为 `c72d2f8cbe9c8eff500ae2268144958502c83f43e3bf968f6349a23da9417a51`。Windows 104 项、Linux 113 项 workspace 测试、fmt、严格 clippy 与 Linux 显式真实外部 VLESS 回归通过。

新增原生周期存档、持久 epoch/冻结快照/ACK、单写者重启和成功确认后才排空的停止协议。五个新增实际连接案例、七个原流量案例、原生三协议/热更新/控制故障/lifecycle 和两轮子进程 64FD 耗尽均通过。慢磁盘任务共用两个 owned 作业配额，取消后仍计入配额直到实际结束；单线程取消和跨轮容量回归通过。

精确绑定及边界见 [当前结果](benchmarks/results/rust-native-durable-20261002.json) 与 [详细说明](docs/RUST_NATIVE_DURABILITY_ZH.md)。保护已经提交的计数；默认一秒并非强杀丢失窗口上限。前两项强杀案例观察原子替换后的文件，不证明目录 fsync 已返回或断电持久性；冻结 RPC/停止成功回复才是相应完整提交屏障。未知上报仍须人工核对，未替换生产或写真实面板。

## 前一流量片段的验证

最终 acct4 候选绑定 59 个产品输入；Linux ARM64 GNU 程序为 4,331,448 字节。Windows 99 项、Linux 106 项 workspace 测试通过，fmt 和严格 clippy 通过；被 workspace 忽略的真实外部 VLESS 集成测试在 Linux 另行通过。

七个流量用例完整通过：VLESS/VLESS TLS/Trojan TLS 逐用户上传下载对账、长连接采集、删除用户后旧连接归属、丢确认/取消/强杀后的持久队列处理，以及确认未送出后的重试。前序三协议各 64 条连接保持、2 MiB 跨更新内容、四类控制故障、256 连接/1024 次传输、恢复与退出清理、两轮仅测试子进程 64FD 耗尽也重新通过。资源对照时关闭本次统计，不能用对照数据声称统计开启后的 CPU/RSS 下降。

精确版本、执行脚本、测试数、原夹具失败与修正记录见 [本版本结果](benchmarks/results/rust-traffic-20261002.json)。接口确认只代表接受请求；实际计费数据库与尚未采样字节的无损保护仍属于后续工作。隔离测试未替换生产服务或写真实面板。

## 之前各片段的验证记录

下面是历史版本记录，二进制/源码清单和测试数只对对应片段成立，不能替代当前 Rust 数据层验收。

2026-10-02 已按用户要求，在美国服务器 `159.195.12.237` 的 `/opt/xbord-rust-lab/20261002-build-01` 构建最初 Linux ARM64 控制层版本，完成 68 项 workspace 测试、1 项真实 VLESS 集成测试和 9 项可执行程序检查。结果和使用边界见 [docs/RUST_US_TEST_ZH.md](docs/RUST_US_TEST_ZH.md)。测试使用隔离 HTTPS 面板夹具及回环监听，测试进程已退出，构建目录保留。

本轮另完成控制层性能与稳定性优化：二进制 6,623,160 → 3,676,088 字节，控制线程 8 → 2，一万用户双 304 同步累计分配和耗时约降 76%；一万用户控制程序常驻 RSS 中位数 32.64 → 29.21 MiB。71 项测试、真实 VLESS、256 条连接、WSS 推送/重连、HTTP 卡住时退出与请求中途内核崩溃恢复均有直接证据。详细范围、每轮数据和 TCP 吞吐未证明提升的结论见 [docs/RUST_PERFORMANCE_ZH.md](docs/RUST_PERFORMANCE_ZH.md)，复现脚本在 `benchmarks/`。

同日的新一轮 [Linux 复验](docs/RUST_LINUX_RECHECK_ZH.md) 保持运行源码与二进制不变，重新通过 71 项测试、真实 VLESS 和 9 项 CLI 验收，并补测每版 90 秒资源、256 连接保持 10 秒、1024 次传输和 6 次用户更新。配置构建临时堆峰值为一万用户 20.51 MiB、两万用户 41.05 MiB；实际更新 RSS 峰值约 58.30 MiB。更新时保持的 64 条连接全部中断，后续优先减少配置复制和改善更新连接连续性。首轮更新探针的误判与独立目录修正结果均保留。

最新流式配置片段已完成并在同一台 Linux 主机配对验证：一万用户控制 RSS 29.70 → 6.25 MiB；启动及 6 次一万/两万用户更新后的 VmHWM 58.28 → 12.62 MiB，约降 78.3%；二进制增加 65,536 B 至 3,741,624 B。本地/Linux 各 76 项测试、真实 VLESS、9 项 CLI、108 组有效输出和 19 组错误对照、256 连接/1024 次传输、VLESS/Trojan TLS 均通过。最终 45 个 Rust/build 文件与独立静态审查及构建清单绑定。详见 [配置内存优化报告](docs/RUST_MEMORY_OPTIMIZATION_ZH.md) 和 [机器可读结果](benchmarks/results/rust-memory-20261002.json)。

同日新增可选用户热更新片段已通过：VLESS、VLESS TLS、Trojan TLS 各 64 条既有连接完整返回，2 MiB 流跨首个发布点且内容一致；默认模式对照仍 0/64。稳定用户身份、不可变认证快照、私有有界控制与丢确认摘要查询已实现。Windows 78 项/Linux 81 项测试、真实 VLESS、7 项 Go 单测/race、四类控制故障、256 连接/1024 次传输及恢复回收有对应证据。实际产品输入绑定 63 文件清单；详情见 [用户热更新报告](docs/RUST_NATIVE_USERS_ZH.md) 和 [机器可读结果](benchmarks/results/rust-native-users-20261002.json)。

上述历史热更新方案的源码/构建入口为原仓库 `kernels/native-users/`，它是外部 Go 可执行文件，按 GPL-3.0-or-later 及保留上游条款提供，不链接进新 Rust 数据层。新默认配置 `examples/runtime-rust-native.json` 已不再填写它。新 Rust 代码继续 MPL-2.0。当前 Linux ARM64 GNU 程序需 glibc 2.39+ 和 libgcc_s；历史对应源码包单独保留。

当前已补齐原生计数存档与确认停止协议；尚未落盘的强杀尾部和实际面板计费继续保留边界。后续优先限制执行、真实面板对账、其他协议/路由兼容、多节点与控制配置持久恢复。原生更新只覆盖用户变更，删除用户不强制断开已认证连接；非用户变更/未知状态恢复会中断连接。TCP 极限需单独验证数据面。没有生产替换、真实面板写入、Git 提交或推送；夹具验证不等于线上功能验收。
