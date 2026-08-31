# speech-to-text-sv

Local-first speech-to-text daemon for `macOS`, `Linux`, and `Steam Deck`, using `whisper.cpp` as the offline transcription engine. Published packages currently target `linux/amd64`; macOS is supported through the local development flow.

The main runtime flow is:
1. detect a trigger
2. start recording
3. stop recording
4. transcribe locally
5. keep the last transcript in memory
6. attempt copy/paste of the result
7. allow paste retry

This project does not use remote transcription services. It does not expose an HTTP API. The useful MVP state is kept in memory.

## How it works

`sttd` is a long-lived local daemon. At runtime it coordinates these modules:

- `trigger`: global hotkey handling and `press` / `release` events
- `audio`: audio capture
- `transcribe`: `whisper.cpp` CLI execution
- `clipboard`: copy/paste or direct typing, depending on platform
- `session`: orchestration of one speech-to-text interaction and in-memory last transcript state

Supported profiles:

- `macos`
- `linux`
- `steam_deck`

Relevant defaults by profile:

- `macos`
  - hotkey: `cmd+shift+space`
  - trigger mode: `hold`
  - audio: macOS capture
- `linux`
  - hotkey: `ctrl+shift+space`
  - trigger mode: `hold`
  - audio: `pw-record`
- `steam_deck`
  - hotkey: `f12`
  - trigger mode: `toggle`
  - audio: `pw-record`

## Requirements

### General

- Go installed for local development
- `curl`
- network access to download the selected model and, during initial setup, the `whisper.cpp` source or runtime

### macOS

- `cmake`
- local build toolchain for compiling `whisper.cpp`

### Linux / Steam Deck

- `docker` for local `dev-setup` of the `whisper.cpp` runtime
- `pw-record` for audio capture
- `mpv` for automatic wake-up of composite USB webcam microphones (optional;
  only used when the resolver finds a related video node)
- clipboard/input tooling depending on session type:
  - X11: `xclip` and `xdotool`
  - Wayland: `wl-copy` and `wtype`
- X11 development and the packaged global hotkey require X11 development/runtime libraries

For a CUDA-enabled Linux runtime, the build uses the official NVIDIA CUDA
container image and does not require the CUDA toolkit to be installed on the
host. The target machine still needs a working NVIDIA driver compatible with
the bundled CUDA runtime. CPU remains the default build mode.

## Quick start

### End-user installation

Install the latest published Linux release with the guided setup:

```bash
curl -fsSL https://raw.githubusercontent.com/LDTorres/speech-to-text-sv/main/scripts/bootstrap-install.sh | bash -s -- --interactive
```

The bootstrap downloads the versioned archive, verifies its SHA-256 checksum,
and runs the packaged installer. Pin a release with `--version v0.1.5`; use
`--non-interactive` for automation. Review the bootstrap script before piping
it to a shell if your environment requires a stricter supply-chain policy.

The installer creates the memorable `listen` command in `~/.local/bin`:

```bash
listen status
listen record start
listen record stop
listen model
listen doctor
listen uninstall
```

The technical catalog is also available with
`~/.local/opt/sttd/sttdctl model list --json`.

Use `--command-name <name>` during installation if `listen` is already in use.

### Local development

Prepare the repository from scratch:

```bash
make dev-setup PROFILE=macos
make dev-setup PROFILE=linux
make dev-setup PROFILE=steam_deck
```

You can also select the model and language during setup:

```bash
make dev-setup PROFILE=macos MODEL=small
make dev-setup PROFILE=steam_deck MODEL=base LANGUAGE=es
```

On an NVIDIA Linux machine, request the CUDA runtime explicitly:

```bash
WHISPER_ACCELERATION=cuda make dev-setup PROFILE=linux MODEL=small LANGUAGE=es
```

This does the following:

- creates `.env` if it does not exist, using the selected profile template
- prepares `./.sttd/bin`
- downloads or builds `whisper.cpp`
- downloads the selected model into `./.sttd/models`
- updates:
  - `STTD_PLATFORM_PROFILE`
  - `STTD_TRANSCRIBE_BINARY_PATH`
  - `STTD_TRANSCRIBE_MODEL_PATH`
  - `STTD_TRANSCRIBE_LANGUAGE`

Then run:

```bash
make run
```

### Change the model in development

Interactive selector:

```bash
make change-model
```

Non-interactive:

```bash
make change-model MODEL=tiny
make change-model MODEL=base
make change-model MODEL=small
make change-model MODEL=large
```

Current supported model catalog:

| Model | Approximate weight | Resource profile |
| --- | ---: | --- |
| `tiny` | 75 MB | lowest resource use |
| `base` | 142 MB | balanced default |
| `small` | 466 MB | moderate resource use |
| `large` | 3.1 GB | highest accuracy and memory use |

`large` downloads Whisper Large v3 (`ggml-large-v3.bin`), approximately 3.1 GB.

Model changes:

- do not delete previously downloaded models
- download the new model only if it is missing
- update `STTD_TRANSCRIBE_MODEL_PATH` in `.env`

Important:

- the running process does not reload configuration automatically
- if you are using the `systemd --user` service and it is active, `change-model.sh` restarts it automatically
- if you are running `sttd` manually in the foreground, restart it yourself after changing the model

### Release

Build an explicit release version:

```bash
make build-release RELEASE_BUMP=patch
```

Official releases currently target `linux/amd64` and are built with the
`x11hotkey` build tag. On Wayland/Hyprland, use external control and desktop
bindings instead of relying on an X11 global hotkey.

The release defaults to automatic acceleration selection. It packages a CPU
runtime and, unless explicitly building CPU-only, a CUDA runtime. At runtime
the wrapper selects CUDA when a usable NVIDIA driver/GPU is detected and
otherwise uses CPU. You can force the behavior with
`STTD_TRANSCRIBE_ACCELERATION=auto|cpu|cuda`.

To build the Linux release with automatic selection and CUDA support:

```bash
WHISPER_ACCELERATION=cuda make build-release
```

`WHISPER_ACCELERATION` accepts `auto`, `cpu` or `cuda`. `cpu` builds a smaller
CPU-only release. `auto` and `cuda` package both runtimes so the installed
release can fall back to CPU on machines without NVIDIA support. The CUDA
runtime is built in the container and the wrapper adds the selected runtime's
libraries to `LD_LIBRARY_PATH`. Set `WHISPER_CPP_COMMIT` to verify the checked
out whisper.cpp commit during container builds.

For the RTX 2070 Super, use CUDA architecture `75` and limit build parallelism
if the machine starts using swap:

```bash
WHISPER_ACCELERATION=cuda \
WHISPER_CUDA_ARCHITECTURES=75 \
WHISPER_BUILD_JOBS=6 \
make build-release
```

`WHISPER_CUDA_ARCHITECTURES` is optional for portable CUDA builds. The value
`75` is specific to the RTX 2070 Super and reduces unnecessary CUDA targets.
`WHISPER_BUILD_JOBS` defaults to `2` for predictable memory usage.

This produces:

- `dist/release/sttd-<version>-linux-amd64/`
- `dist/release/sttd-<version>-linux-amd64.tar.gz`
- `dist/release/sttd-<version>-linux-amd64.tar.gz.sha256`

Publish the release to GitHub Releases with an explicit version or bump:

```bash
make publish-release RELEASE_BUMP=patch
```

End users can download published archives from the [GitHub Releases page](https://github.com/LDTorres/speech-to-text-sv/releases).

Useful variants:

```bash
make publish-release RELEASE_VERSION=v0.1.0
./scripts/publish-release.sh --tag v0.1.0 --title "v0.1.0"
./scripts/publish-release.sh --tag v0.1.0 --notes-file ./release-notes.md
./scripts/publish-release.sh --tag v0.1.0 --draft
```

`publish-release.sh`:

- uses `gh release`
- uploads `dist/release/sttd-<version>-linux-amd64.tar.gz`
- uploads the matching SHA-256 checksum
- creates the release if it does not exist
- updates the release and re-uploads the asset with `--clobber` if it already exists
- runs `build-release` automatically if the archive is missing, unless `--skip-build` is used

Version bumps are calculated from the greatest numeric `vMAJOR.MINOR.PATCH`
tag in the repository. Existing suffixes such as `-nox11` and `-rc6` are used
as the same `0.1.4` version base, so the next patch release is `v0.1.5`.
Publications require an explicit version/bump and a clean worktree.

Examples:

```bash
./scripts/build-release.sh --patch
./scripts/build-release.sh --minor
./scripts/build-release.sh --major
make build-release RELEASE_BUMP=patch
./scripts/publish-release.sh --patch
```

An explicit version is also supported:

```bash
./scripts/build-release.sh --version v0.1.5
./scripts/publish-release.sh --tag v0.1.5
```

Release layout:

- `sttd`
- `sttdctl`
- `install.sh`
- `change-model.sh`
- `uninstall.sh`
- `rollback.sh`
- `doctor.sh`
- `scripts/listen.sh`
- `scripts/lib/hyprland.sh`
- `INSTALL.md`
- `VERSION`
- `profiles/`
- `.sttd/bin/`
- `scripts/speech-to-text.service.template`

For end-user installation, follow the complete instructions packaged in
[`INSTALL.md`](./INSTALL.md). In short:

```bash
sha256sum -c sttd-<version>-linux-amd64.tar.gz.sha256
tar -xzf sttd-<version>-linux-amd64.tar.gz
cd sttd-<version>-linux-amd64
./install.sh --check --profile linux
./install.sh --profile linux --language es --as-service
```

Recommended for Spanish-first usage:

```bash
./install.sh --profile steam_deck --language es --as-service
```

The installer copies the active release to `~/.local/opt/sttd`. Re-running
`install.sh` from a newer extracted release updates that stable location while
preserving the existing configuration and downloaded models.

To roll back the most recent update:

```bash
~/.local/opt/sttd/rollback.sh
```

The rollback swaps the active installation with `~/.local/opt/sttd.previous`
and restarts the user service when it is active.

Use `~/.local/opt/sttd/sttdctl doctor` for a complete installation status,
including version, profile, model, service and external control socket.

On Hyprland, enable the optional Wayland integration. It uses a user-only Unix
socket and lets Hyprland bindings control the daemon without an X11 hotkey:

```bash
./install.sh --profile linux --integration hyprland --language es --as-service
```

For hold mode, add bindings similar to these to the user's Hyprland bindings
file:

```ini
bind = $mainMod, D, exec, /home/TU_USUARIO/.local/opt/sttd/sttdctl control start
bindr = $mainMod, D, exec, /home/TU_USUARIO/.local/opt/sttd/sttdctl control stop
```

For toggle mode, configure `STTD_TRIGGER_MODE=toggle` and use one binding:

```ini
bind = $mainMod, D, exec, /home/TU_USUARIO/.local/opt/sttd/sttdctl control toggle
```

The installer can optionally add a managed hold/release block to Hyprland:

```bash
./install.sh --profile linux --integration hyprland --as-service \
  --hyprland-bindings yes
```

It proposes `$mainMod ALT, SPACE`, detects the selected binding, and offers
two alternatives or a custom combination. The managed block is marked with
`# listen:begin` / `# listen:end`; uninstall removes only that block.

You can also select the model and language during installation:

```bash
./install.sh --profile steam_deck --model small
./install.sh --profile steam_deck --model base --language es
```

Change the model after installation:

```bash
~/.local/opt/sttd/change-model.sh
~/.local/opt/sttd/change-model.sh --model tiny
```

If the `speech-to-text.service` user service is active, `change-model.sh` restarts it automatically. If you are running `sttd` manually in the foreground, restart it yourself.

Uninstall:

```bash
~/.local/opt/sttd/uninstall.sh
```

`uninstall.sh`:

- removes the `speech-to-text.service` user service if present
- removes `~/.local/opt/sttd/.sttd/models`
- keeps binaries and `.env` by default
- accepts `--purge` to remove the complete installation and logs
- removes the `.previous` rollback directory when `--purge` is used

Use `listen uninstall` to remove everything. The command shows a warning and
requires uppercase `Y`; scripts may use
`~/.local/opt/sttd/uninstall.sh --purge --yes`.

### Logs

When installed as `speech-to-text.service`, the daemon sends stdout and stderr
to the systemd user journal. Journald manages retention and rotation according
to the host's systemd policy.

You can inspect logs through `sttdctl`:

```bash
~/.local/opt/sttd/sttdctl logs tail --json --lines 200
journalctl --user-unit speech-to-text.service --follow
```

The legacy file path command remains available for older installations that
still use file-based service output:

```bash
~/.local/opt/sttd/sttdctl logs path --json
```

## Configuration

Available templates:

- [`.env.macos.example`](./.env.macos.example)
- [`.env.linux.example`](./.env.linux.example)
- [`.env.steam_deck.example`](./.env.steam_deck.example)

Runtime uses a single `.env` file at the repository root or release root.
The default transcription language is `es`.

The installer also stores `STTD_PUBLIC_COMMAND_NAME`,
`STTD_HYPRLAND_CONFIG_PATH` and `STTD_HYPRLAND_BINDING` so upgrades can keep
the selected wrapper and desktop integration settings.

### Public variables

#### Platform

- `STTD_PLATFORM_PROFILE`
  - values: `macos`, `linux`, `steam_deck`

#### Trigger

- `STTD_TRIGGER_MODE`
  - values: `hold`, `toggle`
  - `steam_deck` defaults to `toggle`
- `STTD_TRIGGER_HOTKEY_MODIFIERS`
  - example: `cmd+shift`, `ctrl+shift`
- `STTD_TRIGGER_HOTKEY_KEY`
  - example: `space`, `f12`
- `STTD_TRIGGER_DOUBLE_TAP_WINDOW`
  - example: `400ms`

#### Audio

- `STTD_AUDIO_TEMP_DIR`
- `STTD_AUDIO_FILE_NAME`
- `STTD_AUDIO_INPUT_DEVICE`
  - optional
  - mainly useful on `linux` and `steam_deck` to pin a specific PipeWire source

#### Transcription

- `STTD_TRANSCRIBE_BINARY_PATH`
- `STTD_TRANSCRIBE_ACCELERATION`
  - `auto` (default), `cpu` or `cuda`; used by the release wrapper
- `STTD_TRANSCRIBE_MODEL_PATH`
- `STTD_TRANSCRIBE_LANGUAGE`
- `STTD_TRANSCRIBE_TIMEOUT`
- `STTD_MODEL_REVISION`
  - defaults to the pinned Hugging Face revision `362722b3fdcd2300b58a8286933ead1c48619667`
- `STTD_MODEL_SHA256_TINY`, `STTD_MODEL_SHA256_BASE`, `STTD_MODEL_SHA256_SMALL`, `STTD_MODEL_SHA256_LARGE`
  - default SHA-256 checksums validated while downloading models; override them only when intentionally selecting a different revision
- `WHISPER_SOURCE_SHA256`
  - optional SHA-256 checksum for the macOS whisper.cpp source archive
- `WHISPER_CPP_COMMIT`
  - optional commit verification for containerized whisper.cpp builds

#### Clipboard

- `STTD_CLIPBOARD_ENABLE_PASTE`
- `STTD_CLIPBOARD_TIMEOUT`
  - maximum time allowed for `wl-copy`, `wtype` or the platform paste command
  - defaults to `5s`; prevents clipboard integration failures from keeping a session in `processing`

#### Linux audio device wake

Linux audio capture uses PipeWire. The default `auto` mode detects whether the
selected audio source belongs to a composite USB device that also exposes a
video node. If the audio stream produces no frames, it temporarily opens the
related video node with `mpv` so devices such as the OBSBOT Tiny 2 Lite wake
their microphone. Ordinary microphones and Bluetooth headsets do not have a
related video node and are left untouched.

- `STTD_AUDIO_CAMERA_WAKE`
  - `auto` (default on Linux), or `none`
- `STTD_AUDIO_CAMERA_VIDEO_DEVICE`
  - optional explicit V4L2 device override; normally leave empty

The resolver prefers stable `/dev/v4l/by-id` paths and correlates audio/video
through their shared USB parent. Set `STTD_AUDIO_CAMERA_WAKE=none` to disable
the behavior, for example when another application owns the camera.

### Secondary variables

These still exist in config, but currently provide little or no practical value for normal usage:

- `STTD_APP_ENV`
- `STTD_APP_SHUTDOWN_TIMEOUT`
- `STTD_AUDIO_SAMPLE_FORMAT`
- `STTD_NOTIFY_ENABLED`

## Useful commands and scripts

### Makefile

- `make run`
- `make test`
- `make test-x11-docker` (runs the X11-tagged Linux test suite inside Docker)
- `make dev-setup PROFILE=<macos|linux|steam_deck> [MODEL=<tiny|base|small|large>] [LANGUAGE=<code>]`
- `make change-model [MODEL=<tiny|base|small|large>]`
- `make build-whisper-cli`
- `make build-release`
- `make verify-release RELEASE_ARCHIVE=<path>`

### Scripts

- `scripts/dev-setup.sh`
  - prepares the repository for local development
- `scripts/change-model.sh`
  - changes the active model in the repository
- `scripts/build-release.sh`
  - builds the self-contained Linux release
- `scripts/build-whisper-cli-container.sh`
  - containerized Linux build for `whisper.cpp`
- `scripts/test-x11-docker.sh`
  - runs the Linux X11-tagged test suite inside the build container
- `scripts/install-whisper.sh`
  - release installer
- `scripts/uninstall-whisper.sh`
  - removes the user service and, optionally, the installed runtime state
- `scripts/rollback-whisper.sh`
  - swaps the active installation with the previous release safely
- `scripts/doctor.sh`
  - verifies a packaged release, desktop dependencies and current installation status
- `scripts/bootstrap-install.sh`
  - downloads and verifies a published Linux release before running its installer
- `scripts/verify-release.sh`
  - validates a built archive, checksum, required files and X11 hotkey build

## Contributing

### Suggested workflow

1. Prepare the environment:

```bash
make dev-setup PROFILE=macos
```

or:

```bash
make dev-setup PROFILE=linux
```

2. Run tests:

```bash
make test
```

For the X11-tagged Linux tests, use the host toolchain when X11 development
headers are installed, or use the reproducible Docker-based test environment:

```bash
make test-x11-docker
```

3. Run the app:

```bash
make run
```

4. If you changed packaging or release scripts, also validate:

```bash
make build-release
```

### Project conventions

- modular architecture by capability
- `cmd/sttd` is only for wiring and startup
- `internal/platform` contains OS-specific integrations
- `internal/modules` contains runtime capabilities
- `whisper.cpp` is used as an external process
- structured logs use `zap`

Additional engineering rules live in [AGENTS.md](./AGENTS.md).

## Current limitations

- published packages currently target `linux/amd64`; macOS remains development-only
- Steam Deck support uses external control when the X11 hotkey is unavailable; it does not use `evdev`
- the supported script-level model catalog is `tiny`, `base`, `small`, `large`
- `install.sh --as-service` uses `systemd --user`, so it applies to Linux
