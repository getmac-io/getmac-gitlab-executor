# getmac-gitlab-executor

[Custom GitLab Runner executor](https://docs.gitlab.com/runner/executors/custom.html) to run CI/CD jobs in ephemeral [GetMac](https://getmac.io) virtual machines.

Each job gets a fresh macOS virtual machine: the executor creates it in the `prepare` stage, runs the job's script in it over SSH, and deletes it in the `cleanup` stage.

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
      "GETMAC_CLOUD_SSH_PRIVATE_KEY_PATH=/Users/username/.ssh/getmac",
      "GETMAC_CLOUD_PROJECT_ID=2f8aa35f-b1d7-4425-bb26-889dfe92cb53"
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
| `GETMAC_CLOUD_MACHINE_IMAGE`     | ❌      | `getmac`                 | Which macOS image the job runs on: a GetMac **runner label** such as `getmac-tahoe` (recommended) or an image slug such as `macos-tahoe`. See [Choosing the macOS Image](#choosing-the-macos-image). |
| `GETMAC_CLOUD_MACHINE_TYPE`      | ❌      | — (from the runner label) | Machine type, for example `mac-m4-c4-m8`. Leave it unset to use the type of the runner label. Required when `GETMAC_CLOUD_MACHINE_IMAGE` is an image slug. |
| `GETMAC_CLOUD_REGION`            | ❌      | `eu-central-ltu-1`       | VM region                                   |
| `GETMAC_CLOUD_SSH_PRIVATE_KEY_PATH` | ❌   | `$HOME/.ssh/id_rsa`      | SSH private key path                        |
| `GETMAC_CLOUD_DEBUG`             | ❌      | —                       | Enable debug logging (`true`/`false`)       |
| `GETMAC_PROXY_TUNNEL_ENABLED`    | ❌      | `false`                  | Enable reverse SSH tunnel HTTP proxy        |
| `GETMAC_PROXY_TUNNEL_PORT`       | ❌      | `8080`                   | Port on the VM for proxied HTTP traffic     |
| `GETMAC_CLOUD_VM_READY_TIMEOUT`  | ❌      | `20m`                    | How long `prepare` waits for the VM to start (Go duration, e.g. `30m`) |
| `GETMAC_CLOUD_SSH_READY_TIMEOUT` | ❌      | `5m`                     | How long to retry the SSH connection once the VM is running |

> **Note:** You can set the `GETMAC_CLOUD_API_KEY` environment variable via the `config --getmac-cloud-api-key` command to prevent it from appearing in job logs.

#### Where to set the variables

You can set these variables in two places:

- **Runner-wide**, in the `environment` list of the runner's `config.toml`. Every job the runner picks up gets them.
- **Per project or per job**, as CI/CD variables: in `.gitlab-ci.yml` (global or job-level `variables:`) or in the project's CI/CD settings.

Use the plain names shown in the table in both places, **without** a `CUSTOM_ENV_` prefix. GitLab Runner passes job variables, including the runner's `environment` settings, to custom executors with the `CUSTOM_ENV_` prefix already added, and that's the name the executor reads. A variable you name `CUSTOM_ENV_GETMAC_CLOUD_PROJECT_ID` yourself reaches the executor as `CUSTOM_ENV_CUSTOM_ENV_GETMAC_CLOUD_PROJECT_ID` and is ignored.

To keep it clear which value applies, set each variable in only one of these places.

## Choosing the macOS Image

> **Changed:** runner labels are now supported, and the default image is the `getmac` runner label instead of the `macos-sequoia` image with the `mac-m4-c4-m8` machine type. Jobs that don't set `GETMAC_CLOUD_MACHINE_IMAGE` now run on whatever `getmac` points to, which is currently macOS Tahoe 26.5.2 with Xcode 26.6. Read [Upgrading from an earlier version](#upgrading-from-an-earlier-version) before you update.
>
> Runner labels need a GetMac API version with label support. See [Errors](#errors) for what you'll see if the API doesn't support them yet.

### Runner tags and runner labels are different things

Two settings in a GitLab setup look similar but do unrelated jobs:

| | GitLab runner **tag** | GetMac runner **label** |
|---|---|---|
| Where you set it | `tags:` in `.gitlab-ci.yml`, and on the runner when you register it | `GETMAC_CLOUD_MACHINE_IMAGE` |
| Who reads it | GitLab, to decide **which runner** picks up the job | The GetMac API, to decide **which macOS image and machine type** the VM gets |
| Example | `getmac` | `getmac`, `getmac-tahoe`, `getmac-sequoia` |

The executor never reads the job's tags. A job with `tags: [getmac]` runs on whatever image `GETMAC_CLOUD_MACHINE_IMAGE` selects, and changing the tag to `getmac-tahoe` doesn't change the image. It only means GitLab looks for a runner registered with a `getmac-tahoe` tag, and the job waits if there isn't one.

### Runner labels (recommended)

`GETMAC_CLOUD_MACHINE_IMAGE` accepts the same labels as `runs-on` in GetMac's GitHub Actions runners, so one name means the same macOS image on both CI systems. **The current list of labels, with their macOS and Xcode versions, is on the [Workflow Labels](https://getmac.io/docs/runner-images/workflow-labels) page.**

At the time of writing (September 2026) the labels are:

| Label | macOS | Xcode |
| --- | --- | --- |
| `getmac` | Tahoe (26.5.2) | 26.6 |
| `getmac-latest` | Tahoe (26.5.2) | 26.6 |
| `getmac-tahoe` | Tahoe (26.5.2) | 26.6 |
| `getmac-tahoe-legacy` | Tahoe (26.3.2) | 26.4 |
| `getmac-sequoia` | Sequoia (15.6.1) | 16.4 |

GetMac maintains which image each label points to and moves labels to new builds over time. For example, `getmac` and `getmac-latest` always point to the most recent stable image. Pick the label that matches how much change you want:

- `getmac` or `getmac-latest` to always get the latest stable macOS and Xcode.
- A version label such as `getmac-tahoe` or `getmac-sequoia` to stay on one macOS release while still getting its updates.
- A legacy label such as `getmac-tahoe-legacy` to stay on the previous build of a release, for example when your project needs the older Xcode.

A label also decides the machine type, so leave `GETMAC_CLOUD_MACHINE_TYPE` unset unless you want a different one.

### Image slugs (advanced)

`GETMAC_CLOUD_MACHINE_IMAGE` also accepts an image slug, the name of one specific image, for example `macos-tahoe`, `macos-tahoe-legacy` or `macos-sequoia`. Use a slug only if you need to bypass the label mapping or your GetMac API doesn't support labels yet.

An image slug doesn't imply a machine type, so you must set `GETMAC_CLOUD_MACHINE_TYPE` with it:

```yaml
variables:
  GETMAC_CLOUD_MACHINE_IMAGE: "macos-tahoe"
  GETMAC_CLOUD_MACHINE_TYPE: "mac-m4-c4-m8"
```

### How the image is resolved

For every job, the `prepare` stage sends the GetMac API the value of `GETMAC_CLOUD_MACHINE_IMAGE`, the value of `GETMAC_CLOUD_MACHINE_TYPE` (empty if unset) and the region. The API then:

1. Looks for an **image slug** with that name in the region. If it finds one, the VM uses that image, and the machine type must be set.
2. Otherwise, looks for an enabled **runner label** with that name. If it finds one, the VM uses the label's image and the label's machine type. A `GETMAC_CLOUD_MACHINE_TYPE` you set replaces the label's machine type. The label's image must be available in the region.
3. Otherwise, rejects the request with `Image or label <name> not found`.

The job log shows what you asked for and what the VM got. With a runner label:

```text
INFO Creating virtual machine... image=getmac-tahoe type="(from image label)" region=eu-central-ltu-1
INFO Virtual machine created id=8f95fcae-... image=macos-tahoe type=mac-m4-c4-m8
```

With an image slug and machine type:

```text
INFO Creating virtual machine... image=macos-tahoe type=mac-m4-c4-m8 region=eu-central-ltu-1
INFO Virtual machine created id=8f95fcae-... image=macos-tahoe type=mac-m4-c4-m8
```

### Examples

Use the default `getmac` label by not setting anything:

```yaml
build:
  tags:
    - getmac
  script:
    - xcodebuild -version
```

Pin one macOS release for the whole pipeline:

```yaml
variables:
  GETMAC_CLOUD_MACHINE_IMAGE: "getmac-tahoe"
```

Run jobs on different macOS versions in one pipeline:

```yaml
test-tahoe:
  tags:
    - getmac
  variables:
    GETMAC_CLOUD_MACHINE_IMAGE: "getmac-tahoe"
  script:
    - xcodebuild test -scheme App -destination 'platform=iOS Simulator,name=iPhone 17'

test-sequoia:
  tags:
    - getmac
  variables:
    GETMAC_CLOUD_MACHINE_IMAGE: "getmac-sequoia"
  script:
    - xcodebuild test -scheme App -destination 'platform=iOS Simulator,name=iPhone 16'
```

Set a default for every job on a runner, in `config.toml`:

```toml
[[runners]]
  executor = "custom"
  environment = [
    "GETMAC_CLOUD_PROJECT_ID=2f8aa35f-b1d7-4425-bb26-889dfe92cb53",
    "GETMAC_CLOUD_MACHINE_IMAGE=getmac-sequoia"
  ]
```

Use a label but a different machine type:

```yaml
variables:
  GETMAC_CLOUD_MACHINE_IMAGE: "getmac-tahoe"
  GETMAC_CLOUD_MACHINE_TYPE: "mac-m4-c4-m8"
```

### Errors

When the API rejects the image or machine type, `prepare` fails with the API's reason in the job log:

```text
INFO Creating virtual machine... image=getmac-nope type="(from image label)" region=eu-central-ltu-1
Error: failed to create virtual machine: unexpected status code: 400: Image or label getmac-nope not found
ERROR: Preparation failed: exit status 2
Will be retried in 3s ...
```

GitLab Runner treats a failed `prepare` as a system failure. It retries the stage up to three times and then fails the job with `Job failed (system failure)`. No virtual machine is created, so there's nothing to clean up.

| Message in the job log | What it means | What to do |
|---|---|---|
| `Image or label <name> not found` | The name is neither an image slug nor an enabled runner label. The label may be misspelled, removed, or retired. | Pick a label from the [Workflow Labels](https://getmac.io/docs/runner-images/workflow-labels) page. |
| `Label <name> is not available in region <region>` | The label exists, but its image or machine type isn't offered in `GETMAC_CLOUD_REGION`. | Use another label, or unset `GETMAC_CLOUD_REGION` to use the default region. |
| `Instance type is required when image <name> is an image slug. Set a type, or use a runner label instead` | `GETMAC_CLOUD_MACHINE_IMAGE` is an image slug and `GETMAC_CLOUD_MACHINE_TYPE` is unset. | Set `GETMAC_CLOUD_MACHINE_TYPE`, or switch to a runner label. |
| `Instance type <name> not found` | `GETMAC_CLOUD_MACHINE_TYPE` isn't a machine type in the region. | Fix or unset `GETMAC_CLOUD_MACHINE_TYPE`. |
| `Invalid request payload: type is required` | The GetMac API doesn't support runner labels yet, and no machine type is set. This is what the default `getmac` label gets from such an API. | Use an image slug with a machine type until label support is available (see below). |
| `Image getmac-tahoe not found` (without "or label") | The GetMac API doesn't support runner labels yet, and a machine type is set. | Same as above. |

To keep jobs running against a GetMac API without label support, set the previous defaults explicitly:

```yaml
variables:
  GETMAC_CLOUD_MACHINE_IMAGE: "macos-sequoia"
  GETMAC_CLOUD_MACHINE_TYPE: "mac-m4-c4-m8"
```

### Upgrading from an earlier version

Check these changes before you update the executor on your runners:

- **The default image changed.** Before: the `macos-sequoia` image (macOS Sequoia 15.6.1, Xcode 16.4) on the `mac-m4-c4-m8` machine type. Now: the `getmac` runner label, currently macOS Tahoe 26.5.2 with Xcode 26.6, which follows the latest stable image from now on. Jobs that don't set `GETMAC_CLOUD_MACHINE_IMAGE` get a new macOS and Xcode version. To stay on Sequoia, set `GETMAC_CLOUD_MACHINE_IMAGE=getmac-sequoia`.
- **`GETMAC_CLOUD_MACHINE_TYPE` has no default anymore.** With a runner label, the label's machine type applies. If you set `GETMAC_CLOUD_MACHINE_IMAGE` to an image slug such as `macos-sequoia` and relied on the old `mac-m4-c4-m8` default, add `GETMAC_CLOUD_MACHINE_TYPE=mac-m4-c4-m8` or switch to the matching runner label, such as `getmac-sequoia`.
- **Runner labels need GetMac API support.** Against an API without it, the new default fails with `Invalid request payload: type is required`. Set an image slug and machine type as shown in [Errors](#errors) until label support is available.
- **Remove `CUSTOM_ENV_` from variable names in `config.toml`.** Earlier versions of this README showed `CUSTOM_ENV_GETMAC_CLOUD_...` in the runner's `environment` list. GitLab Runner adds that prefix itself, so those settings were never read. Use the plain names, as explained in [Where to set the variables](#where-to-set-the-variables).
- **API errors are more specific.** Messages that used to read `unexpected status code: 400` now include the reason, for example `unexpected status code: 400: Image or label getmac-nope not found`.
- **`cleanup` no longer fails when there's no virtual machine.** When `prepare` failed before creating one, `cleanup` logs `No virtual machine to delete` instead of GitLab Runner's `Cleanup script failed` warning.
- **A missing virtual machine in `run` is a system failure.** If the job's virtual machine is gone, for example because it was deleted outside the job, the job fails as a system failure instead of a script failure.

## Virtual Machine Startup

The `prepare` stage creates the virtual machine and waits until it's ready. It polls the GetMac API until the VM reports `running`, logging each status change with its reason (for example, when the VM is queued because a project limit was reached). Then it retries SSH until the VM accepts a session. The stage fails right away if the VM reports `error` or gets deleted.

Each `run` stage also retries its SSH connection for up to `GETMAC_CLOUD_SSH_READY_TIMEOUT`.

Keep GitLab Runner's `prepare_exec_timeout` (default: 3600 seconds) above `GETMAC_CLOUD_VM_READY_TIMEOUT` plus `GETMAC_CLOUD_SSH_READY_TIMEOUT`, or the runner will stop the stage before the executor reports why the VM isn't ready.

The `cleanup` stage deletes the job's virtual machine. It succeeds when there's nothing to delete, for example when `prepare` failed before creating the VM.

## Proxy Tunnel

When running against a self-hosted GitLab instance or other internal services that the ephemeral VM cannot reach directly, you can enable a reverse SSH tunnel HTTP proxy. This routes the VM's HTTP/HTTPS traffic back through the Runner host, which has network access.

Add `--enable-proxy-tunnel` to the `config_args` in your `config.toml`:

```toml
[[runners]]
  executor = "custom"
  environment = [
    "GETMAC_CLOUD_SSH_PRIVATE_KEY_PATH=/Users/username/.ssh/getmac",
    "GETMAC_CLOUD_PROJECT_ID=2f8aa35f-b1d7-4425-bb26-889dfe92cb53",
    "GETMAC_CLOUD_DEBUG=true"
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

You can also specify a custom port with `--proxy-tunnel-port <port>` (default: `8080`). Instead of the flags, you can set `GETMAC_PROXY_TUNNEL_ENABLED=true` (and `GETMAC_PROXY_TUNNEL_PORT`) in the runner's `environment`.

When enabled, the executor automatically:
1. Starts a local HTTP forward proxy on the Runner host
2. Opens a reverse SSH tunnel so the VM can reach the proxy at `127.0.0.1:8080`
3. Sets `HTTP_PROXY`, `HTTPS_PROXY`, `http_proxy`, and `https_proxy` environment variables in the job script

## Update Checks

The executor tells you when a newer release is available. It logs a warning in the job log during the `config` stage and does nothing else:

```text
WARN A newer version of getmac-gitlab-executor is available current=0.0.4 latest=0.1.0 upgrade=https://github.com/getmac-io/getmac-gitlab-executor/releases/latest
```

It never downloads or replaces the binary. Upgrading stays something you do deliberately, so a new release can't change how your pipelines behave until you choose it, and your runners never execute code fetched at job time.

The check is designed to stay out of the way:

- **It can't fail a job.** Every error ends in silence. If GitHub is unreachable, rate-limited, or slow, the executor carries on.
- **It can't stall a job.** The request is bounded by a 3-second timeout.
- **It won't exhaust GitHub's rate limit.** GitHub is queried at most once every 24 hours and the answer is cached, so the warning still appears in every job log without an API call per job.
- **Development builds never check**, so local builds don't nag.

To turn it off, add `--disable-update-check` to `config_args` in your `config.toml`:

```toml
config_args = ["config", "--getmac-cloud-api-key", "<API_KEY>", "--disable-update-check"]
```

## Example `.gitlab-ci.yml`

```yaml
variables:
  GETMAC_CLOUD_PROJECT_ID: "2f8aa35f-b1d7-4425-bb26-889dfe92cb53"
  # Optional: GetMac runner label, see https://getmac.io/docs/runner-images/workflow-labels
  # Defaults to getmac, the latest stable macOS image.
  GETMAC_CLOUD_MACHINE_IMAGE: "getmac-tahoe"

stages:
  - build

build-job:
  stage: build
  tags:
    - getmac # GitLab runner tag: selects the runner, not the image
  script:
    - echo "Building the project..."
```

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
