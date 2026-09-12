package command

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/getmac-io/getmac-gitlab-executor/internal/gitlab"
	"github.com/getmac-io/getmac-sdk-golang"
	"github.com/spf13/cobra"
)

func NewPrepareCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Prepare command to create a virtual machine for the job",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runPrepareCommand(cmd, args)
			if err != nil {
				return gitlab.NewSystemFailureError(err)
			}

			return nil
		},
		Args: cobra.NoArgs,
	}

	return cmd
}

func runPrepareCommand(cmd *cobra.Command, args []string) error {
	env, err := gitlab.NewEnvironment()
	if err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	// Check the key before creating a virtual machine the job couldn't connect to.
	signer, err := loadSSHSigner(env.SSHPrivateKeyPath)
	if err != nil {
		return err
	}

	// GitLab Runner sends SIGTERM when the job is cancelled or times out. Stop
	// waiting then; the cleanup stage still deletes the virtual machine.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	machineType := env.MachineType
	if machineType == "" {
		machineType = "(from image label)"
	}
	slog.Info("Creating virtual machine...", "image", env.MachineImage, "type", machineType, "region", env.Region)

	client := getmac.NewClient(
		getmac.WithToken(env.Token), getmac.WithBaseURL(env.URL))

	_, vm, err := client.VirtualMachines().Create(ctx, env.ProjectID, &getmac.CreateVirtualMachineRequest{
		Name:   fmt.Sprintf("gitlab-job-%s", env.JobID),
		Image:  env.MachineImage,
		Type:   env.MachineType,
		Region: env.Region,
	})
	if err != nil {
		return fmt.Errorf("failed to create virtual machine: %w", err)
	}

	// The API reports the image and type a label resolved to.
	slog.Info("Virtual machine created", "id", vm.ID, "image", vm.Image, "type", vm.Type)
	slog.Info("Waiting for the virtual machine to start...", "timeout", env.VMReadyTimeout.String())

	vm, err = waitForVirtualMachineRunning(ctx, client, env.ProjectID, vm.ID, waitOptions{
		Timeout:        env.VMReadyTimeout,
		PollInterval:   vmPollInterval,
		RequestTimeout: vmRequestTimeout,
	})
	if err != nil {
		return err
	}

	slog.Info("Waiting for SSH access to the virtual machine...", "timeout", env.SSHReadyTimeout.String())

	sshClient, err := connectToVirtualMachine(
		ctx, sshGatewayAddr, newSSHClientConfig(signer, vm.ID), env.SSHReadyTimeout, sshRetryInterval)
	if err != nil {
		return fmt.Errorf("virtual machine %s is running but not reachable via SSH: %w", vm.ID, err)
	}
	sshClient.Close()

	slog.Info("Virtual machine is ready", "id", vm.ID)

	return nil
}
