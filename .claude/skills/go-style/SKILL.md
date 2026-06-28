---
name: go-style
description: Idiomatic Go conventions for this repository, distilled from Effective Go, Go Code Review Comments, and the Google/Uber style guides (the conventions Kubernetes and Terraform follow). Use when writing, reviewing, or refactoring Go code here — naming, error handling, package/interface design, testing, and the cmd/internal layout.
---

# Go style for this repository

Apply these rules whenever you add or change Go code. They encode the
conventions used across large Go OSS projects (Kubernetes, Terraform) and the
official references. When in doubt, run `gofmt`, `go vet`, and `golangci-lint`.

## Formatting & imports

- Always `gofmt` (tabs, gofmt brace style). Never hand-format.
- Group imports into three blocks separated by blank lines: standard library,
  third-party, then local (`speech-recognition/...`). `goimports` ordering.
- Line length is not fixed, but break long calls for readability.

## Naming

- Packages: short, lower-case, no underscores or plurals (`vad`, `audio`,
  `recognition`). The package name is part of every identifier — avoid stutter.
- Constructors are `New` and live in the package they construct: `vad.New`,
  `audio.New`, `output.NewConsole`. Do **not** write `vad.NewAdaptiveVAD`.
- Getters drop `Get`: `Threshold()`, not `GetThreshold()`. Setters keep `Set`.
- Exported identifiers use `MixedCaps`; unexported use `mixedCaps`.
- Acronyms keep their case: `RMS`, `VAD`, `URL`, `ID` (`CalculateRMS`, `vadMode`).
- Interface names: single-method interfaces often end in `-er` (`Reader`,
  `Printer`). Keep interfaces small.

## Errors

- Return errors, do not panic in library code. `panic` is only for truly
  unrecoverable programmer errors; `main` may `log.Fatal`.
- Wrap with context and `%w` so callers can `errors.Is`/`errors.As`:
  `return fmt.Errorf("open stream: %w", err)`.
- Error strings are lower-case, no trailing punctuation.
- Never ignore an error silently. If intentionally discarded, assign to `_` with
  a reason, e.g. best-effort teardown: `_ = stream.Close()`.
- Check errors immediately after the call; keep the happy path un-indented by
  returning early.

## Package & interface design

- **Accept interfaces, return concrete types.** Functions take the narrow
  interface they need; constructors return the concrete `*T`.
- **Declare interfaces where they are consumed**, not where they are
  implemented. In this repo the `recognition` use case declares `AudioSource`,
  `VoiceDetector`, `Recognizer`, `Printer`; the `audio`/`recognizer`/`output`
  packages just satisfy them structurally.
- Keep `domain/vad` and `usecase/recognition` dependency-free (stdlib only).
- Keep `main` a thin composition root: parse config, build drivers, wire, run.

## Context & concurrency

- Pass `context.Context` as the first parameter of long-running / cancellable
  functions; never store it in a struct.
- Use `signal.NotifyContext` for graceful shutdown instead of manual signal
  channels.
- Check `ctx.Err()` at loop boundaries and return it; treat `context.Canceled`
  as a clean stop at the top level (`errors.Is(err, context.Canceled)`).

## Documentation

- Every exported identifier has a doc comment beginning with its name
  (`// Service drives ...`). Package docs go on one file as `// Package x ...`.
- Comment *why*, not *what*; the code already says what.

## Testing (test-driven)

- **Write tests first** and follow red → green → refactor: a failing test that
  pins the behaviour, the minimum code to pass it, then cleanup with tests green.
  Confirm the red test fails for the intended reason before implementing.
- Table-driven tests with `t.Run` subtests for multiple cases.
- Use fakes that satisfy the small interfaces to test the use case without cgo;
  keep the cgo-free packages (`config`, `vad`, `output`, `recognition`,
  `grpcserver`) fully unit-tested. Use `bufconn` for in-memory gRPC tests.
- Failure messages read `got X, want Y`.
- Prefer the standard `testing` package; avoid heavyweight assertion libraries.

## cgo specifics for this repo

- Audio frames are little-endian 16-bit PCM `[]byte`; convert with
  `encoding/binary` (`binary.LittleEndian`), not manual byte shifts.
- Adapter packages (`audio`, `recognizer`) require the PortAudio and VOSK native
  libraries; the pure packages must remain buildable and testable without them.

## Pull requests

- **Write every pull request title and body in English**, regardless of the
  language used in the conversation or in commit messages. Keep titles short and
  imperative (e.g. `fix: prompt shutdown on signal`); structure the body with a
  brief summary and, when useful, a bullet list of the changes.
- Reference the issue the PR closes (`Closes #N`).
- Never force-push a branch that already has an open PR; integrate updates with
  normal pushes or merge commits instead.

## Quick checklist before committing

1. `gofmt -l .` prints nothing.
2. `go vet ./...` is clean.
3. `golangci-lint run` is clean (or run the cgo-free subset locally).
4. New exported symbols have doc comments.
5. Errors are wrapped with context; none are silently dropped.
6. No new dependency added from `vad`/`config` to other packages.
7. PR title and body are written in English.
