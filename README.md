# getmac-gitlab-executor

[Custom GitLab Runner executor](https://docs.gitlab.com/runner/executors/custom.html) to run CI/CD jobs in ephemeral [GetMac](https://getmac.io) virtual machines.

## Prerequisites

- A [GetMac](https://getmac.io) account. You can sign up for a free account if you don't have one.
- GetMac API key. You can create an API key in the [GetMac Dashboard](https://cloud.getmac.io/api-keys). Make sure to copy the key as you won't be able to see it again.

## Configuration

1. Download the [GitLab Runner](https://docs.gitlab.com/runner/install/) and install it on your machine. Follow the [official installation guide](https://docs.gitlab.com/runner/install/) for your operating system.

2. Register the [GitLab Runner](https://docs.gitlab.com/runner/register/) with your GitLab instance. Use the `custom` executor during registration.

3. Add the following configuration to `[runners.custom]` section in your GitLab Runner's `config.toml` file:

  ```toml
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

4. Set `executor = "custom"` in the `[[runners]]` section in your GitLab Runner's `config.toml` file.

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

> **Note:** You can set the `GETMAC_CLOUD_API_KEY` environment variable via the `config --getmac-cloud-api-key` command to prevent it from appearing in job logs.

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
