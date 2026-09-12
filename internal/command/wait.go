package command

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/getmac-io/getmac-sdk-golang"
)

const (
	vmPollInterval   = 3 * time.Second
	vmRequestTimeout = 15 * time.Second
)

// vmFailedStatuses are statuses from which a new virtual machine won't become
// running on its own, so waiting any longer would only use up the job timeout.
var vmFailedStatuses = map[string]bool{
	"error":     true,
	"stopping":  true,
	"stopped":   true,
	"suspended": true,
	"deleting":  true,
	"deleted":   true,
}

type waitOptions struct {
	Timeout        time.Duration
	PollInterval   time.Duration
	RequestTimeout time.Duration
}

// waitForVirtualMachineRunning polls the virtual machine until the API reports it
// as running. A new virtual machine is queued, placed on a host, cloned and
// started first, and how long that takes depends on project quota and host
// capacity, so a fixed delay can't cover it.
func waitForVirtualMachineRunning(
	ctx context.Context, client *getmac.Client, projectID, vmID string, opts waitOptions) (*getmac.VirtualMachine, error) {
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	start := time.Now()
	var last *getmac.VirtualMachine
	var lastErr error

	for {
		reqCtx, reqCancel := context.WithTimeout(ctx, opts.RequestTimeout)
		resp, vm, err := client.VirtualMachines().Get(reqCtx, projectID, vmID)
		reqCancel()

		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				return nil, fmt.Errorf("virtual machine %s no longer exists", vmID)
			}

			if resp != nil && !isRetryableStatusCode(resp.StatusCode) {
				return nil, fmt.Errorf("failed to get virtual machine %s: %w", vmID, err)
			}

			// A request cut short by the overall deadline or cancellation says nothing
			// about the API, so keep the earlier error in that case.
			if ctx.Err() == nil || lastErr == nil {
				if ctx.Err() == nil && (lastErr == nil || lastErr.Error() != err.Error()) {
					slog.Warn("Failed to get virtual machine status, retrying", "id", vmID, "error", err)
				}
				lastErr = err
			}
		} else {
			lastErr = nil
			if last == nil || vm.Status != last.Status || vm.StatusReason != last.StatusReason {
				logVirtualMachineStatus(vm, time.Since(start))
			}
			last = vm

			if vm.Status == "running" {
				return vm, nil
			}

			if vmFailedStatuses[vm.Status] {
				return nil, fmt.Errorf("virtual machine %s is %s", vmID, describeStatus(vm))
			}
		}

		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("stopped waiting for virtual machine %s: %w", vmID, ctx.Err())
			}

			if last == nil {
				return nil, fmt.Errorf("virtual machine %s is not running after %s: %w", vmID, opts.Timeout, lastErr)
			}

			return nil, fmt.Errorf("virtual machine %s is not running after %s (last status: %s)",
				vmID, opts.Timeout, describeStatus(last))
		case <-time.After(opts.PollInterval):
		}
	}
}

func isRetryableStatusCode(code int) bool {
	return code >= http.StatusInternalServerError ||
		code == http.StatusRequestTimeout ||
		code == http.StatusTooManyRequests
}

func describeStatus(vm *getmac.VirtualMachine) string {
	if vm.StatusReason == "" {
		return vm.Status
	}

	return fmt.Sprintf("%s (%s)", vm.Status, vm.StatusReason)
}

func logVirtualMachineStatus(vm *getmac.VirtualMachine, elapsed time.Duration) {
	args := []any{"id", vm.ID, "status", vm.Status, "elapsed", elapsed.Round(time.Second).String()}
	if vm.StatusReason != "" {
		args = append(args, "reason", vm.StatusReason)
	}

	slog.Info("Virtual machine status", args...)
}
