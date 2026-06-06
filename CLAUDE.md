# CLAUDE.md

Guidance for Claude Code (and humans) working in this repository.

## Project overview

Real-time Japanese speech recognition CLI built on **VOSK** (speech-to-text)
and **PortAudio** (microphone capture). Audio is captured from the default
input device, gated by an adaptive voice-activity detector (VAD), and
transcribed incrementally to the terminal.

Both VOSK and PortAudio are C libraries used through **cgo**, so building the
adapter packages requires those native libraries to be installed (see Build).

## Architecture

The codebase follows a clean-architecture-inspired layout. **Dependencies point
inward**: the entrypoint depends on everything, the use case depends only on the
interfaces it declares, and the domain depends on nothing but the standard
library.

```
cmd/speech-recognition/   entrypoint / composition root — wiring only
internal/
  config/                 runtime config + flag parsing        (stdlib only)
  vad/                    domain: adaptive VAD                  (stdlib only, pure)
  recognition/            use case: calibration + recognition loop
  audio/                  adapter: PortAudio capture            (cgo)
  recognizer/             adapter: VOSK speech-to-text          (cgo)
  output/                 adapter: console presentation
```

Key rule: **`internal/recognition` owns the interfaces it consumes**
(`AudioSource`, `VoiceDetector`, `Recognizer`, `Printer`). Concrete drivers are
constructed in `cmd/speech-recognition/main.go` and injected. When adding a
capability, add a method to the relevant interface there and implement it in the
adapter — do not let the use case import a driver package directly.

Layer dependency rules:
- `internal/vad` and `internal/config` must not import other internal packages
  or third-party libraries (keep them pure and trivially testable).
- `internal/recognition` must not import `internal/audio`, `internal/recognizer`
  or `internal/output` — it depends on its own interfaces only.
- Only `cmd/` may import everything and perform the wiring.

## Build, run, test

cgo flags are exported by `shell/setup.sh` into the shell profile:

```bash
export CGO_CFLAGS="-I/usr/local/include"
export CGO_LDFLAGS="-L/usr/local/lib -lvosk"
# Linux: LD_LIBRARY_PATH=/usr/local/lib   macOS: DYLD_LIBRARY_PATH=/usr/local/lib
```

```bash
bash shell/setup.sh                     # install PortAudio + VOSK, download model
go run ./cmd/speech-recognition         # run (needs a microphone + model)
go build ./...                          # build everything (needs cgo libs)
go test ./...                           # run tests
go test ./internal/config/... ./internal/vad/... ./internal/output/... ./internal/recognition/...
                                        # the cgo-free subset — runs without native libs
gofmt -l .                              # list mis-formatted files (should be empty)
go vet ./...
```

The `config`, `vad`, `output` and `recognition` packages have **no cgo
dependency**, so prefer testing those directly when native libraries are not
installed.

## Go conventions

This repo aims to match idiomatic Go as practised in large OSS codebases
(Kubernetes, Terraform) and the official references. See
`.claude/skills/go-style` for the actionable checklist. In short:

- Format with `gofmt`; keep imports grouped (stdlib / third-party / local).
- Wrap errors with context: `fmt.Errorf("load model %q: %w", path, err)`.
  Never discard an error silently; never `panic` in library code.
- Accept interfaces, return concrete types. Declare interfaces in the consuming
  package, keep them small.
- Constructors are `New` (e.g. `vad.New`, `audio.New`); avoid stutter like
  `vad.NewAdaptiveVAD`.
- Getters omit the `Get` prefix (`Threshold()`, not `GetThreshold()`).
- Every exported identifier has a doc comment that starts with its name.
- Use `context.Context` for cancellation; thread it through long-running loops.
- Keep `main` thin: parse, wire, run.

## Conventions specific to this repo

- Audio frames are **little-endian 16-bit PCM** (`[]byte`). Use
  `encoding/binary` for conversions, not manual bit twiddling.
- Sample rate, model path and VAD mode come from `internal/config`; do not
  hard-code them elsewhere.
- User-facing output goes through `internal/output` (the `Printer` interface),
  not raw `fmt.Print` scattered across the use case.

## Reference

- Effective Go — https://go.dev/doc/effective_go
- Go Code Review Comments — https://go.dev/wiki/CodeReviewComments
- Google Go Style Guide — https://google.github.io/styleguide/go/
- Uber Go Style Guide — https://github.com/uber-go/guide/blob/master/style.md
- Standard project layout — https://github.com/golang-standards/project-layout
