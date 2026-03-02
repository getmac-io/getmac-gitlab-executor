package tunnel

import (
	"fmt"
	"io"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

// Tunnel manages a reverse SSH tunnel that forwards connections from the
// remote side to a local address.
type Tunnel struct {
	listener net.Listener
	wg       sync.WaitGroup
	done     chan struct{}
}

// Start opens a remote listener on the SSH server at remoteAddr and forwards
// each accepted connection to localAddr on the runner host.
func Start(sshClient *ssh.Client, remoteAddr, localAddr string) (*Tunnel, error) {
	ln, err := sshClient.Listen("tcp", remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on remote %s: %w", remoteAddr, err)
	}

	t := &Tunnel{
		listener: ln,
		done:     make(chan struct{}),
	}

	t.wg.Add(1)
	go t.acceptLoop(localAddr)

	return t, nil
}

// Close stops accepting new connections and waits for existing ones to drain.
func (t *Tunnel) Close() error {
	close(t.done)
	err := t.listener.Close()
	t.wg.Wait()
	return err
}

func (t *Tunnel) acceptLoop(localAddr string) {
	defer t.wg.Done()
	for {
		remote, err := t.listener.Accept()
		if err != nil {
			select {
			case <-t.done:
				return
			default:
				continue
			}
		}
		t.wg.Add(1)
		go t.bridge(remote, localAddr)
	}
}

func (t *Tunnel) bridge(remote net.Conn, localAddr string) {
	defer t.wg.Done()
	defer remote.Close()

	local, err := net.Dial("tcp", localAddr)
	if err != nil {
		return
	}
	defer local.Close()

	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		io.Copy(dst, src)
		done <- struct{}{}
	}
	go cp(local, remote)
	go cp(remote, local)
	<-done
}
