// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
)

func config(users, port string) []byte {
	return []byte(`{"log":{"level":"error"},"inbounds":[{"type":"vless","tag":"vless-in","listen":"127.0.0.1","listen_port":` + port + `,"users":` + users + `}],"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"direct"}}`)
}

const userA = `[{"name":"100","uuid":"00000000-0000-4000-8000-000000000100"}]`
const userB = `[{"name":"200","uuid":"00000000-0000-4000-8000-000000000200"}]`

type fakeInbound struct {
	adapter.Inbound
	err   error
	calls int
}

func (f *fakeInbound) UpdateUsers(_ []option.VLESSUser) error { f.calls++; return f.err }

func writeConfig(t *testing.T, directory, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOnlyUserReplacementAndFailedUpdateHaveDefinedState(t *testing.T) {
	directory := t.TempDir()
	ctx := kernelContext()
	a, err := readCandidate(ctx, writeConfig(t, directory, "a.json", config(userA, "12345")))
	if err != nil {
		t.Fatal(err)
	}
	bPath := writeConfig(t, directory, "b.json", config(userB, "12345"))
	b, err := readCandidate(ctx, bPath)
	if err != nil {
		t.Fatal(err)
	}
	in := &fakeInbound{}
	c := &controller{ctx, in, a.digest, a.base, directory}
	reply := c.apply(request{"replace", a.digest, b.digest, bPath})
	if reply.Code != "ok" || c.digest != b.digest || in.calls != 1 {
		t.Fatalf("replacement failed: %+v", reply)
	}
	// Stale expected state must never call the kernel.
	reply = c.apply(request{"replace", a.digest, b.digest, bPath})
	if reply.Code != "conflict" || in.calls != 1 {
		t.Fatal("stale state applied")
	}
	in.err = errors.New("injected rejection")
	aPath := writeConfig(t, directory, "a2.json", config(userA, "12345"))
	reply = c.apply(request{"replace", b.digest, a.digest, aPath})
	if reply.Code != "rejected" || c.digest != b.digest {
		t.Fatal("failed replacement committed")
	}
	changedPath := writeConfig(t, directory, "changed.json", config(userA, "12346"))
	changed, err := readCandidate(ctx, changedPath)
	if err != nil {
		t.Fatal(err)
	}
	calls := in.calls
	reply = c.apply(request{"replace", b.digest, changed.digest, changedPath})
	if reply.Code != "restart_required" || in.calls != calls || c.digest != b.digest {
		t.Fatal("non-user change mutated live kernel")
	}
}

func TestInvalidStructureDigestPathAndNamesCannotCommit(t *testing.T) {
	directory := t.TempDir()
	ctx := kernelContext()
	aPath := writeConfig(t, directory, "a.json", config(userA, "12345"))
	a, err := readCandidate(ctx, aPath)
	if err != nil {
		t.Fatal(err)
	}
	in := &fakeInbound{}
	c := &controller{ctx, in, a.digest, a.base, directory}
	other := writeConfig(t, t.TempDir(), "other.json", config(userB, "12345"))
	for _, req := range []request{{"replace", a.digest, strings.Repeat("f", 64), aPath}, {"replace", a.digest, a.digest, other}, {"unknown", a.digest, a.digest, aPath}} {
		if c.apply(req).Code == "ok" {
			t.Fatal("invalid request accepted")
		}
	}
	if in.calls != 0 || c.digest != a.digest {
		t.Fatal("invalid request reached kernel")
	}
	for i, users := range []string{`[{"name":"","uuid":"a"}]`, `[{"name":"100","uuid":"a"},{"name":"100","uuid":"b"}]`} {
		_, err := readCandidate(ctx, writeConfig(t, directory, string(rune('x'+i))+".json", config(users, "12345")))
		if err == nil {
			t.Fatal("unstable identity accepted")
		}
	}
}

func TestControlFramingBoundsAndStrictRequest(t *testing.T) {
	c := &controller{ctx: context.Background(), digest: strings.Repeat("a", 64)}
	for _, payload := range [][]byte{[]byte(`{"operation":"status"}`), []byte(`{"operation":"status","unexpected":1}`), []byte(`{"operation":"status"} {}`)} {
		client, server := net.Pipe()
		finished := make(chan struct{})
		go func() { handleControl(server, c); close(finished) }()
		client.SetDeadline(time.Now().Add(time.Second))
		if err := binary.Write(client, binary.BigEndian, uint32(len(payload))); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Write(payload); err != nil {
			t.Fatal(err)
		}
		var size uint32
		err := binary.Read(client, binary.BigEndian, &size)
		valid := bytes.Equal(payload, []byte(`{"operation":"status"}`))
		if valid {
			if err != nil || size > 8192 {
				t.Fatal("valid status framing failed")
			}
			reply := make([]byte, size)
			if _, err = io.ReadFull(client, reply); err != nil {
				t.Fatal(err)
			}
			var result response
			if json.Unmarshal(reply, &result) != nil || result.Code != "ok" || result.Capability != capability {
				t.Fatal("invalid status response")
			}
		} else if err == nil {
			t.Fatal("invalid request received success response")
		}
		client.Close()
		<-finished
	}
	client, server := net.Pipe()
	finished := make(chan struct{})
	go func() { handleControl(server, c); close(finished) }()
	client.SetDeadline(time.Now().Add(time.Second))
	binary.Write(client, binary.BigEndian, uint32(maxRequestBytes+1))
	var size uint32
	if binary.Read(client, binary.BigEndian, &size) == nil {
		t.Fatal("oversized frame accepted")
	}
	client.Close()
	<-finished
}
