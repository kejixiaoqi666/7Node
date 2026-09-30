package singbox

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cedar2025/xboard-node/internal/config"
	"github.com/cedar2025/xboard-node/internal/kernel"
	"github.com/cedar2025/xboard-node/internal/model"
	"github.com/miekg/dns"
)

func reloadFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}

func TestRuntimeReloadDNS(t *testing.T) {
	var first, second atomic.Int32
	resolver := func(count *atomic.Int32) int {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		server := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
			count.Add(1)
			m := new(dns.Msg)
			m.SetReply(q)
			for _, question := range q.Question {
				if question.Qtype == dns.TypeA {
					m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: net.ParseIP("127.0.0.1")})
				}
			}
			w.WriteMsg(m)
		})}
		go server.ActivateAndServe()
		t.Cleanup(func() { server.Shutdown() })
		return pc.LocalAddr().(*net.UDPAddr).Port
	}
	a, b := resolver(&first), resolver(&second)
	file := t.TempDir() + "/dns.json"
	write := func(port int) {
		data, _ := json.Marshal(M{"dns": M{"servers": []M{{"type": "udp", "tag": "test", "server": "127.0.0.1", "server_port": port}}, "final": "test"}})
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(a)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	defer target.Close()
	u, _ := url.Parse(target.URL)
	_, port, _ := net.SplitHostPort(u.Host)
	node := &model.NodeSpec{Protocol: "http", ServerPort: reloadFreePort(t), CustomRoutes: []M{{"action": "resolve"}, {"outbound": "direct"}}}
	server := New(config.KernelConfig{LogLevel: "error", CustomConfig: file})
	if err := server.Start(node, nil, kernel.TLSCert{}); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", node.ServerPort))
	tr := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	fetch := func() {
		resp, err := client.Get("http://reload.test:" + port)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatal(resp.Status)
		}
	}
	fetch()
	if first.Load() == 0 || second.Load() != 0 {
		t.Fatal("initial resolver not selected")
	}
	oldBox := server.box
	if err := server.Reload(node, nil, kernel.TLSCert{}); err != nil {
		t.Fatal(err)
	}
	if server.box != oldBox {
		t.Fatal("unchanged graph unnecessarily rebuilt")
	}
	count := first.Load()
	write(b)
	if err := server.Reload(node, nil, kernel.TLSCert{}); err != nil {
		t.Fatal(err)
	}
	fetch()
	if first.Load() != count || second.Load() == 0 {
		t.Fatal("resolver or DNS cache remained stale")
	}
}

func TestRuntimeReloadOutboundLifecycle(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "direct") }))
	defer target.Close()
	upstream := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodConnect {
				fmt.Fprint(w, body)
				return
			}
			c, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(3 * time.Second))
			fmt.Fprint(rw, "HTTP/1.1 200 Connection Established\r\n\r\n")
			rw.Flush()
			if _, err = http.ReadRequest(rw.Reader); err != nil {
				return
			}
			fmt.Fprintf(rw, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
			rw.Flush()
		}))
	}
	a, b := upstream("upstream-a"), upstream("upstream-b")
	defer a.Close()
	defer b.Close()
	node := model.NodeSpec{Protocol: "http", ServerPort: reloadFreePort(t), CustomRoutes: []M{{"outbound": "direct"}}}
	server := New(config.KernelConfig{LogLevel: "error"})
	if err := server.Start(&node, nil, kernel.TLSCert{}); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	tracker := server.connTracker
	otherNode := node
	otherNode.ServerPort = reloadFreePort(t)
	other := New(config.KernelConfig{LogLevel: "error"})
	if err := other.Start(&otherNode, nil, kernel.TLSCert{}); err != nil {
		t.Fatal(err)
	}
	defer other.Stop()
	otherBox := other.box
	fetch := func(port int, want string) {
		t.Helper()
		proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
		tr := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
		defer tr.CloseIdleConnections()
		client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
		resp, err := client.Get(target.URL)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(data) != want {
			t.Fatalf("got %q want %q", data, want)
		}
	}
	reload := func(next model.NodeSpec, want string) {
		t.Helper()
		if err := server.Reload(&next, nil, kernel.TLSCert{}); err != nil {
			t.Fatal(err)
		}
		if server.connTracker != tracker {
			t.Fatal("traffic tracker replaced")
		}
		if other.box != otherBox {
			t.Fatal("other node replaced")
		}
		fetch(next.ServerPort, want)
		fetch(otherNode.ServerPort, "direct")
	}
	fetch(node.ServerPort, "direct")
	relay := node
	relay.CustomRoutes = []M{{"outbound": "relay"}}
	outbound := func(s *httptest.Server) []model.OutboundConfig {
		u, _ := url.Parse(s.URL)
		host, port, _ := net.SplitHostPort(u.Host)
		var p int
		fmt.Sscanf(port, "%d", &p)
		return []model.OutboundConfig{{Protocol: "http", Tag: "relay", Settings: M{"server": host, "server_port": p}}}
	}
	relay.CustomOutbounds = outbound(a)
	reload(relay, "upstream-a")
	relay.CustomOutbounds = outbound(b)
	reload(relay, "upstream-b")

	// Invalid removal must leave the running configuration intact.
	invalid := relay
	invalid.CustomOutbounds = nil
	before := server.box
	if err := server.Reload(&invalid, nil, kernel.TLSCert{}); err == nil {
		t.Fatal("accepted dangling route")
	}
	if server.box != before {
		t.Fatal("invalid configuration interrupted node")
	}
	fetch(node.ServerPort, "upstream-b")

	// A failure binding the replacement inbound must restore the old listener.
	conflict, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conflict.Close()
	failed := relay
	failed.ServerPort = conflict.Addr().(*net.TCPAddr).Port
	failed.CustomOutbounds = outbound(a)
	if err := server.Reload(&failed, nil, kernel.TLSCert{}); err == nil {
		t.Fatal("expected bind failure")
	}
	fetch(node.ServerPort, "upstream-b")
	if server.connTracker != tracker {
		t.Fatal("rollback lost tracker")
	}
	reload(node, "direct")
}
