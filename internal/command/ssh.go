package command

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	sshGatewayAddr    = "ssh.getmac.io:22"
	sshRetryInterval  = 5 * time.Second
	sshAttemptTimeout = 10 * time.Second
)

func loadSSHSigner(path string) (ssh.Signer, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open SSH private key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH private key: %w", err)
	}

	return signer, nil
}

func newSSHClientConfig(signer ssh.Signer, vmID string) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User:            vmID,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         sshAttemptTimeout,
	}
}

// connectToVirtualMachine opens an SSH connection to the virtual machine through
// the GetMac SSH gateway, retrying until it succeeds or the timeout expires. The
// gateway rejects authentication until the virtual machine is running, and the
// guest can still be booting after that, so early attempts are expected to fail.
func connectToVirtualMachine(
	ctx context.Context, addr string, config *ssh.ClientConfig, timeout, retryInterval time.Duration) (*ssh.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	var lastErr error

	for attempt := 1; ; attempt++ {
		client, err := dialVirtualMachine(ctx, addr, config)
		if err == nil {
			if attempt > 1 {
				slog.Info("Connected to virtual machine",
					"attempts", attempt, "elapsed", time.Since(start).Round(time.Second).String())
			}

			return client, nil
		}

		// An attempt cut short by the overall deadline or cancellation fails with an
		// I/O timeout that hides why the earlier attempts failed, so keep the
		// earlier error in that case.
		if ctx.Err() == nil || lastErr == nil {
			if lastErr == nil || lastErr.Error() != err.Error() {
				slog.Info("Virtual machine is not reachable via SSH yet, retrying", "attempt", attempt, "error", err)
			}
			lastErr = err
		}

		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, ctx.Err()
			}

			return nil, fmt.Errorf("gave up after %d attempts in %s: %w", attempt, timeout, lastErr)
		case <-time.After(retryInterval):
		}
	}
}

// dialVirtualMachine makes a single connection attempt. The gateway completes the
// SSH handshake before it connects to the virtual machine, so an attempt only
// succeeds once the virtual machine has accepted a session.
func dialVirtualMachine(ctx context.Context, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	attemptTimeout := config.Timeout
	if attemptTimeout <= 0 {
		attemptTimeout = sshAttemptTimeout
	}

	dialer := net.Dialer{Timeout: attemptTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	// Bound the handshake and the session check, and abort both if ctx ends first.
	deadline := time.Now().Add(attemptTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, err
	}
	client := ssh.NewClient(clientConn, chans, reqs)

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("virtual machine did not accept an SSH session: %w", err)
	}
	session.Close()

	if !stop() {
		client.Close()
		return nil, ctx.Err()
	}
	_ = conn.SetDeadline(time.Time{})

	return client, nil
}
