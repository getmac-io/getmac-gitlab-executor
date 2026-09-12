package command

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/getmac-io/getmac-gitlab-executor/internal/gitlab"
	"github.com/getmac-io/getmac-gitlab-executor/internal/proxy"
	"github.com/getmac-io/getmac-gitlab-executor/internal/tunnel"
	"github.com/getmac-io/getmac-sdk-golang"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

func NewRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "run <script>",
		RunE: runRunCommand,
		Args: cobra.MinimumNArgs(1),
	}

	return cmd
}

func runRunCommand(cmd *cobra.Command, args []string) error {
	script, err := os.OpenFile(args[0], os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open script file: %w", err)
	}
	defer script.Close()

	env, err := gitlab.NewEnvironment()
	if err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	client := getmac.NewClient(
		getmac.WithToken(env.Token), getmac.WithBaseURL(env.URL))

	_, vm, err := client.VirtualMachines().GetByName(cmd.Context(), env.ProjectID, fmt.Sprintf("gitlab-job-%s", env.JobID))
	if err != nil {
		// The job can't run without its virtual machine. That's a problem with the
		// environment, not with the job's script.
		return gitlab.NewSystemFailureError(fmt.Errorf("failed to get virtual machine by name: %w", err))
	}

	signer, err := loadSSHSigner(env.SSHPrivateKeyPath)
	if err != nil {
		return err
	}

	// Retries cover a virtual machine that is still booting or a brief gateway
	// outage. Signal handling ends once connected, so cancelling a running script
	// still stops the executor as before.
	connectCtx, stopSignals := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	sshClient, err := connectToVirtualMachine(
		connectCtx, sshGatewayAddr, newSSHClientConfig(signer, vm.ID), env.SSHReadyTimeout, sshRetryInterval)
	stopSignals()
	if err != nil {
		return gitlab.NewSystemFailureError(fmt.Errorf("failed to connect via SSH: %w", err))
	}
	defer sshClient.Close()

	var stdin io.Reader = script

	if env.ProxyTunnelEnabled {
		// Start local HTTP proxy on a dynamic port
		proxySrv := proxy.NewServer()
		proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("failed to start proxy listener: %w", err)
		}
		go proxySrv.Serve(proxyLn)
		defer proxySrv.Close()

		proxyAddr := proxyLn.Addr().String()

		// Start reverse tunnel: VM:<tunnelPort> → runner:proxyAddr
		remoteAddr := fmt.Sprintf("127.0.0.1:%d", env.ProxyTunnelPort)
		tun, err := tunnel.Start(sshClient, remoteAddr, proxyAddr)
		if err != nil {
			return fmt.Errorf("failed to start reverse tunnel: %w", err)
		}
		defer tun.Close()

		// Prepend proxy config to script stdin
		proxyURL := fmt.Sprintf("http://127.0.0.1:%d", env.ProxyTunnelPort)
		envExport := fmt.Sprintf(
			"export HTTP_PROXY=%s HTTPS_PROXY=%s http_proxy=%s https_proxy=%s no_proxy=localhost,127.0.0.1 NO_PROXY=localhost,127.0.0.1\ngit config --global http.proxy %s\n",
			proxyURL, proxyURL, proxyURL, proxyURL, proxyURL,
		)
		stdin = io.MultiReader(strings.NewReader(envExport), script)
	}

	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	session.Stdin = stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	err = session.Shell()
	if err != nil {
		return fmt.Errorf("failed to start SSH session shell: %w", err)
	}

	err = session.Wait()
	if err != nil {
		switch err.(type) {
		case *ssh.ExitError:
			return fmt.Errorf("remote command exited with non-zero status: %w", err)
		case *ssh.ExitMissingError:
			return errors.New("remote command exited without exit status or exit signal")
		default:
			return fmt.Errorf("failed to wait for SSH session: %w", err)
		}
	}

	return nil
}
