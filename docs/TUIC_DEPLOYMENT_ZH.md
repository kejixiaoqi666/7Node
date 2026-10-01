# TUIC v5 实机部署验证（2026-10-01）

已在独立 Debian 13 / amd64 VPS 部署标准 TUIC v5 + TLS，使用发行包
`custom-hotreload-20261001`。当前定制内核已有 TUIC 支持，本次没有改造 TUIC 协议，
没有新增 REALITY 或混淆功能，也不需要为本次测试重新发布内核二进制。

## 配置与隔离

- 独立面板机器、权限分组、测试账号及 TUIC 节点；监听 UDP 10443。
- TUIC v5，ALPN `h3`，拥塞控制 `bbr`，UDP relay `native`。
- 使用域名申请的公开可信 TLS 证书，客户端不跳过证书校验。
- 测试账号配额 10 GiB、有效期 7 天，不自动开放给其他用户。
- 715 MiB 内存测试机设置 `GOMEMLIMIT=256MiB`、`GOGC=50`，保留既有 Swap。
- Certbot 自动续期及成功续期后重启节点服务；HTTP-01 续期需要 TCP 80 可达。

## 实测结果

1. 节点从面板加载一名隔离测试用户，WebSocket mux 启动，流量上报成功。
2. 面板生成的完整 sing-box 和 FlClash 订阅均通过对应内核配置检查。
3. 提取订阅中的真实 TUIC 出站，使用项目配套 sing-box 客户端及 Mihomo v1.19.32
   分别代理访问 HTTPS，出口 IP 与 VPS 一致；均开启证书校验。
4. sing-box 客户端通过 TUIC 转发 SOCKS5 UDP DNS 查询并收到正确响应。
5. Certbot 模拟续期通过。
6. 本地新增 `TestTUICStandardTLSRoundTrip`，使用真实 QUIC/TUIC 客户端和服务端，
   验证约 120 KiB 数据回传一致。本地自签名证书仅用于隔离测试。

这些结果不代表已经在全部手机、客户端版本或移动运营商网络中测试。
FlClash 部分验证的是其订阅格式及 Mihomo 内核，不是手机应用 UI 实测。

## 安装脚本修复

发现 `install-custom.sh` 原来虽然能申请证书，却没有把证书路径写入配置，
导致首次安装并关联 TLS 节点后报缺少证书。现已修复：选择申请证书时写入
`cert.cert_mode=file`、`cert.cert_file` 和 `cert.key_file`；跳过申请时保持不设置证书。
脚本仍在证书成功签发之后才安装并启动服务。

回归测试：

```bash
python3 tests/install_cert_config_test.py
go test -tags with_quic ./internal/kernel/singbox -run TestTUICStandardTLSRoundTrip
```

## REALITY、ECH 与混淆边界

当前内核不能把现有 REALITY 握手直接用于 TUIC 的 QUIC 连接。面板添加 REALITY
字段不会产生可用的 TUIC + REALITY 协议。ECH 或自定义数据包混淆应单独验证客户端、
服务端和订阅的兼容性；本次没有部署它们。不得将此节点标记为 TUIC + REALITY。

参考：[TUIC 配置](https://sing-box.sagernet.org/configuration/inbound/tuic/)、
[QUIC 自定义 TLS 支持](https://sing-box.sagernet.org/configuration/shared/tls/#custom-tls-support)。
