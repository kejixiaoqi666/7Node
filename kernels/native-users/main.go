// SPDX-License-Identifier: GPL-3.0-or-later
// Optional external protocol kernel for xbord-node-v3; not an official sing-box release.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/adapter/service"
	"github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/dns/transport/local"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/direct"
	"github.com/sagernet/sing-box/protocol/trojan"
	"github.com/sagernet/sing-box/protocol/vless"
)

const capability = "xbord-native-users-v1"
const maxConfigBytes = 16 * 1024 * 1024
const maxRequestBytes = 64 * 1024
const controlTimeout = 2 * time.Second

type candidate struct {
	options option.Options
	digest  string
	base    []byte
}

// Register only the protocols/outbound needed by the experimental Rust slice.
// This avoids adding a TCP relay or a second data-plane process for user updates.
func kernelContext() context.Context {
	in := inbound.NewRegistry()
	vless.RegisterInbound(in)
	trojan.RegisterInbound(in)
	out := outbound.NewRegistry()
	direct.RegisterOutbound(out)
	d := dns.NewTransportRegistry()
	local.RegisterTransport(d)
	return box.Context(context.Background(), in, out, endpoint.NewRegistry(), d, service.NewRegistry())
}

func readCandidate(ctx context.Context, path string) (candidate, error) {
	if !filepath.IsAbs(path) {
		return candidate{}, errors.New("absolute configuration path required")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxConfigBytes {
		return candidate{}, errors.New("invalid configuration file")
	}
	file, err := os.Open(path)
	if err != nil {
		return candidate{}, errors.New("configuration unavailable")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil || len(data) > maxConfigBytes {
		return candidate{}, errors.New("configuration exceeds bound")
	}
	var opts option.Options
	if err = opts.UnmarshalJSONContext(ctx, data); err != nil {
		return candidate{}, errors.New("invalid kernel configuration")
	}
	if len(opts.Inbounds) != 1 || opts.Inbounds[0].Tag == "" {
		return candidate{}, errors.New("one named inbound required")
	}
	switch users := opts.Inbounds[0].Options.(type) {
	case *option.VLESSInboundOptions:
		if err = validateNames(users.Users, func(u option.VLESSUser) string { return u.Name }); err != nil {
			return candidate{}, err
		}
	case *option.TrojanInboundOptions:
		if err = validateNames(users.Users, func(u option.TrojanUser) string { return u.Name }); err != nil {
			return candidate{}, err
		}
	default:
		return candidate{}, errors.New("only VLESS/Trojan supported")
	}
	base, err := withoutUsers(data)
	if err != nil {
		return candidate{}, errors.New("invalid configuration structure")
	}
	hash := sha256.Sum256(data)
	return candidate{opts, hex.EncodeToString(hash[:]), base}, nil
}

func validateNames[T any](users []T, name func(T) string) error {
	seen := make(map[string]struct{}, len(users))
	for _, user := range users {
		id := name(user)
		if id == "" {
			return errors.New("stable user names required")
		}
		if _, exists := seen[id]; exists {
			return errors.New("duplicate user identity")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func withoutUsers(data []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var inbounds []map[string]json.RawMessage
	if err := json.Unmarshal(root["inbounds"], &inbounds); err != nil || len(inbounds) != 1 {
		return nil, errors.New("one inbound required")
	}
	delete(inbounds[0], "users")
	encoded, err := json.Marshal(inbounds)
	if err != nil {
		return nil, err
	}
	root["inbounds"] = encoded
	// Normalize objects, including nested TLS settings, without losing numbers.
	encoded, err = json.Marshal(root)
	if err != nil {
		return nil, err
	}
	var normalized any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err = decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

type request struct {
	Operation      string `json:"operation"`
	ExpectedDigest string `json:"expected_digest,omitempty"`
	Digest         string `json:"digest,omitempty"`
	Path           string `json:"path,omitempty"`
}
type response struct {
	Capability string `json:"capability"`
	Code       string `json:"code"`
	Digest     string `json:"digest"`
}
type controller struct {
	ctx       context.Context
	kernel    adapter.Inbound
	digest    string
	base      []byte
	directory string
}

// Calls are serialized by the single bounded control listener. Data-plane
// authentication runs concurrently against immutable, atomically published maps.
func (c *controller) apply(req request) response {
	result := response{capability, "rejected", c.digest}
	if req.Operation == "status" {
		result.Code = "ok"
		return result
	}
	if req.Operation != "replace" || req.ExpectedDigest != c.digest {
		result.Code = "conflict"
		return result
	}
	if !filepath.IsAbs(req.Path) || filepath.Dir(filepath.Clean(req.Path)) != c.directory {
		return result
	}
	info, err := os.Lstat(req.Path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return result
	}
	next, err := readCandidate(c.ctx, req.Path)
	if err != nil || next.digest != req.Digest {
		return result
	}
	if !bytes.Equal(c.base, next.base) {
		result.Code = "restart_required"
		return result
	}
	// UpdateUsers validates completely before one atomic store. No mutation
	// or connection teardown occurs on validation errors.
	switch users := next.options.Inbounds[0].Options.(type) {
	case *option.VLESSInboundOptions:
		kernel, ok := c.kernel.(adapter.UpdatableInbound[option.VLESSUser])
		if !ok {
			return result
		}
		err = kernel.UpdateUsers(users.Users)
	case *option.TrojanInboundOptions:
		kernel, ok := c.kernel.(adapter.UpdatableInbound[option.TrojanUser])
		if !ok {
			return result
		}
		err = kernel.UpdateUsers(users.Users)
	default:
		return result
	}
	if err != nil {
		return result
	}
	c.digest = next.digest
	result.Code, result.Digest = "ok", next.digest
	return result
}

func handleControl(conn net.Conn, c *controller) {
	defer conn.Close()
	if conn.SetDeadline(time.Now().Add(controlTimeout)) != nil {
		return
	}
	var length uint32
	if binary.Read(conn, binary.BigEndian, &length) != nil || length == 0 || length > maxRequestBytes {
		return
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return
	}
	var req request
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil {
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		return
	}
	reply, err := json.Marshal(c.apply(req))
	if err != nil {
		return
	}
	if binary.Write(conn, binary.BigEndian, uint32(len(reply))) != nil {
		return
	}
	_, _ = conn.Write(reply)
}

func serveControl(listener *net.UnixListener, c *controller) error {
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			return err
		}
		handleControl(conn, c)
	}
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println(capability)
		return nil
	}
	if len(args) == 0 || (args[0] != "run" && args[0] != "check") {
		return errors.New("usage: xbord-native-users <run|check> -c <absolute config> [--control-socket <private Unix socket>]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("c", "", "configuration")
	socket := flags.String("control-socket", "", "private Unix socket")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return errors.New("invalid arguments")
	}
	ctx := kernelContext()
	initial, err := readCandidate(ctx, *path)
	if err != nil {
		return err
	}
	instance, err := box.New(box.Options{Context: ctx, Options: initial.options})
	if err != nil {
		return errors.New("kernel rejected configuration")
	}
	defer instance.Close()
	if args[0] == "check" {
		return nil
	}
	if !filepath.IsAbs(*socket) || filepath.Dir(*socket) != filepath.Dir(*path) {
		return errors.New("private control socket must share configuration directory")
	}
	dir, err := os.Stat(filepath.Dir(*socket))
	if err != nil || !dir.IsDir() || dir.Mode().Perm()&0077 != 0 {
		return errors.New("private state directory permissions required")
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: *socket, Net: "unix"})
	if err != nil {
		return errors.New("could not create control socket")
	}
	defer listener.Close()
	if err = os.Chmod(*socket, 0600); err != nil {
		return errors.New("could not protect control socket")
	}
	if err = instance.Start(); err != nil {
		return errors.New("kernel could not start")
	}
	in, ok := instance.Inbound().Get(initial.options.Inbounds[0].Tag)
	if !ok {
		return errors.New("kernel inbound unavailable")
	}
	c := &controller{ctx, in, initial.digest, initial.base, filepath.Dir(*path)}
	// Release temporary decoded user lists before the long-running control loop.
	initial.options = option.Options{}
	stopped := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		stopped <- serveControl(listener, c)
	}()
	// Close the bounded control loop before closing the data plane, so no user
	// publication can race a graceful shutdown. Rust can still reap with SIGKILL.
	defer func() {
		listener.Close()
		<-done
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
		return nil
	case <-stopped:
		return errors.New("control listener stopped")
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
