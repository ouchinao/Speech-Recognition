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

The codebase follows **Clean Architecture**, organised into the four canonical
layers. The directory names *are* the layer names, and **dependencies point
inward** (outer layers may import inner ones, never the reverse):

```
cmd/                                     frameworks & drivers / main — wiring only
  speech-recognition/                      CLI: microphone -> recognition -> console
  speech-recognition-server/               gRPC streaming server
internal/
  domain/        vad/                     entities / domain logic: adaptive VAD (stdlib only, pure)
  usecase/       recognition/             application: Service (mic) + Streamer (stream) + ports
  gateway/       grpcserver/              interface adapter: gRPC controller
                 output/                  interface adapter: console presenter
                 genproto/speechv1/       generated protobuf / gRPC transport types
  infrastructure/audio/                   driver: PortAudio capture (cgo)
                 recognizer/              driver: VOSK speech-to-text (cgo)
                 config/                  flag/env parsing
```

Layer responsibilities and the dependency rule:

| Layer | Packages | May import |
| --- | --- | --- |
| `domain` | `vad` | standard library only |
| `usecase` | `recognition` | (currently) standard library only — it owns its ports |
| `gateway` | `grpcserver`, `output`, `genproto` | `usecase`, `domain`, `gateway` |
| `infrastructure` | `audio`, `recognizer`, `config` | external libraries (VOSK, PortAudio) |
| `cmd` | the two binaries | everything (composition root) |

Key rule: **`usecase/recognition` owns the interfaces it consumes**
(`AudioSource`, `VoiceDetector`, `Recognizer`, `Printer`). Concrete drivers are
constructed in `cmd/` and injected. When adding a capability, add a method to
the relevant port and implement it in the adapter — never let an inner layer
import an outer one.

Hard rules (enforced by review):
- `domain` and `usecase` must not import other internal packages or third-party
  libraries (keep them pure and trivially testable).
- `usecase` must not import any `gateway` or `infrastructure` package.
- `gateway` must not import `infrastructure` (drivers are injected via `cmd`).
- Only `cmd/` may import across all layers and perform the wiring.

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
go test ./internal/domain/... ./internal/usecase/... ./internal/gateway/grpcserver/... \
        ./internal/gateway/output/... ./internal/infrastructure/config/...
                                        # the cgo-free subset — runs without native libs
gofmt -l .                              # list mis-formatted files (should be empty)
go vet ./...
```

Everything except `infrastructure/audio` and `infrastructure/recognizer` (and
the `cmd` binaries that wire them) is **cgo-free**, so prefer testing those
layers directly when native libraries are not installed.

## Go conventions

Two separate concerns — do not conflate them:

- **Architecture** (layers, dependency direction) follows **Clean Architecture**
  (the dependency rule + ports & adapters), described above.
- **Go idiom** (naming, error handling, interface placement, formatting) follows
  Effective Go and the conventions used in large Go OSS such as Kubernetes and
  Terraform. Those projects are references for *style*, not for the layer
  layout. See `.claude/skills/go-style` for the actionable checklist.

In short:

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

## Testing (TDD)

Develop test-first. For every behaviour change follow the red/green/refactor
cycle:

1. **Red** — write a failing test that specifies the desired behaviour, and run
   it to confirm it fails for the right reason.
2. **Green** — write the minimum code to make the test pass.
3. **Refactor** — clean up with the tests staying green.

- Drive new logic from the cgo-free packages (`config`, `vad`, `output`,
  `recognition`, `grpcserver`) so tests run without native libraries.
- Use fakes that satisfy the small consumer interfaces to test use cases in
  isolation; use `bufconn` for in-memory gRPC tests.
- Commit history should show tests arriving with (or before) the code they
  cover, not bolted on afterwards.

## Conventions specific to this repo

- Audio frames are **little-endian 16-bit PCM** (`[]byte`). Use
  `encoding/binary` for conversions, not manual bit twiddling.
- Sample rate, model path and VAD mode come from `infrastructure/config`; do not
  hard-code them elsewhere.
- User-facing output goes through `gateway/output` (the `Printer` interface),
  not raw `fmt.Print` scattered across the use case.

## Pull requests

- Write pull request **titles and bodies in English**, even when the working
  conversation is in another language. Keep titles short and imperative and
  reference the issue with `Closes #N`.
- Do not force-push branches that already have an open PR.

## Reference

- Effective Go — https://go.dev/doc/effective_go
- Go Code Review Comments — https://go.dev/wiki/CodeReviewComments
- Google Go Style Guide — https://google.github.io/styleguide/go/
- Uber Go Style Guide — https://github.com/uber-go/guide/blob/master/style.md
- Standard project layout — https://github.com/golang-standards/project-layout
