# AGENTS.md

## Rule Priority

The following priority must be respected:

1. Strict Rules (non-negotiable)
2. Layer Responsibilities
3. Conventions
4. Style Preferences

If there is a conflict, higher priority rules override lower ones.

---

## Purpose

This document defines the engineering standards, architectural rules, and coding conventions for this Go project.

This project is a local-first speech-to-text daemon for Steam Deck / Linux handheld usage. It is NOT an HTTP CRUD backend.

All AI agents and contributors MUST follow these rules strictly.

---

## Product Scope

The MVP is a resident local service that:
- listens for a configured input trigger
- starts audio capture while the trigger is held
- stops capture on release
- runs offline speech-to-text on the recorded segment
- stores the last transcript in memory
- attempts clipboard copy / paste injection
- allows retrying paste of the last transcript
- exposes minimal configuration and observability for local debugging

Out of scope for MVP unless explicitly added:
- cloud transcription
- user accounts
- multi-tenant concepts
- database-backed persistence
- generic plugin frameworks
- unnecessary HTTP APIs

---

## High-Level Architecture

This project follows a modular architecture organized by capability:

```text
cmd/
  sttd/
    main.go
internal/
  app/
  config/
  log/
  platform/
  modules/
    trigger/
    audio/
    transcribe/
    clipboard/
    session/
    notify/
  bootstrap/
```

Each module is self-contained and represents a runtime capability.

Suggested module responsibilities:
- `trigger`: input detection, hold/release, double-tap recognition
- `audio`: recording lifecycle and audio file/buffer generation
- `transcribe`: whisper.cpp process orchestration and transcript parsing
- `clipboard`: copy, paste retry, virtual input integration
- `session`: orchestration of one speech-to-text interaction and in-memory last transcript state
- `notify`: optional user-facing local notifications/log-friendly status projection

---

## Tech Stack

- Language: Go
- Logging: zap (structured logging)
- Config: environment variables + local config file when justified
- Transcription engine: whisper.cpp invoked as an external binary for MVP
- Observability: OpenTelemetry optional/minimal, extensible
- Testing: testify
- Process model: long-lived local daemon
- Service manager target: systemd user service

Avoid introducing a database, ORM, or HTTP router unless a real product requirement appears.

---

## Layer Responsibilities

### Bootstrap Layer
- Wires dependencies
- Loads configuration
- Initializes logger
- Builds application components
- Starts and stops long-lived processes cleanly
- Lives in `cmd/...` and `internal/bootstrap`

### App / Orchestration Layer
- Coordinates runtime flows
- Owns lifecycle of managed components
- Connects trigger -> audio -> transcribe -> clipboard -> notify
- Contains use-case orchestration
- Must not contain low-level platform implementation details

### Module / Domain Layer
- Implements capability-specific behavior
- Keeps logic isolated by runtime concern
- Defines explicit inputs/outputs
- Must not depend on unrelated modules unless justified by orchestration needs

### Platform Layer
- Encapsulates OS-specific integrations
- Handles evdev/uinput/clipboard/process execution/system notifications when needed
- Keeps Linux-specific details out of orchestration logic

### Config / Log Layer
- Centralized configuration parsing and validation
- Centralized logger construction
- No business logic

---

## Strict Rules

1. NO global mutable variables
2. ALWAYS pass `context.Context`
3. DO NOT introduce HTTP just because the template had it
4. DO NOT introduce a database for MVP runtime state
5. KEEP the last transcript in memory unless persistence becomes a proven requirement
6. ISOLATE platform-specific code behind explicit packages/interfaces only when needed
7. USE structured logging (zap), no `fmt.Println`
8. ALWAYS handle errors explicitly
9. LONG-LIVED goroutines must have explicit ownership and shutdown paths
10. DO NOT spawn fire-and-forget goroutines
11. KEEP modules isolated by capability
12. PREFER local-first, offline-first behavior for MVP
13. DO NOT over-generalize into a plugin framework
14. MAIN package is only for wiring and lifecycle start/stop
15. ALL external process execution must be bounded, logged, and cancellable

---

## Naming Conventions

- Package names must be short, lowercase, and singular when possible
- Module names must represent a concrete runtime capability
- Exported names must use clear domain language
- Avoid vague names like `data`, `util`, `common`, `helper`
- Types should reflect runtime intent:
  - `Manager`
  - `Recorder`
  - `Transcriber`
  - `Clipboard`
  - `SessionService`
  - `TriggerWatcher`
- Methods should use explicit, behavior-oriented names:
  - `Start`
  - `Stop`
  - `RecordUntilRelease`
  - `TranscribeFile`
  - `Copy`
  - `Paste`
  - `RetryLastPaste`
  - `HandleTriggerPressed`

---

## File Conventions

- Keep `main.go` only for wiring and bootstrap
- Keep runtime wiring in `internal/bootstrap` or `internal/app`
- Keep modules focused and small
- Split files by responsibility only when needed, for example:
  - `service.go`
  - `manager.go`
  - `runner.go`
  - `watcher.go`
  - `types.go`
- Do not create generic shared files prematurely
- Prefer explicit packages over deep nesting

---

## Runtime Flow Conventions

The default MVP flow is:

1. Trigger pressed
2. Audio recording starts
3. Trigger released
4. Recording finalizes
5. Transcription executes
6. Transcript is stored as last transcript
7. Clipboard copy is attempted
8. Paste injection is attempted
9. If paste fails, transcript remains available for retry
10. Double-tap trigger retries paste of last transcript

Keep this flow explicit in code. Do not hide orchestration inside platform adapters.

---

## State Conventions

- Runtime state must be explicit and minimal
- Keep only the state needed for MVP, for example:
  - whether recording is active
  - last transcript
  - last transcription timestamp
  - retry eligibility
- Protect shared mutable state with clear ownership
- Prefer encapsulating mutable state behind a dedicated type
- Do not add persistent storage until a concrete user need exists

---

## Error Conventions

- Define sentinel errors at the module or service layer when needed
- Wrap errors using `fmt.Errorf("...: %w", err)` when adding context
- Never compare error strings; always use `errors.Is` / `errors.As`
- Do not leak raw platform or process errors above orchestration boundaries without context
- Error messages must be lowercase and without trailing punctuation
- Differentiate clearly between:
  - trigger errors
  - audio capture errors
  - transcription errors
  - clipboard/paste errors
  - configuration errors

---

## Dependency Injection Conventions

- Use explicit constructor injection
- Wire dependencies in bootstrap only
- Avoid service locators and hidden dependency resolution
- Each component should depend only on what it actually uses
- Introduce interfaces only when they provide clear value for testing, platform isolation, or process abstraction

---

## Configuration Conventions

- Configuration must be centralized in `internal/config`
- Fail fast if required configuration is missing or invalid
- Prefer environment variables for service/runtime configuration
- A local config file is acceptable for end-user settings such as:
  - trigger binding
  - model path
  - language
  - paste retry behavior
  - notification enablement
- Do not hardcode device paths, binary paths, or secrets

---

## External Process Conventions

- `whisper.cpp` execution must be encapsulated in the transcription module
- Always execute external binaries with `context.Context`
- Capture stdout/stderr explicitly
- Validate binary path and model path at startup when possible
- Bound process execution with timeout where justified
- Surface actionable errors when transcription fails
- Do not scatter subprocess execution across the codebase

---

## Platform Conventions

- Keep Linux/Steam Deck specifics under `internal/platform`
- Separate input listening from input injection
- OS integration code must be replaceable without rewriting orchestration logic
- Avoid mixing evdev/uinput/clipboard shell logic directly into app orchestration
- Prefer narrow adapters over giant platform manager types

---

## Concurrency Conventions

- Prefer simple synchronous code unless concurrency provides a clear benefit
- Concurrency must be explicit, bounded, and cancellable
- Always propagate `context.Context` into concurrent work when possible
- Never start goroutines without a clear ownership and shutdown path
- Do not use concurrency as a substitute for good process design

### Goroutine Rules

- A goroutine must have one of these purposes:
  - watching input events
  - coordinating lifecycle of long-lived components
  - running bounded background work with explicit ownership
- Every goroutine must have a termination condition
- Long-lived goroutines must be started only from bootstrap or managed components
- Do not start fire-and-forget goroutines inside runtime orchestration

### Primitive Selection

- Use channels for:
  - signaling
  - ownership transfer of events
  - coordination between watchers and orchestrators
- Use `sync.Mutex` or `sync.RWMutex` for protecting shared in-memory state
- Prefer `sync.Mutex` by default
- Use `sync.WaitGroup` only to wait for a fixed set of goroutines to finish
- Prefer `errgroup.Group` when concurrent tasks can fail, need cancellation, or are lifecycle-scoped
- Use `atomic` only for simple counters or flags
- Do not mix channels, mutexes, and atomics in the same flow unless clearly justified

### Channel Conventions

- Channel ownership must be clear
- The sender is responsible for closing a channel when closure is required
- Do not close a channel from the receiver side
- Prefer unbuffered channels by default
- Use buffered channels only when you can justify capacity and backpressure behavior
- Every blocking channel operation in long-lived code should consider `ctx.Done()` when cancellation matters

### Mutex Conventions

- Keep critical sections small
- Do not hold locks while performing I/O, OS calls, or subprocess execution
- Avoid nested locks unless lock ordering is explicit and documented
- Prefer encapsulating mutable shared state behind a type with well-defined methods

### Select Conventions

- Use `select` to coordinate:
  - cancellation
  - multiple input/event sources
  - send/receive readiness when necessary
- Avoid empty `default` branches that create busy loops
- Use `time.Ticker` carefully and always stop it
- Prefer `context`-driven cancellation over ad hoc done channels unless ownership is clearer that way

### Performance and Safety

- Measure before adding concurrency for performance reasons
- Avoid unbounded goroutine creation from loops or event streams
- Be explicit about ordering guarantees when processing events concurrently
- Protect all shared mutable state
- Run tests with the race detector when concurrency is involved

### Testing Concurrency

- Keep concurrency tests deterministic where possible
- Prefer testing behavior and cancellation semantics over timing assumptions
- Avoid fragile sleeps in tests; use synchronization or eventually-style assertions only when justified
- Add race-detector coverage for packages that use shared mutable state or goroutines

---

## Testing Conventions

- Prefer table-driven tests where useful
- Test orchestration logic first
- Add module tests where runtime behavior matters
- Keep tests deterministic
- Mock only real external dependencies such as:
  - subprocess execution
  - clipboard integration
  - input devices
  - notifications
- Avoid unnecessary mocks for internal logic
- Test names should describe behavior clearly

Examples:
- `TestSessionService_TranscribeAndPaste_Success`
- `TestSessionService_PasteFails_KeepsLastTranscript`
- `TestSessionService_DoubleTap_RetriesLastTranscript`
- `TestTranscriber_TranscribeFile_ProcessFailure`
- `TestTriggerWatcher_PressRelease_EmitsSessionEvent`

---

## Code Style Conventions

- Follow idiomatic Go formatting
- Keep functions small and focused
- Avoid hidden behavior and implicit side effects
- Avoid premature generalization
- Avoid unnecessary interfaces
- Keep error messages lowercase and without trailing punctuation
- Prefer explicit types and explicit control flow for runtime-critical paths

---

## Observability Conventions

- Logs should include enough context for debugging runtime flow
- Prefer structured fields over interpolated log strings
- Include identifiers that help correlate one interaction flow, for example:
  - session id
  - trigger event type
  - transcription duration
  - paste outcome
- Do not log secrets or sensitive user content unless explicitly justified for local debug mode
- Do not log full transcripts by default

---

## Context Conventions

- Always propagate context from bootstrap down into managed components
- Do not create background contexts inside runtime flow
- Use `context.WithTimeout` at boundaries such as subprocess calls when justified
- Never ignore context cancellation

---

## Security and Privacy Conventions

- Default to offline/local-first processing
- Do not send audio or transcripts to remote services unless explicitly implemented and documented
- Never log secrets, tokens, or credentials
- Treat audio and transcript data as user-sensitive
- Keep error output safe and actionable
- Minimize retention of transcript data in memory to what is needed for MVP retry behavior

---