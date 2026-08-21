# Installation

The latest stable release is **orch v0.3.0**, published on 21 August 2026. Release archives contain the `orch` CLI, `orch-server`, and `orch-agent`. The automated installer installs only the `orch` CLI.

## Supported Release Platforms

| Operating system | Architectures | Archive |
| --- | --- | --- |
| Linux | `amd64`, `arm64` | `.tar.gz` |
| macOS | `amd64`, `arm64` | `.tar.gz` |
| Windows | `amd64`, `arm64` | `.zip` |

All artifacts and `checksums.txt` are published on the [v0.3.0 GitHub release](https://github.com/alekpopovic/orch/releases/tag/0.3.0).

## Install The CLI On Linux Or macOS

The release installer requires `curl`, `tar`, and either `sha256sum` or `shasum`. It detects the operating system and CPU architecture, downloads the matching archive, verifies the SHA-256 checksum, and installs `orch`.

Install the latest stable CLI to `/usr/local/bin`:

```sh
curl -fsSL https://github.com/alekpopovic/orch/releases/latest/download/install.sh | sh
orch version
```

If `/usr/local/bin` is not writable, the installer uses `sudo`. To install v0.3.0 in a user-owned directory:

```sh
mkdir -p "$HOME/.local/bin"
curl -fsSL https://github.com/alekpopovic/orch/releases/download/0.3.0/install.sh | \
  ORCH_VERSION=0.3.0 ORCH_INSTALL_DIR="$HOME/.local/bin" sh
export PATH="$HOME/.local/bin:$PATH"
orch version
```

For a permanent PATH update, add the export to your shell profile.

### Review Before Running

To inspect the installer before execution:

```sh
curl -fsSLo install-orch.sh https://github.com/alekpopovic/orch/releases/download/0.3.0/install.sh
less install-orch.sh
ORCH_VERSION=0.3.0 ORCH_INSTALL_DIR="$HOME/.local/bin" sh install-orch.sh
```

Installer environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `ORCH_VERSION` | latest GitHub release | Pin a tag such as `0.3.0`. |
| `ORCH_INSTALL_DIR` | `/usr/local/bin` | Select the target directory. |
| `ORCH_REPOSITORY` | `alekpopovic/orch` | Select a release repository. |
| `ORCH_DOWNLOAD_BASE` | release download URL | Override the artifact source, for example with an internal mirror. |

## Install Manually On Linux Or macOS

Choose the archive for your platform from the release page. This Linux `amd64` example pins v0.3.0 and verifies the published checksum:

```sh
version=0.3.0
archive="orch_${version}_linux_amd64.tar.gz"
base="https://github.com/alekpopovic/orch/releases/download/${version}"

curl -fsSLO "${base}/${archive}"
curl -fsSLO "${base}/checksums.txt"
grep " ${archive}$" checksums.txt | sha256sum --check -
tar -xzf "$archive"
sudo install -m 0755 orch /usr/local/bin/orch
orch version
```

On macOS, use `darwin_amd64` or `darwin_arm64` and replace the checksum command with:

```sh
expected="$(awk -v name="$archive" '$2 == name { print $1 }' checksums.txt)"
actual="$(shasum -a 256 "$archive" | awk '{ print $1 }')"
test "$actual" = "$expected"
```

## Install On Windows

Download `orch_0.3.0_windows_amd64.zip` or `orch_0.3.0_windows_arm64.zip` from the [release page](https://github.com/alekpopovic/orch/releases/tag/0.3.0), then verify and extract it in PowerShell:

```powershell
$Version = "0.3.0"
$Archive = "orch_${Version}_windows_amd64.zip"
$Base = "https://github.com/alekpopovic/orch/releases/download/$Version"

Invoke-WebRequest "$Base/$Archive" -OutFile $Archive
Invoke-WebRequest "$Base/checksums.txt" -OutFile checksums.txt
$Expected = ((Select-String -Path checksums.txt -Pattern " $Archive$").Line -split '\s+')[0]
$Actual = (Get-FileHash -Algorithm SHA256 $Archive).Hash.ToLowerInvariant()
if ($Actual -ne $Expected) { throw "checksum verification failed" }

Expand-Archive $Archive -DestinationPath .\orch-$Version
```

Move `orch.exe` into a directory on `PATH`, then verify with `orch version`.

## Install Server And Agent Binaries

Manual archives contain all three production binaries. After checksum verification and extraction on a Linux host:

```sh
sudo install -m 0755 orch-server /usr/local/bin/orch-server
sudo install -m 0755 orch-agent /usr/local/bin/orch-agent
test -x /usr/local/bin/orch-server
test -x /usr/local/bin/orch-agent
```

Do not start these binaries with development defaults in production. Continue with [Production Deployment](https://alekpopovic.github.io/orch/#PRODUCTION_DEPLOYMENT.md), [Configuration](https://alekpopovic.github.io/orch/#CONFIGURATION.md), and [Upgrades](https://alekpopovic.github.io/orch/#UPGRADES.md).

## Build From Source

Building requires Go 1.25 or newer:

```sh
git clone https://github.com/alekpopovic/orch.git
cd orch
git checkout 0.3.0
go build -o ./bin/orch ./cmd/orch
go build -o ./bin/orch-server ./cmd/orch-server
go build -o ./bin/orch-agent ./cmd/orch-agent
```

## Configure And Verify The CLI

Set the control-plane address and, when authentication is enabled, a user JWT:

```sh
export ORCH_SERVER_URL=https://orch.example.com
export ORCH_TOKEN='<jwt>'
export ORCH_NAMESPACE=default

orch version
orch cluster check-upgrade
orch node ls
```

See the [CLI reference](https://alekpopovic.github.io/orch/#CLI.md) for configuration precedence and all commands.

## Upgrade

Re-run the installer without `ORCH_VERSION` to install the latest stable CLI, or set an explicit version for reproducible automation. Before upgrading a cluster, always run:

```sh
orch cluster check-upgrade
orch-server migrate status --database-url "$DATABASE_URL"
```

Follow the [cluster upgrade order](https://alekpopovic.github.io/orch/#UPGRADES.md). The v0.3.0 server accepts agents from v0.2.0 through v0.3.0 and schema versions 15 through 16.

## Uninstall

Remove only the binaries you installed. For the default CLI location:

```sh
sudo rm /usr/local/bin/orch
```

If you installed server or agent processes, stop and disable their services before removing the binaries. Preserve configuration, PostgreSQL data, encryption keys, and backups unless you intentionally want to destroy the cluster.
