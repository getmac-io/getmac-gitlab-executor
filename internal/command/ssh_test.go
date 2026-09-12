package command

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type fakeGateway struct {
	addr     string
	auths    atomic.Int32
	sessions atomic.Int32
}

func newTestSigner(t *testing.T) ssh.Signer {
	t.Helper()

	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}

	return signer
}

// startFakeGateway runs an SSH server that rejects authentication for the first
// rejectAuths connections and refuses the first rejectSessions session channels,
// the way the GetMac gateway behaves while a virtual machine is starting.
func startFakeGateway(t *testing.T, clientKey ssh.PublicKey, rejectAuths, rejectSessions int32) *fakeGateway {
	t.Helper()

	gw := &fakeGateway{}
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if !bytes.Equal(key.Marshal(), clientKey.Marshal()) {
				return nil, errors.New("public key not authorized")
			}

			if gw.auths.Add(1) <= rejectAuths {
				return nil, errors.New("virtual machine not running")
			}

			return &ssh.Permissions{}, nil
		},
	}
	config.AddHostKey(newTestSigner(t))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	gw.addr = ln.Addr().String()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go gw.serve(conn, config, rejectSessions)
		}
	}()

	return gw
}

func (gw *fakeGateway) serve(conn net.Conn, config *ssh.ServerConfig, rejectSessions int32) {
	defer conn.Close()

	serverConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer serverConn.Close()
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if gw.sessions.Add(1) <= rejectSessions {
			newChannel.Reject(ssh.ConnectionFailed, "virtual machine is not reachable")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go ssh.DiscardRequests(requests)
		go func() {
			_, _ = io.Copy(io.Discard, channel)
			channel.Close()
		}()
	}
}

func testSSHClientConfig(signer ssh.Signer) *ssh.ClientConfig {
	config := newSSHClientConfig(signer, "vm-1")
	config.Timeout = 2 * time.Second
	return config
}

func TestConnectToVirtualMachine_RetriesUntilAuthenticated(t *testing.T) {
	signer := newTestSigner(t)
	gw := startFakeGateway(t, signer.PublicKey(), 2, 0)

	client, err := connectToVirtualMachine(context.Background(), gw.addr, testSSHClientConfig(signer), 5*time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	if got := gw.auths.Load(); got != 3 {
		t.Fatalf("expected 3 authentication attempts, got %d", got)
	}

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("returned client can't open a session: %v", err)
	}
	session.Close()
}

func TestConnectToVirtualMachine_RetriesWhenSessionIsRefused(t *testing.T) {
	signer := newTestSigner(t)
	gw := startFakeGateway(t, signer.PublicKey(), 0, 1)

	client, err := connectToVirtualMachine(context.Background(), gw.addr, testSSHClientConfig(signer), 5*time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	if got := gw.sessions.Load(); got != 2 {
		t.Fatalf("expected 2 session attempts, got %d", got)
	}
}

func TestConnectToVirtualMachine_GivesUpAfterTimeout(t *testing.T) {
	signer := newTestSigner(t)
	gw := startFakeGateway(t, signer.PublicKey(), 1<<30, 0)

	_, err := connectToVirtualMachine(context.Background(), gw.addr, testSSHClientConfig(signer), 200*time.Millisecond, 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "unable to authenticate") {
		t.Fatalf("expected authentication error after timeout, got %v", err)
	}
}

func TestConnectToVirtualMachine_StopsWhenCancelled(t *testing.T) {
	signer := newTestSigner(t)
	gw := startFakeGateway(t, signer.PublicKey(), 1<<30, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := connectToVirtualMachine(ctx, gw.addr, testSSHClientConfig(signer), time.Minute, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error")
	}

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("expected to stop shortly after cancellation, took %s", elapsed)
	}
}
