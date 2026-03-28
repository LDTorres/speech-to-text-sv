# speech-to-text-sv

Local-first speech-to-text daemon for `macOS`, `Linux`, and `Steam Deck`, using `whisper.cpp` as the offline transcription engine.

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
- clipboard/input tooling depending on session type:
  - X11: `xdotool`
  - Wayland: `wl-copy` and `wtype`

## Quick start

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
go run ./cmd/sttd
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
```

Current supported model catalog:

- `tiny`
- `base`
- `small`

Model changes:

- do not delete previously downloaded models
- download the new model only if it is missing
- update `STTD_TRANSCRIBE_MODEL_PATH` in `.env`

Important:

- the running process does not reload configuration automatically
- if you are using the `systemd --user` service and it is active, `change-model.sh` restarts it automatically
- if you are running `sttd` manually in the foreground, restart it yourself after changing the model

### Release

Build the release:

```bash
make build-release
```

This produces:

- `dist/release/sttd-<version>-linux-amd64/`
- `dist/release/sttd-<version>-linux-amd64.tar.gz`

Release layout:

- `sttd`
- `install.sh`
- `change-model.sh`
- `uninstall.sh`
- `profiles/`
- `.sttd/bin/`
- `scripts/speech-to-text.service.template`

Install a release:

```bash
./install.sh --profile linux
./install.sh --profile steam_deck
./install.sh --profile steam_deck --as-service
```

Recommended for Spanish-first usage:

```bash
./install.sh --profile linux --language es
./install.sh --profile steam_deck --language es
./install.sh --profile steam_deck --language es --as-service
```

You can also select the model and language during installation:

```bash
./install.sh --profile steam_deck --model small
./install.sh --profile steam_deck --model base --language es
```

Change the model after installation:

```bash
./change-model.sh
./change-model.sh --model tiny
```

If the `speech-to-text.service` user service is active, `change-model.sh` restarts it automatically. If you are running `sttd` manually in the foreground, restart it yourself.

Uninstall:

```bash
./uninstall.sh
```

`uninstall.sh`:

- removes the `speech-to-text.service` user service if present
- removes `./.sttd/models`
- does not remove `./.sttd/bin`
- does not remove `.env`

If you want to remove everything else, delete the release directory manually.

## Configuration

Available templates:

- [`.env.macos.example`](./.env.macos.example)
- [`.env.linux.example`](./.env.linux.example)
- [`.env.steam_deck.example`](./.env.steam_deck.example)

Runtime uses a single `.env` file at the repository root or release root.
The default transcription language is `es`.

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
- `STTD_TRANSCRIBE_MODEL_PATH`
- `STTD_TRANSCRIBE_LANGUAGE`
- `STTD_TRANSCRIBE_TIMEOUT`

#### Clipboard

- `STTD_CLIPBOARD_ENABLE_PASTE`

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
- `make dev-setup PROFILE=<macos|linux|steam_deck> [MODEL=<tiny|base|small>] [LANGUAGE=<code>]`
- `make change-model [MODEL=<tiny|base|small>]`
- `make build-whisper-cli`
- `make build-release`

### Scripts

- `scripts/dev-setup.sh`
  - prepares the repository for local development
- `scripts/change-model.sh`
  - changes the active model in the repository
- `scripts/build-release.sh`
  - builds the self-contained Linux release
- `scripts/build-whisper-cli-container.sh`
  - containerized Linux build for `whisper.cpp`
- `scripts/install-whisper.sh`
  - release installer
- `scripts/uninstall-whisper.sh`
  - removes models and the user service from the release

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

3. Run the app:

```bash
go run ./cmd/sttd
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

- the packaged release is currently oriented to `linux/amd64`
- Steam Deck support currently relies on `hotkey`, not `evdev`
- the supported script-level model catalog is `tiny`, `base`, `small`
- `install.sh --as-service` uses `systemd --user`, so it applies to Linux
