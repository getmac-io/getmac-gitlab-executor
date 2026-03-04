package command

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

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
		return fmt.Errorf("failed to get virtual machine by name: %w", err)
	}

	sshKey, err := os.ReadFile(env.SSHPrivateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to open SSH private key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(sshKey)
	if err != nil {
		return fmt.Errorf("failed to parse SSH private key: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            vm.ID,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	sshClient, err := ssh.Dial("tcp", "ssh.getmac.io:22", sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect via SSH: %w", err)
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
