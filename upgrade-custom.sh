#!/usr/bin/env bash
# Upgrade the standard systemd installation; preserve configuration/certificates.
set -Eeuo pipefail
umask 077
RELEASE=custom-hotreload-20261001
ARCHIVE_SHA=0cffb212f033b54eca40aee9b5f0f15cee728af83636cf90d624b81ec282e45d
BINARY_SHA=58ce7b14e5e2d23b6cd2765c2ac1771b1f247e4e7e2df1ea8e61a0d1bcbcdcda
UNIT=xboard-node-custom.service
BIN=/opt/xboard-node-custom/xboard-node
CONF=/etc/xboard-node-custom/config.yml
fail() { printf '错误：%s\n' "$*" >&2; exit 1; }
if [[ ${1:-} == --help ]]; then
    echo '升级已有 xboard-node-custom（Linux amd64）；备份程序和配置，校验发行包，重启服务。'
    exit 0
fi
[[ $# == 0 ]] || fail '只支持无参数升级或 --help'
[[ $EUID == 0 && $(uname -m) == x86_64 ]] || fail '需要 root 和 x86_64/amd64'
[[ -f $BIN && -f $CONF ]] || fail '未找到标准安装；不适用于 Docker 或其他服务名'
for tool in curl gzip sha256sum flock systemctl; do command -v "$tool" >/dev/null || fail "缺少 $tool"; done
exec 9>/opt/xboard-node-custom/.upgrade.lock
flock -n 9 || fail '已有升级正在执行'
systemctl is-active --quiet "$UNIT" || fail '原服务未运行，请先排查'
BACKUP=$(mktemp -d /root/xboard-node-upgrade-XXXXXXXX)
cp -p "$BIN" "$BACKUP/xboard-node"
cp -p "$CONF" "$BACKUP/config.yml"
systemctl cat "$UNIT" > "$BACKUP/service.txt"
printf '备份：%s\n' "$BACKUP"
curl -fL --retry 3 --connect-timeout 15 --max-time 600 \
    "https://github.com/xiaofujie369/xbord-node-v3/releases/download/$RELEASE/xboard-node-linux-amd64.gz" \
    -o "$BACKUP/update.gz"
echo "$ARCHIVE_SHA  $BACKUP/update.gz" | sha256sum -c -
gzip -dc "$BACKUP/update.gz" > "$BACKUP/update"
echo "$BINARY_SHA  $BACKUP/update" | sha256sum -c -
pending=0
finish() {
    result=$?
    trap - EXIT
    if [[ $pending == 1 ]]; then
        echo '升级未完成，正在回退旧程序……' >&2
        if cp "$BACKUP/xboard-node" "${BIN}.rollback" && chmod 755 "${BIN}.rollback" && \
           mv -f "${BIN}.rollback" "$BIN" && systemctl restart "$UNIT"; then
            echo '已恢复旧程序。' >&2
        else
            printf '自动回退失败，请使用备份手动恢复：%s\n' "$BACKUP" >&2
        fi
        result=1
    fi
    exit "$result"
}
trap finish EXIT
trap 'exit 130' INT TERM
pending=1
install -m 755 "$BACKUP/update" "${BIN}.new"
mv -f "${BIN}.new" "$BIN"
systemctl restart "$UNIT"
sleep 10
systemctl is-active --quiet "$UNIT"
pending=0
printf '已升级到 %s，服务运行中。备份：%s\n' "$RELEASE" "$BACKUP"
echo '请检查节点启动日志，并在客户端测试连接；服务运行不代表每个节点均已成功启动。'
