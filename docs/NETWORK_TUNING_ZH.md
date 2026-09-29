# 一键高吞吐 TCP 配置

适用于有 root 权限、支持 BBR 的 Linux VPS。无需安装节点程序，也不依赖 CPU 架构。

Debian / Ubuntu：

```bash
apt-get update && apt-get install -y ca-certificates curl python3 procps kmod
curl -fL https://raw.githubusercontent.com/xiaofujie369/xbord-node-v3/main/tune-network.sh -o /root/tune-network.sh && bash /root/tune-network.sh
```

启用内核提供的 BBR，默认队列设置为 fq；启用 TCP 接收缓冲自动调节、窗口缩放、SACK 和按需 MTU 黑洞探测。内存至少 768 MiB 时 TCP 缓冲上限目标为 64 MiB，384–767 MiB 为 32 MiB；已有更高上限不降低。保留缓冲初始值以及全局 TCP 内存压力控制，不预分配每连接最大值。大量并发连接仍可能增加内存压力。

保存首次执行前的参数到 `/var/lib/xboard-net-tune/original.json`，可重复执行。持久配置为 `/etc/sysctl.d/99-zz-xboard-performance.conf`。后续其他配置管理工具或 `/etc/sysctl.conf` 可能覆盖参数，重启后可以用下列命令核验。

```bash
bash /root/tune-network.sh --status
bash /root/tune-network.sh --rollback
```

脚本不更换内核，不关闭防火墙，不删除 tc 限速/整形，不重启网络或节点。设置 default_qdisc 不会改写当前运行中的网卡队列；需结合输出判断是否已有 fq。新 TCP 连接才采用新的默认拥塞算法，重新发起下载即可。

参数调大不是带宽解锁。下载速度仍受 VPS 套餐、线路拥塞/丢包、CPU、客户端和下载源影响。BBR 针对内核 TCP，不直接改变 Hysteria2 的 QUIC 拥塞控制。1000 Mbps 理论为 125 MB/s，应用有效吞吐通常低于线路速率。

依据：[Linux 内核 TCP 参数文档](https://docs.kernel.org/networking/ip-sysctl.html)。这是高吞吐配置，不是对所有 VPS 都能提速的承诺。
