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

This document defines the engineering standards, architectural rules, and coding conventions for this Go backend project.

All AI agents and contributors MUST follow these rules strictly.

---

## High-Level Architecture

This project follows a modular architecture organized by feature:

internal/
  modules/
    <module>/
      dto.go
      model.go
      repository.go
      service.go
      handler.go

Each module is self-contained and represents a business capability (e.g., users, auth, orders).

---

## Tech Stack

- HTTP Router: chi
- ORM: GORM (PostgreSQL)
- Migrations: golang-migrate (REQUIRED, no AutoMigrate in runtime)
- Logging: zap (structured logging)
- Config: envconfig (environment variables only)
- Observability: OpenTelemetry (minimal setup, extensible)
- Testing: testify

---

## Layer Responsibilities

### Handler (HTTP Layer)
- Handles HTTP requests and responses
- Parses DTOs
- Calls service layer
- NEVER contains business logic
- NEVER accesses database directly

### Service (Business Logic)
- Contains business rules
- Validates inputs (beyond basic parsing)
- Coordinates repositories
- Does NOT depend on HTTP

### Repository (Data Access)
- Uses GORM
- Contains ALL database logic
- No business logic
- Accepts context.Context

### DTOs
- Used ONLY for HTTP request/response
- Must NOT reuse GORM models
- Must be explicitly mapped

### Models
- Represent database schema (GORM structs)
- Used ONLY in repository/service layers

---

## Strict Rules

1. NO global variables
2. ALWAYS pass context.Context
3. DO NOT use GORM AutoMigrate in application runtime
4. ALL schema changes MUST go through migrations
5. DO NOT mix DTOs with database models
6. DO NOT put business logic in handlers
7. DO NOT access database outside repositories
8. USE structured logging (zap), no fmt.Println
9. ALWAYS handle errors explicitly
10. KEEP modules isolated (no cross-module imports unless necessary)

---

## Routing Conventions

- All routes must be versioned: `/api/v1/...`
- Each module registers its own routes
- Use chi.Router with Route() grouping

Example:

```go
r.Route("/api/v1/users", func(r chi.Router) {
    r.Get("/", handler.ListUsers)
    r.Post("/", handler.CreateUser)
})
```

## Naming Conventions

- Package names must be short, lowercase, and singular when possible
- Module names must represent a business capability (`users`, `auth`, `orders`)
- Exported names must use clear domain language
- Avoid vague names like `data`, `util`, `common`, `helper`
- DTO names must be explicit:
  - `<Action><Entity>Request`
  - `<Entity>Response`
  - `<Entity>ListResponse` when needed
- Service methods should use business-oriented names:
  - `CreateUser`
  - `GetUserByID`
  - `ListUsers`
- Repository methods should be persistence-oriented and explicit:
  - `Create`
  - `GetByID`
  - `List`
  - `Delete`
- Handler methods should reflect the HTTP use case:
  - `CreateUser`
  - `GetUser`
  - `ListUsers`

---

## File Conventions

- Keep module files limited to:
  - `dto.go`
  - `model.go`
  - `repository.go`
  - `service.go`
  - `handler.go`
- If a module grows, split files by responsibility only when necessary:
  - `repository_create.go`
  - `repository_query.go`
  - `service_create.go`
- Do not create generic shared files prematurely
- Keep `main.go` only for wiring and bootstrap
- Keep router setup in `internal/server`

---

## Handler Conventions

- Handlers must:
  - parse input
  - validate basic request shape
  - call the service
  - map service output into HTTP response
- Handlers must not:
  - access the database
  - contain business rules
  - perform persistence mapping logic unrelated to HTTP
- Always return JSON responses
- Always set `Content-Type: application/json`
- Decode request bodies using `json.Decoder`
- Do not ignore decode/encode errors unless explicitly justified

---

## Service Conventions

- Services contain business logic only
- Services must not depend on HTTP concepts
- Services must not return HTTP-specific responses
- Services should receive explicit inputs and return explicit domain results
- Services coordinate repositories and enforce business invariants
- Prefer constructor injection for dependencies

---

## Repository Conventions

- Repositories are the only layer allowed to interact with GORM
- Repositories must accept `context.Context`
- Repositories must return domain-relevant errors, never HTTP errors
- Keep queries explicit and readable
- Avoid hidden side effects in repository methods
- Transactions must be handled explicitly when needed

---

## DTO Conventions

- DTOs exist only for transport boundaries
- DTOs must not contain GORM annotations
- DTOs must not be reused as database models
- Request DTOs should validate the transport contract
- Response DTOs should expose only what the API intends to return
- Mapping between DTOs and models must be explicit

---

## Model Conventions

- Models represent persistence structure
- Models may contain GORM tags
- Models must not be exposed directly as HTTP responses
- Keep models focused on stored data, not presentation format
- Avoid embedding transport concerns into models

---

## Response Conventions

- Success responses must be JSON
- Error responses must be JSON
- Use appropriate HTTP status codes
- For single-resource creation, return `201 Created`
- For successful reads, return `200 OK`
- For invalid input, return `400 Bad Request`
- For missing records, return `404 Not Found`
- For unexpected failures, return `500 Internal Server Error`

Example error format:

```json
{
  "error": "user not found"
}
```

---

## Response Envelope Conventions

- Prefer a consistent envelope for all responses
- Success responses should use:

```json
{
  "data": {},
  "error": null
}
```

- Error responses should use:

```json
{
  "data": null,
  "error": {
    "message": "description",
    "code": "optional_machine_code"
  }
}
```

- Do not mix envelope and non-envelope formats within the same service
- Keep envelope structure stable across versions

---

## Error Conventions

- Define sentinel errors in service layer when needed
- Use errors.Is / errors.As for comparisons
- Do not compare error strings
- Repository errors must be mapped to domain-level errors in service layer
- Handlers must translate domain errors into HTTP responses

---

## Error Typing Conventions

- Define sentinel errors at the service layer for domain conditions (e.g., ErrNotFound, ErrInvalidInput)
- Wrap errors using `fmt.Errorf("...: %w", err)` when adding context
- Never compare error strings; always use `errors.Is` / `errors.As`
- Do not leak infrastructure errors (DB/driver) past the service layer
- Handlers must map domain errors to HTTP status codes consistently

---

## Validation Conventions
- Basic transport validation happens in handlers
- Business validation happens in services
- Validation errors must return clear client-facing messages
- Do not leak internal implementation details in validation errors

---

## Pagination Conventions

- List endpoints should support pagination when applicable
- Use query params: `limit` and `offset`
- Validate and sanitize pagination inputs in handlers
- Do not expose database pagination implementation details

---

## Dependency Injection Conventions
- Use explicit constructor injection
- Wire dependencies in main.go only
- Avoid service locators and hidden dependency resolution
- Each layer should depend only on the layer directly below it

---

## Server Conventions

- HTTP server must define timeouts:
  - ReadHeaderTimeout
  - ReadTimeout
  - WriteTimeout
  - IdleTimeout
- Always implement graceful shutdown
- Server wiring must be done in main.go only

---

## Middleware Conventions
- Middleware must be placed in internal/server
- Middleware should be composable and focused on one responsibility
- Preferred middleware responsibilities:
  - logging
  - recovery
  - request ID
  - tracing
  - timeout
- Middleware must not contain business logic

---

## Configuration Conventions
- Configuration must come from environment variables
- Do not hardcode credentials or secrets
- Keep configuration centralized in internal/config
- Fail fast if required configuration is missing
- Use .env.example only as documentation, never as a secret store

---

## Migration Conventions
- Every schema change must have an explicit migration
- Migration filenames must be ordered and descriptive
- Do not modify old migrations after they are applied
- Do not rely on runtime schema mutation
- Keep migrations deterministic and reversible when possible

---

## Testing Conventions
- Prefer table-driven tests where useful
- Test service logic first
- Add repository tests when persistence behavior matters
- Keep tests deterministic
- Avoid unnecessary mocks
- Mock only true external dependencies
- Test names should describe behavior clearly

Examples:
- TestCreateUser_Success
- TestCreateUser_InvalidEmail
- TestGetUserByID_NotFound

---

## Code Style Conventions
- Follow idiomatic Go formatting
- Keep functions small and focused
- Avoid hidden behavior and implicit side effects
- Avoid premature generalization
- Avoid unnecessary interfaces
- Introduce interfaces only when they provide clear value for testing or decoupling
- Keep error messages lowercase and without trailing punctuation

---

## Cross-Module Conventions
- Modules should be as independent as possible
- Avoid direct imports between modules unless justified by business rules
- Shared behavior should be extracted carefully, only after duplication becomes meaningful
- Do not create a shared package too early

---

## Observability Conventions
- Traces should wrap meaningful operations
- Logs should include enough context for debugging
- Do not log secrets, tokens, passwords, or raw credentials
- Prefer structured fields over interpolated log strings

---

## Context Conventions

- Always propagate context from HTTP layer down to repository
- Do not create background contexts inside request flow
- Use context.WithTimeout only at boundaries (e.g., external calls)
- Never ignore context cancellation

---

## Request Context Conventions

- Every request should carry a request ID
- Request ID must be added at the middleware layer
- Propagate request ID through `context.Context`
- Include request ID in logs for traceability
- When available, propagate trace/span identifiers consistently with request ID
---


## Security Conventions
- Never log secrets or credentials
- Validate and sanitize client input where appropriate
- Do not expose internal stack traces in API responses
- Keep error responses safe for clients

---

## Concurrency Conventions

- Prefer simple synchronous code unless concurrency provides a clear benefit
- Concurrency must be explicit, bounded, and cancellable
- Always propagate `context.Context` into concurrent work when possible
- Never start goroutines without a clear ownership and shutdown path
- Do not use concurrency as a substitute for good algorithm or query design

### Goroutine Rules

- A goroutine must have one of these purposes:
  - parallelizing independent work
  - background processing with explicit lifecycle management
  - streaming or fan-out/fan-in coordination
- Every goroutine must have a termination condition
- Goroutines spawned during request handling must respect request cancellation
- Long-lived goroutines must be started only from application bootstrap or managed components
- Do not start fire-and-forget goroutines inside handlers or services

### Primitive Selection

- Use channels for:
  - ownership transfer of data
  - signaling
  - pipelines and worker coordination
- Use `sync.Mutex` or `sync.RWMutex` for protecting shared in-memory state
- Prefer `sync.Mutex` by default; use `sync.RWMutex` only when read contention is proven to matter
- Use `sync.WaitGroup` only to wait for a fixed set of goroutines to finish
- Prefer `errgroup.Group` when concurrent tasks can fail, need cancellation, or are request-scoped
- Use `atomic` operations only for simple counters, flags, or low-level coordination
- Do not mix channels, mutexes, and atomics in the same flow unless the design is clearly justified

### Channel Conventions

- Channel ownership must be clear
- The sender is responsible for closing a channel when closure is required
- Do not close a channel from the receiver side
- Do not close a channel with multiple concurrent senders unless closure is externally coordinated
- Prefer unbuffered channels by default
- Use buffered channels only when you can justify the capacity and backpressure behavior
- Avoid using channels as general-purpose queues when a simpler abstraction is sufficient
- Use `select` when waiting on multiple signals, channel operations, or cancellation
- Every blocking channel operation in long-lived or request-scoped code should consider `ctx.Done()` when cancellation matters

### Mutex Conventions

- Keep critical sections small
- Do not hold locks while performing I/O, database calls, network calls, or blocking channel operations
- Avoid nested locks unless lock ordering is explicit and documented
- Prefer encapsulating mutable shared state behind a type with well-defined methods

### errgroup Conventions

- Use `errgroup.WithContext` for request-scoped parallel work
- If one task failure should cancel sibling work, prefer `errgroup` over `WaitGroup`
- Return the first meaningful error and rely on context cancellation for the rest
- Do not ignore errors produced by concurrent tasks

### Worker Pool Conventions

- Use worker pools only when you need bounded concurrency for many homogeneous tasks
- Do not introduce a worker pool for a small fixed number of tasks; use `errgroup` instead
- Worker pools must define:
  - max worker count
  - input ownership
  - shutdown behavior
  - error handling strategy
  - backpressure behavior
- Workers must stop on context cancellation or input channel closure
- Keep job payloads explicit and typed

### Select Conventions

- Use `select` to coordinate:
  - cancellation
  - timeouts
  - multiple input sources
  - send/receive readiness when necessary
- Avoid empty `default` branches that create busy loops
- Use `time.Ticker` carefully and always stop it
- Prefer `context`-driven cancellation over ad hoc done channels unless a done channel is the clearer ownership model

### Performance and Safety

- Measure before adding concurrency for performance reasons
- Bound concurrency when calling external systems or the database
- Avoid unbounded goroutine creation from loops or queues
- Be explicit about ordering guarantees when processing concurrently
- Protect all shared mutable state
- Run tests with the race detector when concurrency is involved

### Testing Concurrency

- Keep concurrency tests deterministic where possible
- Prefer testing behavior and cancellation semantics over timing assumptions
- Avoid fragile sleeps in tests; use synchronization or eventually-style assertions only when justified
- Add race-detector coverage for packages that use shared mutable state or goroutines
