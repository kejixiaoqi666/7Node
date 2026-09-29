# Custom Routes

## Xboard 面板路由（定制 sing-box 内核）

面板关联节点的 `routes` 支持 `block` / `reject`、`direct`、`proxy`、`dns`。
规则按面板返回顺序执行，只作用于关联节点的入站。此增强针对本项目的
sing-box 内核，不表示 Xray 内核也实现了相同的 DNS 转换。

- 匹配内容可以换行，忽略空行和以 `#`、`//`、`;` 开头的整行注释。
- 普通域名、`.example.com`、`*.example.com` 匹配域名及子域；支持
  `domain:`、`full:`、`keyword:`、`regexp:`、IPv4/IPv6 地址或 CIDR。
- 不同匹配类型之间为“或”。`*` / `*.*` 表示该节点所有目标。
- `proxy` 的 `action_value` 为已定义出站标签；在本地 `kernel.custom_outbound`
  或 `kernel.custom_config` 的 sing-box 原生 `outbounds` 中定义。
  旧项目的 `routes.json` 不会自动导入，也不支持直接照搬 Xray `vnext` 结构。
- `dns` 的 `action_value` 可填 `223.5.5.5,119.29.29.29`，也支持 IPv6、
  `local`、`udp://`、`tcp://`、`tls://`、`https://`、`quic://`。
  IPv6 非默认端口使用 `udp://[IPv6]:5353`。
  多地址均导入，使用第一个有效地址，没有自动故障切换或并发竞速。
- DNS `*`、`*.*`、`0.0.0.0/0`、`::/0` 为默认 DNS 匹配；具体域名优先于默认，
  面板 DNS 优先于本地 DNS，缓存按解析器隔离。
- DNS 规则控制服务端连接前的域名解析；不劫持客户端发往其他 DNS 的流量。
  客户端只提交 IP 时，无法根据缺失的域名进行 DNS 分流。
- `rule-set:标签` 引用 `kernel.custom_config` 的 `route.rule_set` 中定义的规则集。
  `geosite:cn`、`geoip:cn`、裸 `geosite-cn` 等不会自动下载数据库，会告警并跳过。
  流量规则支持 `geoip:private`。

无效匹配、无效 DNS 地址、未知动作、缺失代理标签或规则集会告警并跳过。
全部匹配无效时不会生成全局规则。**跳过的规则没有生效，包括阻断规则**，
应检查服务日志修正配置。原有私网保护仍优先于面板流量规则。
存在面板 DNS 时先解析，再检查私网保护与面板流量规则；解析失败也会阻止连接。

测试：`go test ./internal/kernel/singbox -run TestPanel` 包含真实 UDP DNS、
HTTP 直连、代理和阻断测试。可设置 `XBOARD_TEST_ROUTES_FILE` 为仅包含 `routes`
数组的私有 JSON 文件，检查现场规则能否在嵌入式内核启动；不要提交现场配置或密钥。

## Quick Example

```json
{
  "custom_route_rules": [
    {
      "name": "direct-example",
      "match": {"domain_suffixes": ["example.com"]},
      "action": {"type": "direct"}
    },
    {
      "name": "block-ads",
      "match": {"domains": ["ads.example.com"]},
      "action": {"type": "block"}
    },
    {
      "name": "route-warp",
      "match": {"ip_cidrs": ["1.1.1.0/24"], "ports": ["80", "443"]},
      "action": {"type": "route", "target": "warp-out"}
    }
  ]
}
```

## Match Conditions

| Condition | Description | Example |
|-----------|-------------|---------|
| `domains` | Exact domain match | `["api.example.com"]` |
| `domain_suffixes` | Suffix match | `["example.com"]` |
| `ip_cidrs` | IP CIDR ranges | `["10.0.0.0/8"]` |
| `ports` | Port (single or range) | `["443", "8000-9000"]` |
| `networks` | Protocol | `["tcp"]` or `["udp"]` |
| `source_cidrs` | Source IP CIDR | `["192.168.1.0/24"]` |
| `source_ports` | Source port | `["1024-65535"]` |

## Action Types

| Action | Description |
|--------|-------------|
| `{"type": "direct"}` | Direct connection, bypass proxy |
| `{"type": "block"}` | Block connection |
| `{"type": "route", "target": "tag"}` | Route to specified outbound (by tag) |

## Application Order

Without panel routes, existing structured/raw custom routing precedence is unchanged.
With panel routes on the custom sing-box kernel:

1. Panel DNS resolution (when configured)
2. Built-in private-address protection
3. Panel `routes`, in panel response order
4. Local/custom routes in their existing relative order

Xray retains its existing routing order; the sing-box DNS behavior above does not apply to Xray.

## Kernel Compatibility

| Feature | Xray | Sing-box |
|---------|------|----------|
| All match conditions | ✅ | ✅ |
| direct / block / route | ✅ | ✅ |

## Best Practices

- **Prefer** `custom_route_rules`: cross-kernel compatible, panel-managed
- **Use** `custom_routes` only: when native features are needed (e.g., load balancing)
