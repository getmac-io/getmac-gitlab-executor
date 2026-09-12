package command

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/getmac-io/getmac-gitlab-executor/internal/gitlab"
	"github.com/getmac-io/getmac-sdk-golang"
	"github.com/spf13/cobra"
)

func NewCleanupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Cleanup command to remove the virtual machine after job completion",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runCleanupCommand(cmd, args)
			if err != nil {
				return gitlab.NewSystemFailureError(err)
			}

			return nil
		},
		Args: cobra.NoArgs,
	}

	return cmd
}

func runCleanupCommand(cmd *cobra.Command, _ []string) error {
	env, err := gitlab.NewEnvironment()
	if err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	client := getmac.NewClient(
		getmac.WithToken(env.Token), getmac.WithBaseURL(env.URL))

	name := fmt.Sprintf("gitlab-job-%s", env.JobID)
	_, vm, err := client.VirtualMachines().GetByName(cmd.Context(), env.ProjectID, name)
	switch {
	case errors.Is(err, getmac.ErrNotFound):
		// GitLab Runner runs cleanup even when prepare failed before creating the
		// virtual machine, so there may be nothing to delete.
		slog.Info("No virtual machine to delete", "name", name)
		return nil
	case err != nil:
		return fmt.Errorf("failed to get virtual machine by name: %w", err)
	}

	_, err = client.VirtualMachines().Delete(cmd.Context(), env.ProjectID, vm.ID)
	if err != nil && !errors.Is(err, getmac.ErrNotFound) {
		return fmt.Errorf("failed to delete virtual machine: %w", err)
	}

	return nil
}
