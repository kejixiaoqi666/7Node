# 出站与 DNS 自动更新修复

旧版 `Reload` 只替换路由和入站，没有更新出站及 DNS 实例。面板新增代理出站后，
路由会引用一个运行中不存在的标签，出现 `outbound not found`；修改解析器也可能继续
使用旧实例。配置已被收到并不代表完整应用成功。

新版在有效的出站、DNS、路由、规则集等配置改变时，自动重建**该节点**的内核。
整个 Xboard-Node 服务进程和其他节点继续运行。用户更新仍使用原有增量接口。
该节点原有连接可能短暂断开重连，不承诺已有连接无缝迁移。

重建保留流量统计对象、用户映射及限速/设备限制回调。缺失出站引用会在修改运行实例
之前拒绝；新配置启动失败时尝试恢复最后成功配置及当前用户。失败会记录日志，
不会再把路由更新错误当作成功。

## 本地回归验证

- 真实 HTTP 流量验证直连 → 新增代理 → 修改代理地址 → 删除代理恢复直连。
- 新出站和路由同时更新后可用，不需要 `systemctl restart`。
- 删除仍被路由引用的出站时拒绝新配置，原连接能力保留。
- 替换实例遇到监听端口占用时恢复旧监听和旧出口。
- 修改节点期间，另一节点保持原实例并可正常访问。
- 更换 DNS 服务端后实际查询新解析器，原解析器及缓存不再沿用。
- 配置未改变时不重建内核，流量统计对象在重建与回退时保留。

运行：`go test ./internal/kernel/singbox -run TestRuntimeReload`。

完整发行标签测试、`go vet` 及上述回归测试的 `-race` 检查均通过。

## 面板下发实测（2026-10-01）

在已有 AnyTLS TCP 443 和 Hysteria2 UDP 8443 的 VPS 上部署后，通过面板数据模型保存
触发原有配置通知，依次完成新增临时代理出站、修改该出站连接超时、删除临时出站
并恢复原配置。三次均自动加载，服务 PID 保持不变，没有手动重启服务。
每一步均通过外部客户端验证：443 保持本机出口，8443 使用指定 VLESS 上游出口。
没有出现 `outbound not found`，测试结束后面板已恢复原配置。

本次程序升级本身需要一次服务重启；升级后的出站变更不再需要手动重启。
VPS 原程序与配置备份在 `/root/codex-hotreload-backup-20261001`。

## 已安装旧版升级

适用于 `xboard-node-custom.service` 标准安装、Linux amd64，root 执行：

```bash
curl -fL https://raw.githubusercontent.com/xiaofujie369/xbord-node-v3/main/upgrade-custom.sh -o /root/upgrade-xboard-custom.sh && bash /root/upgrade-xboard-custom.sh
```

脚本备份原程序、配置及 unit，校验压缩包和二进制 SHA-256，只替换程序，
保留 Token、节点绑定和证书。服务启动失败时回退原程序。
仍需检查每个节点日志和客户端连接，服务 active 不保证所有节点启动成功。
不适用于 Docker、ARM64 或其他服务名。成功、摘要失败拒绝更新、启动失败回退均已
通过隔离模拟测试。

发行包：`custom-hotreload-20261001`。
二进制 SHA-256：`58ce7b14e5e2d23b6cd2765c2ac1771b1f247e4e7e2df1ea8e61a0d1bcbcdcda`。
压缩包 SHA-256：`0cffb212f033b54eca40aee9b5f0f15cee728af83636cf90d624b81ec282e45d`。
