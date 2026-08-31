# Install and maintain sttd

This release is supported on Linux `amd64`. It includes the `sttd`, `sttdctl`, and `whisper.cpp` runtime binaries; the transcription model is downloaded during installation.

## Requirements

- An active graphical session.
- PipeWire and `pw-record`.
- X11: `xclip` and `xdotool`.
- Wayland: `wl-copy` and `wtype`.
- `curl` to download the model.
- `systemctl --user` only when using `--as-service`.

The installer validates these requirements before changing files. You can check them separately with:

```bash
./install.sh --check --profile linux
```

## Installation

The recommended guided flow for a new installation is:

```bash
curl -fsSL https://raw.githubusercontent.com/LDTorres/speech-to-text-sv/main/scripts/bootstrap-install.sh | bash -s -- --interactive
```

The bootstrap downloads the latest published release, verifies its SHA-256 checksum, and runs the packaged installer. If you prefer to review the script first, download the file and run it locally. You can also pin a version with `--version v0.1.5`. The installer never requires root privileges.

The default download is the CPU-only package. NVIDIA users can request the
larger CUDA package:

```bash
curl -fsSL https://raw.githubusercontent.com/LDTorres/speech-to-text-sv/main/scripts/bootstrap-install.sh \
  | bash -s -- --interactive --acceleration cuda
```

The CUDA package requires a compatible NVIDIA driver.

Download the `sttd-<version>-linux-amd64.tar.gz` archive and its `.sha256` file. For CUDA, use the `-cuda.tar.gz` archive instead. Verify the archive first:

```bash
sha256sum -c sttd-<version>-linux-amd64.tar.gz.sha256
tar -xzf sttd-<version>-linux-amd64.tar.gz
cd sttd-<version>-linux-amd64
```

Install the general Linux profile:

```bash
./install.sh --profile linux --language es --acceleration auto --as-service
```

Install for Steam Deck:

```bash
./install.sh --profile steam_deck --language es --acceleration auto --as-service
```

Without flags, `./install.sh` opens an interactive setup that asks for the profile, integration, model, language, service, public command name, and optional Hyprland bindings. For automation, use `--non-interactive` and pass the options explicitly.

The CPU package accepts `--acceleration auto` or `--acceleration cpu`. The CUDA package must be installed with `--acceleration cuda`.

By default, the active installation is stored in `~/.local/opt/sttd`. The systemd service points to this stable location, so upgrades do not break unit paths.

To install without a service:

```bash
./install.sh --profile linux --language es
~/.local/opt/sttd/sttd
```

Stop the manually started process with `Ctrl+C`.

## Verification and diagnostics

After installing as a service:

```bash
~/.local/opt/sttd/sttdctl service status
~/.local/opt/sttd/sttdctl logs tail --lines 200
~/.local/opt/sttd/doctor.sh --status
~/.local/opt/sttd/sttdctl doctor
```

The installation creates `~/.local/bin/listen`. If `~/.local/bin` is in `PATH`, the main commands are:

```bash
listen status
listen record start
listen record stop
listen record retry
listen model
~/.local/opt/sttd/sttdctl model list
listen doctor
listen logs tail
```

You can choose another name with `--command-name whisper`; the name is stored in `.env` and the wrapper is removed only if it was created by this installer.

The `steam_deck` profile and the `hyprland` integration enable external control. When it is active, check the socket with:

```bash
~/.local/opt/sttd/sttdctl control ping
```

## Hyprland

The optional integration can install a managed block in the Hyprland configuration file. The installer proposes `$mainMod ALT, SPACE` and also supports `$mainMod SHIFT, SPACE`, `CTRL ALT, SPACE`, or a custom combination:

```bash
./install.sh --profile linux --integration hyprland --as-service \
  --hyprland-bindings yes
```

The block is delimited by `# listen:begin` and `# listen:end`, creates a backup of the configuration, and only that block is removed by `listen uninstall`. You can also edit it manually. For hold mode:

```ini
bind = $mainMod, D, exec, /home/YOUR_USER/.local/opt/sttd/sttdctl control start
bindr = $mainMod, D, exec, /home/YOUR_USER/.local/opt/sttd/sttdctl control stop
```

For toggle mode:

```ini
bind = $mainMod, D, exec, /home/YOUR_USER/.local/opt/sttd/sttdctl control toggle
```

To avoid modifying Hyprland, use `--hyprland-bindings no` or answer `no` during setup.

## Upgrade and rollback

To upgrade, download and extract a new release, enter that directory, and run the same `install.sh` command. The installer preserves `.env` and downloaded models, replaces the active installation, and stores the previous version in `~/.local/opt/sttd.previous`.

Example:

```bash
cd sttd-<new-version>-linux-amd64
./install.sh --profile linux --language es --as-service
```

To return to the previous version:

```bash
~/.local/opt/sttd/rollback.sh
```

Rollback swaps the active installation with `~/.local/opt/sttd.previous` and restarts the service when it is active.

## Models and integrity

Supported models are `tiny`, `base`, `small`, and `large`. To change the model:

```bash
~/.local/opt/sttd/change-model.sh --model small
```

`large` is Whisper Large v3 (`ggml-large-v3.bin`) and requires approximately 3.1 GB.
During setup and each interactive change, the selector shows the approximate size and a resource warning:

| Model | Approximate size | Profile |
| --- | ---: | --- |
| `tiny` | 75 MB | minimal resource use, lower accuracy |
| `base` | 142 MB | recommended balance |
| `small` | 466 MB | better accuracy, moderate resource use |
| `large` | 3.1 GB | high accuracy, more memory required |

Before downloading, the installer checks free disk space and adds a safety margin.

The model source is configured in `.env` through `STTD_MODEL_REVISION`. The default is the pinned Hugging Face revision `362722b3fdcd2300b58a8286933ead1c48619667`, with verified SHA-256 values for all supported models. If you select a different revision, configure the corresponding SHA-256 in `STTD_MODEL_SHA256_TINY`, `STTD_MODEL_SHA256_BASE`, `STTD_MODEL_SHA256_SMALL`, or `STTD_MODEL_SHA256_LARGE`.

## Uninstallation

Uninstall while keeping binaries and configuration:

```bash
~/.local/opt/sttd/uninstall.sh
```

Remove the complete installation, models, and logs:

```bash
~/.local/opt/sttd/uninstall.sh --purge
```

Purge displays a warning and requires an uppercase `Y`. It also removes the service, models, runtime, configuration, wrapper, managed Hyprland block, logs, and the `.previous` rollback directory. For automation, use `--purge --yes`.
