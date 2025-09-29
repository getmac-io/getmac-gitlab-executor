package command

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/getmac-io/getmac-gitlab-executor/internal/gitlab"
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

	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	session.Stdin = script
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
