#!/usr/bin/env bash
# Fresh Debian/Ubuntu amd64 installs only. Uses the live-tested custom binary.
set -Eeuo pipefail
umask 077
RELEASE=custom-hotreload-20261001
ARCHIVE_SHA=0cffb212f033b54eca40aee9b5f0f15cee728af83636cf90d624b81ec282e45d
BINARY_SHA=58ce7b14e5e2d23b6cd2765c2ac1771b1f247e4e7e2df1ea8e61a0d1bcbcdcda
BASE=https://github.com/xiaofujie369/xbord-node-v3/releases/download
UNIT=xboard-node-custom.service
fail() { printf '错误：%s\n' "$*" >&2; exit 1; }
if [[ ${1:-} == --help ]]; then
    echo '全新 Debian/Ubuntu amd64 VPS 安装；交互输入面板 URL、服务器 ID、专用 token，可选申请证书。已有安装会拒绝覆盖。'
    exit 0
fi
[[ $# == 0 ]] || fail '只支持交互安装或 --help'
[[ $EUID == 0 ]] || fail '请以 root 执行'
[[ $(uname -m) == x86_64 ]] || fail '当前发行包仅支持 x86_64/amd64'
command -v apt-get >/dev/null || fail '当前脚本仅支持 Debian/Ubuntu'
[[ -d /run/systemd/system ]] || fail '需要 systemd 系统'
for path in /opt/xboard-node-custom /etc/xboard-node-custom /var/lib/xboard-node-custom /etc/systemd/system/$UNIT; do
    [[ ! -e $path ]] || fail "已存在 $path；为保留原服务，本脚本不覆盖安装"
done
# Refuse a second copy of the previously deployed test service.
[[ ! -e /etc/systemd/system/codex-dual-anytls.service ]] || fail '本机已有实测服务，不需要重复安装'
exec 3<>/dev/tty || fail '需要交互终端'
read -r -u 3 -p '面板根地址（https://panel.example.com）：' PANEL_URL
[[ $PANEL_URL == https://* && $PANEL_URL != *'#'* && $PANEL_URL != *'?'* ]] || fail '请输入 HTTPS 面板根地址'
read -r -u 3 -p '面板服务器 ID：' MACHINE_ID
[[ $MACHINE_ID =~ ^[1-9][0-9]*$ ]] || fail '服务器 ID 必须是正整数'
read -r -s -u 3 -p '服务器专用 token（输入隐藏）：' MACHINE_TOKEN
printf '\n' >&3
[[ -n $MACHINE_TOKEN ]] || fail 'token 不能为空'
read -r -u 3 -p '申请证书的域名（已解析到本机；留空跳过）：' CERT_DOMAIN
if [[ -n $CERT_DOMAIN ]]; then
    [[ $CERT_DOMAIN =~ ^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$ && $CERT_DOMAIN == *.* ]] || fail '域名格式不正确'
    read -r -u 3 -p '证书联系邮箱：' CERT_EMAIL
    [[ $CERT_EMAIL == *@*.* && $CERT_EMAIL != -* ]] || fail '邮箱格式不正确'
    echo '将使用 Certbot standalone 申请证书，需要域名解析、TCP 80 放行，并同意 Let’s Encrypt 服务条款。' >&3
    read -r -u 3 -p '同意并申请？[y/N]：' CERT_AGREE
    [[ $CERT_AGREE == y || $CERT_AGREE == Y ]] || fail '未同意申请证书'
fi
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl python3 gzip iproute2
TMP_DIR=$(mktemp -d)
trap 'rm -rf -- "$TMP_DIR"' EXIT
curl --fail --location --retry 3 --proto '=https' --tlsv1.2 \
    "$BASE/$RELEASE/xboard-node-linux-amd64.gz" -o "$TMP_DIR/node.gz"
echo "$ARCHIVE_SHA  $TMP_DIR/node.gz" | sha256sum -c -
gzip -dc "$TMP_DIR/node.gz" > "$TMP_DIR/xboard-node"
echo "$BINARY_SHA  $TMP_DIR/xboard-node" | sha256sum -c -
export PANEL_URL MACHINE_ID MACHINE_TOKEN CERT_DOMAIN
python3 - "$TMP_DIR/config.yml" <<'PY'
import json, os, sys, urllib.parse
url=os.environ['PANEL_URL'].rstrip('/')
parts=urllib.parse.urlsplit(url)
if not parts.hostname or parts.username or parts.password or parts.path:
    sys.exit('面板地址必须是根地址，不含管理路径、用户名或密码')
config={'panel':{'url':url}, 'machine':{'machine_id':int(os.environ['MACHINE_ID']),
        'token':os.environ['MACHINE_TOKEN']}, 'kernel':{'type':'singbox'},
        'log':{'level':'info'}, 'health_port':0}
domain=os.environ.get('CERT_DOMAIN','')
if domain:
    cert_dir='/etc/letsencrypt/live/'+domain
    config['cert']={'cert_mode':'file','cert_file':cert_dir+'/fullchain.pem',
                    'key_file':cert_dir+'/privkey.pem'}
# JSON without escaped slashes is also valid YAML for this node parser.
with open(sys.argv[1], 'w') as f: json.dump(config,f)
PY
unset MACHINE_TOKEN
if [[ -n $CERT_DOMAIN ]]; then
    [[ -z $(ss -H -ltn 'sport = :80') ]] || fail 'TCP 80 已占用，请使用已有网站的证书申请方式'
    DEBIAN_FRONTEND=noninteractive apt-get install -y certbot
    certbot certonly --standalone -d "$CERT_DOMAIN" -m "$CERT_EMAIL" --agree-tos --non-interactive
    systemctl enable --now certbot.timer
fi
install -d -m 700 /etc/xboard-node-custom /var/lib/xboard-node-custom
install -d -m 755 /opt/xboard-node-custom
install -m 755 "$TMP_DIR/xboard-node" /opt/xboard-node-custom/xboard-node
install -m 600 "$TMP_DIR/config.yml" /etc/xboard-node-custom/config.yml
cat > /etc/systemd/system/$UNIT <<'SERVICE'
[Unit]
Description=Custom Xboard-Node
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
ExecStart=/opt/xboard-node-custom/xboard-node -c /etc/xboard-node-custom/config.yml
WorkingDirectory=/var/lib/xboard-node-custom
Environment=GOMEMLIMIT=384MiB
Environment=GOGC=50
UMask=0077
Restart=on-failure
RestartSec=5
LimitNOFILE=65535
[Install]
WantedBy=multi-user.target
SERVICE
chmod 644 /etc/systemd/system/$UNIT
if [[ -n $CERT_DOMAIN ]]; then
    install -d /etc/letsencrypt/renewal-hooks/deploy
    printf '#!/bin/sh\nsystemctl try-restart xboard-node-custom.service\n' > /etc/letsencrypt/renewal-hooks/deploy/xboard-node-custom.sh
    chmod 700 /etc/letsencrypt/renewal-hooks/deploy/xboard-node-custom.sh
fi
systemctl daemon-reload
systemctl enable --now "$UNIT"
sleep 3
if ! systemctl is-active --quiet "$UNIT"; then
    systemctl disable --now "$UNIT"
    fail '服务未启动，配置已保留；请执行 journalctl -u xboard-node-custom -n 50 排查，分享前隐藏凭据'
fi
echo '服务已安装并启动（尚不代表节点已连通）。请在面板把节点绑定到此服务器并分配权限组，再更新订阅测试。'
echo '放行 TCP 443/8443 和 UDP 2443；证书续期需要 TCP 80。本脚本不修改防火墙。'
if [[ -n $CERT_DOMAIN ]]; then
    printf '面板 file 证书路径：\n/etc/letsencrypt/live/%s/fullchain.pem\n/etc/letsencrypt/live/%s/privkey.pem\n' "$CERT_DOMAIN" "$CERT_DOMAIN"
fi
echo 'FlClash 使用标准 AnyTLS / Hysteria2；AnyTLS REALITY 需要兼容 sing-box。主站需配置有效的 wss:// 地址及 WebSocket 反向代理；REST 轮询保留。'
