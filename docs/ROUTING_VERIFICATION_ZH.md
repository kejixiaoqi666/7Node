# 面板路由修复与实测（2026-09-30）

本次修复默认 sing-box 内核的面板 `routes` 转换。旧程序将带 `/` 的整行注释识别为
CIDR，并将 DNS 地址当作代理出站标签，导致配置初始化失败或新规则无法应用。
新版解析注释、域名、CIDR、通配符和规则集引用，分别生成流量规则与 DNS 规则。
无效条目告警并跳过，不能把空规则意外扩大为全局匹配。

## 已完成验证

- 全部 `internal/...`、`cmd/...` 测试通过，使用发行构建标签
  `with_quic with_utls with_wireguard with_acme with_clash_api`。
- sing-box 包 `go vet` 通过；新增真实 UDP DNS/HTTP 内核集成测试验证特定与默认 DNS
  的选择、直连、代理、阻断；补充本地合并、规则集标签和 Hysteria 入站隔离测试。
- 用户 VPS 上关联的两个节点：VLESS REALITY TCP 2088、Hysteria2 TLS + Salamander
  UDP 2443。每节点加载 742 名用户，服务启动后无自动重启，WS mux 启动。
- 分别使用外部原生客户端测试，两个协议访问 `api.ipify.org` 和 `www.taobao.com`
  均返回 HTTP 200；出口地址与 VPS 一致。
- 分别抓包观察服务端 DNS：`www.taobao.com` 的 A/AAAA 查询发往 `223.5.5.5`，
  `api.ipify.org` 发往 `1.1.1.1`，未在另一组解析器观察到这些测试查询。
- 阻断规则中的 `www.gov.cn` 在 VPS 直接访问返回 HTTP 200，经两个代理节点均无法
  建立 HTTPS 连接；本地内核测试另外验证了明确的拒绝规则。

未声称完成所有客户端、地区网络、DNS 故障切换或 WS 所有更新边界验收。
本次未修改面板下发的规则内容、用户限制、证书或其他服务。

## 现有规则的限制

现场每节点有三组规则：阻断、国内 DNS、国际 DNS。逗号分隔的多个解析器会导入，
实际使用第一个有效地址，不是自动故障切换。裸 `geosite-cn` / `geoip-cn` 等名称
不是可用的规则集，已明确告警并跳过；需要配置原生 `route.rule_set`，再用
`rule-set:标签` 引用。其余有效域名规则仍正常工作。详见
[路由配置说明](../docs-custom-routes.md)。

## 备份与版本

部署前已在目标 VPS 的 `/root/codex-route-backup-20260929-182627` 备份旧二进制、
本地配置、systemd unit 和现场节点配置。目录仅 root 可读，其中包含敏感配置，
没有提交到仓库。目录日期为 VPS 的 UTC 时间。部署使用同目录临时文件原子替换，
启动失败或两个监听端口缺失时会恢复旧二进制。

- 发行版本：`custom-routes-20260930`
- gzip SHA-256：`f9a0db349f67011e664b57b922655612784b709f63c9498b52c290e6f8cae901`
- 二进制 SHA-256：`bc7d56a087cf3f202be596674aa707d6ef92ac093ac7949f6f9721a538fe181f`

只在本次已升级的 VPS 上，需要回退程序时执行：

```bash
set -e
cp /root/codex-route-backup-20260929-182627/xboard-node /opt/xboard-node-custom/xboard-node.rollback
chmod 755 /opt/xboard-node-custom/xboard-node.rollback
mv /opt/xboard-node-custom/xboard-node.rollback /opt/xboard-node-custom/xboard-node
systemctl restart xboard-node-custom
```

回退会恢复旧版本的路由缺陷；本次没有变更本地配置，不需要覆盖现有配置或证书。
