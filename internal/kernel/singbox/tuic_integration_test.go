//go:build with_quic

package singbox

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cedar2025/xboard-node/internal/config"
	"github.com/cedar2025/xboard-node/internal/kernel"
	"github.com/cedar2025/xboard-node/internal/model"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singJSON "github.com/sagernet/sing/common/json"
)

// Real QUIC/TUIC round trip; self-signed certificate and all destinations are
// local fixtures. Production clients must validate the issued certificate.
func TestTUICStandardTLSRoundTrip(t *testing.T) {
	certServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer certServer.Close()
	fixture := certServer.TLS.Certificates[0]
	key, err := x509.MarshalPKCS8PrivateKey(fixture.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	cert := kernel.TLSCert{CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.Certificate[0]}), KeyPEM: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	pc.Close()
	user := model.UserSpec{ID: 1, UUID: "11111111-1111-4111-8111-111111111111"}
	node := &model.NodeSpec{Protocol: "tuic", ServerPort: port, CongestionControl: "bbr", CustomRoutes: []M{{"ip_cidr": []string{"127.0.0.1/32"}, "outbound": "direct"}}}
	server := New(config.KernelConfig{LogLevel: "error"})
	if err = server.Start(node, []model.UserSpec{user}, cert); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(w, r.Body) }))
	defer target.Close()
	proxyPort := reloadFreePort(t)
	cfg := M{"log": M{"disabled": true}, "inbounds": []M{{"type": "mixed", "listen": "127.0.0.1", "listen_port": proxyPort}}, "outbounds": []M{{"type": "tuic", "tag": "proxy", "server": "127.0.0.1", "server_port": port, "uuid": user.UUID, "password": user.UUID, "congestion_control": "bbr", "tls": M{"enabled": true, "insecure": true, "alpn": []string{"h3"}}}}, "route": M{"final": "proxy"}}
	data, _ := json.Marshal(cfg)
	ctx := include.Context(context.Background())
	opts, err := singJSON.UnmarshalExtendedContext[option.Options](ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	clientCore, err := box.New(box.Options{Context: ctx, Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	defer clientCore.Close()
	if err = clientCore.Start(); err != nil {
		t.Fatal(err)
	}
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
	tr := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	payload := strings.Repeat("tuic-tls-fixture", 8192)
	response, err := client.Post(target.URL, "application/octet-stream", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(body) != payload {
		t.Fatalf("TUIC round trip failed: status=%d bytes=%d err=%v", response.StatusCode, len(body), err)
	}
}
