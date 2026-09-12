# getmac-gitlab-executor

[Custom GitLab Runner executor](https://docs.gitlab.com/runner/executors/custom.html) to run CI/CD jobs in ephemeral [GetMac](https://getmac.io) virtual machines.

## Prerequisites

- A [GetMac](https://getmac.io) account. You can sign up for a free account if you don't have one.
- GetMac API key. You can create an API key in the [GetMac Dashboard](https://cloud.getmac.io/api-keys). Make sure to copy the key as you won't be able to see it again.

## Configuration

1. Download the [GitLab Runner](https://docs.gitlab.com/runner/install/) and install it on your machine. Follow the [official installation guide](https://docs.gitlab.com/runner/install/) for your operating system.

2. Register the [GitLab Runner](https://docs.gitlab.com/runner/register/) with your GitLab instance. Use the `custom` executor during registration.

3. Create a new SSH key pair or use an existing one. Add the public key in the [GetMac Dashboard](https://cloud.getmac.io/ssh-keys). This key will be used to access the virtual machine. Set the path to the private key using the `GETMAC_CLOUD_SSH_PRIVATE_KEY_PATH` environment variable if it's not the default path (`$HOME/.ssh/id_rsa`).

4. Add the following configuration to the `[[runners]]` section in your GitLab Runner's `config.toml` file:

  ```toml
  [[runners]]
    executor = "custom"
    environment = [
      "CUSTOM_ENV_GETMAC_CLOUD_SSH_PRIVATE_KEY_PATH=/Users/username/.ssh/getmac",
      "CUSTOM_ENV_GETMAC_CLOUD_PROJECT_ID=2f8aa35f-b1d7-4425-bb26-889dfe92cb53"
    ]

    [runners.custom]
      config_exec  = "getmac-gitlab-executor"
      config_args  = ["config", "--getmac-cloud-api-key", "<API_KEY>"]
      prepare_exec = "getmac-gitlab-executor"
      prepare_args = ["prepare"]
      run_exec     = "getmac-gitlab-executor"
      run_args     = ["run"]
      cleanup_exec = "getmac-gitlab-executor"
      cleanup_args = ["cleanup"]
  ```

### Environment Variables

The executor uses the following environment variables for configuration:

| Variable                         | Required | Default                 | Description                                 |
|----------------------------------|---------|--------------------------|---------------------------------------------|
| `GETMAC_CLOUD_API_URL`           | ✅      | https://api.getmac.io/v1 | GetMac API URL                              |
| `GETMAC_CLOUD_API_KEY`           | ✅      | —                       | GetMac API key. You can set this via `config --getmac-cloud-api-key` as well to prevent it from appearing in job logs. |
| `GETMAC_CLOUD_PROJECT_ID`        | ✅      | —                       | GetMac project ID                           |
| `GETMAC_CLOUD_MACHINE_IMAGE`     | ❌      | `macos-sequoia`          | VM image name                               |
| `GETMAC_CLOUD_MACHINE_TYPE`      | ❌      | `mac-m4-c4-m8`           | VM type                                     |
| `GETMAC_CLOUD_REGION`            | ❌      | `eu-central-ltu-1`       | VM region                                   |
| `GETMAC_CLOUD_SSH_PRIVATE_KEY_PATH` | ❌   | `$HOME/.ssh/id_rsa`      | SSH private key path                        |
| `GETMAC_CLOUD_DEBUG`             | ❌      | —                       | Enable debug logging (`true`/`false`)       |
| `GETMAC_PROXY_TUNNEL_ENABLED`    | ❌      | `false`                  | Enable reverse SSH tunnel HTTP proxy        |
| `GETMAC_PROXY_TUNNEL_PORT`       | ❌      | `8080`                   | Port on the VM for proxied HTTP traffic     |
| `GETMAC_CLOUD_VM_READY_TIMEOUT`  | ❌      | `20m`                    | How long `prepare` waits for the VM to start (Go duration, e.g. `30m`) |
| `GETMAC_CLOUD_SSH_READY_TIMEOUT` | ❌      | `5m`                     | How long to retry the SSH connection once the VM is running |

> **Note:** You can set the `GETMAC_CLOUD_API_KEY` environment variable via the `config --getmac-cloud-api-key` command to prevent it from appearing in job logs.

### Virtual Machine Startup

The `prepare` stage creates the virtual machine and waits until it's ready. It polls the GetMac API until the VM reports `running`, logging each status change with its reason (for example, when the VM is queued because a project limit was reached). Then it retries SSH until the VM accepts a session. The stage fails right away if the VM reports `error` or gets deleted.

Each `run` stage also retries its SSH connection for up to `GETMAC_CLOUD_SSH_READY_TIMEOUT`.

Keep GitLab Runner's `prepare_exec_timeout` (default: 3600 seconds) above `GETMAC_CLOUD_VM_READY_TIMEOUT` plus `GETMAC_CLOUD_SSH_READY_TIMEOUT`, or the runner will stop the stage before the executor reports why the VM isn't ready.

### Proxy Tunnel

When running against a self-hosted GitLab instance or other internal services that the ephemeral VM cannot reach directly, you can enable a reverse SSH tunnel HTTP proxy. This routes the VM's HTTP/HTTPS traffic back through the Runner host, which has network access.

Add `--enable-proxy-tunnel` to the `config_args` in your `config.toml`:

```toml
[[runners]]
  executor = "custom"
  environment = [
    "CUSTOM_ENV_GETMAC_CLOUD_SSH_PRIVATE_KEY_PATH=/Users/username/.ssh/getmac",
    "CUSTOM_ENV_GETMAC_CLOUD_PROJECT_ID=2f8aa35f-b1d7-4425-bb26-889dfe92cb53",
    "CUSTOM_ENV_GETMAC_CLOUD_DEBUG=true"
  ]

  [runners.custom]
    config_exec  = "getmac-gitlab-executor"
    config_args  = ["config", "--getmac-cloud-api-key", "<API_KEY>", "--enable-proxy-tunnel", "--proxy-tunnel-port", "8080"]
    prepare_exec = "getmac-gitlab-executor"
    prepare_args = ["prepare"]
    run_exec     = "getmac-gitlab-executor"
    run_args     = ["run"]
    cleanup_exec = "getmac-gitlab-executor"
    cleanup_args = ["cleanup"]
```

You can also specify a custom port with `--proxy-tunnel-port <port>` (default: `8080`).

When enabled, the executor automatically:
1. Starts a local HTTP forward proxy on the Runner host
2. Opens a reverse SSH tunnel so the VM can reach the proxy at `127.0.0.1:8080`
3. Sets `HTTP_PROXY`, `HTTPS_PROXY`, `http_proxy`, and `https_proxy` environment variables in the job script

## Example `.gitlab-ci.yml`

```yaml
variables:
  GETMAC_CLOUD_PROJECT_ID: "2f8aa35f-b1d7-4425-bb26-889dfe92cb53"

stages:
  - build

build-job:
  stage: build
  tags:
    - getmac
  script:
    - echo "Building the project..."
```

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
