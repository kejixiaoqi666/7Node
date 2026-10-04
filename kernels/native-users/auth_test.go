// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"net"
	"sync"
	"testing"
	"time"

	trojan "github.com/sagernet/sing-box/transport/trojan"
	vless "github.com/sagernet/sing-vmess/vless"
	"github.com/sagernet/sing/common/auth"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type authHandler struct{ identities chan string }

func (h *authHandler) NewConnectionEx(ctx context.Context, conn net.Conn, _ M.Socksaddr, _ M.Socksaddr, _ N.CloseHandlerFunc) {
	identity, ok := auth.UserFromContext[string](ctx)
	if !ok {
		identity = "MISSING"
	}
	h.identities <- identity
	conn.Close()
}
func (h *authHandler) NewPacketConnectionEx(context.Context, N.PacketConn, M.Socksaddr, M.Socksaddr, N.CloseHandlerFunc) {
	panic("unexpected packet")
}

const uuidA = "00000000-0000-4000-8000-000000000100"
const uuidB = "00000000-0000-4000-8000-000000000200"

func vlessHeader() []byte {
	id, _ := hex.DecodeString("00000000000040008000000000000100")
	data := append([]byte{0}, id...)
	// version, UUID, empty addons, TCP, port 12345, IPv4 127.0.0.1.
	return append(data, 0, 1, 0x30, 0x39, 1, 127, 0, 0, 1)
}

func authenticateVless(s *vless.Service[string], header []byte) error {
	client, server := net.Pipe()
	client.SetDeadline(time.Now().Add(2 * time.Second))
	server.SetDeadline(time.Now().Add(2 * time.Second))
	finished := make(chan error, 1)
	go func() {
		err := s.NewConnection(context.Background(), server, M.Socksaddr{}, nil)
		server.Close()
		finished <- err
	}()
	client.Write(header)
	client.Close()
	return <-finished
}

func TestVlessConcurrentReplacementKeepsStableIdentity(t *testing.T) {
	h := &authHandler{make(chan string, 1024)}
	s := vless.NewService[string](nil, h)
	if err := s.UpdateUsers([]string{"100", "200"}, []string{uuidA, uuidB}, []string{"", ""}); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for i := 0; i < 1000; i++ {
			var err error
			if i%2 == 0 {
				err = s.UpdateUsers([]string{"200", "100"}, []string{uuidB, uuidA}, []string{"", ""})
			} else {
				err = s.UpdateUsers([]string{"100"}, []string{uuidA}, []string{""})
			}
			if err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for w := 0; w < 16; w++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 32; i++ {
				if err := authenticateVless(s, vlessHeader()); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	workers.Wait()
	if len(h.identities) != 512 {
		t.Fatalf("authenticated %d/512", len(h.identities))
	}
	for len(h.identities) > 0 {
		if identity := <-h.identities; identity != "100" {
			t.Fatalf("wrong identity %s", identity)
		}
	}
	if err := s.UpdateUsers(nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if authenticateVless(s, vlessHeader()) == nil {
		t.Fatal("removed UUID authenticated")
	}
}

func TestVlessInvalidReplacementDoesNotPublishPartialState(t *testing.T) {
	h := &authHandler{make(chan string, 16)}
	s := vless.NewService[string](nil, h)
	s.UpdateUsers([]string{"100"}, []string{uuidA}, []string{""})
	cases := []struct{ names, ids, flows []string }{
		{[]string{"100", "200"}, []string{uuidA, uuidA}, []string{"", ""}},
		{[]string{"100", "100"}, []string{uuidA, uuidB}, []string{"", ""}},
		{[]string{"200"}, []string{"invalid"}, []string{""}},
		{[]string{"200"}, nil, []string{""}},
		{[]string{"200"}, []string{uuidB}, []string{"unsupported"}},
	}
	for _, c := range cases {
		if s.UpdateUsers(c.names, c.ids, c.flows) == nil {
			t.Fatal("invalid replacement accepted")
		}
		if err := authenticateVless(s, vlessHeader()); err != nil {
			t.Fatal("old snapshot lost", err)
		}
		if <-h.identities != "100" {
			t.Fatal("wrong old identity")
		}
	}
}

func trojanTail() []byte {
	data := []byte{trojan.CommandTCP, 1, 127, 0, 0, 1}
	data = binary.BigEndian.AppendUint16(data, 12345)
	return append(data, '\r', '\n')
}

func TestTrojanDelayedHandshakeRetainsAuthenticatedIdentityAfterRemoval(t *testing.T) {
	h := &authHandler{make(chan string, 16)}
	s := trojan.NewService[string](h, nil, nil)
	if err := s.UpdateUsers([]string{"100", "200"}, []string{"a-password", "b-password"}); err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	client.SetDeadline(time.Now().Add(2 * time.Second))
	server.SetDeadline(time.Now().Add(2 * time.Second))
	finished := make(chan error, 1)
	go func() {
		err := s.NewConnection(context.Background(), server, M.Socksaddr{}, nil)
		server.Close()
		finished <- err
	}()
	key := trojan.Key("a-password")
	// Fragmented credential header is legal on TCP.
	for i := 0; i < len(key); i += 7 {
		end := i + 7
		if end > len(key) {
			end = len(key)
		}
		if _, err := client.Write(key[i:end]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.Write([]byte{'\r', '\n'}); err != nil {
		t.Fatal(err)
	}
	// The service authenticated A before reading the command; B is now index 0.
	if err := s.UpdateUsers([]string{"200"}, []string{"b-password"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(trojanTail()); err != nil {
		t.Fatal(err)
	}
	client.Close()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if identity := <-h.identities; identity != "100" {
		t.Fatal("identity changed after removal", identity)
	}
}

func TestTrojanConcurrentUpdateValidationAndRemoval(t *testing.T) {
	h := &authHandler{make(chan string, 1024)}
	s := trojan.NewService[string](h, nil, nil)
	s.UpdateUsers([]string{"100"}, []string{"a-password"})
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for i := 0; i < 1000; i++ {
			if err := s.UpdateUsers([]string{"200", "100"}, []string{"b-password", "a-password"}); err != nil {
				t.Error(err)
			}
		}
	}()
	for w := 0; w < 16; w++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 32; i++ {
				client, server := net.Pipe()
				client.SetDeadline(time.Now().Add(2 * time.Second))
				server.SetDeadline(time.Now().Add(2 * time.Second))
				finished := make(chan error, 1)
				go func() {
					err := s.NewConnection(context.Background(), server, M.Socksaddr{}, nil)
					server.Close()
					finished <- err
				}()
				key := trojan.Key("a-password")
				header := append(key[:], '\r', '\n')
				header = append(header, trojanTail()...)
				client.Write(header)
				client.Close()
				if err := <-finished; err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	workers.Wait()
	if len(h.identities) != 512 {
		t.Fatal("missing authentications", len(h.identities))
	}
	for len(h.identities) > 0 {
		if identity := <-h.identities; identity != "100" {
			t.Fatal("wrong identity", identity)
		}
	}
	for _, passwords := range [][]string{{"same", "same"}, {"a-password", ""}, nil} {
		if s.UpdateUsers([]string{"100", "200"}, passwords) == nil {
			t.Fatal("invalid replacement accepted")
		}
	}
	if err := s.UpdateUsers(nil, nil); err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	client.SetDeadline(time.Now().Add(time.Second))
	server.SetDeadline(time.Now().Add(time.Second))
	finished := make(chan error, 1)
	go func() {
		err := s.NewConnection(context.Background(), server, M.Socksaddr{}, nil)
		server.Close()
		finished <- err
	}()
	key := trojan.Key("a-password")
	client.Write(key[:])
	client.Close()
	if <-finished == nil {
		t.Fatal("removed credential authenticated")
	}
}
