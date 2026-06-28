# Speech Recognition

Real-time Japanese speech recognition CLI built on [VOSK](https://alphacephei.com/vosk/)
and [PortAudio](http://www.portaudio.com/). Audio is captured from the default
microphone, gated by an adaptive voice-activity detector (VAD), and transcribed
incrementally.

## Architecture

The project follows **Clean Architecture**: the four layer directories are named
after the layers, and dependencies point inward (outer may import inner, never
the reverse).

```
.
├── proto/speech/v1/                       # gRPC service definition (.proto)
├── cmd/                                   # frameworks & drivers / main (wiring only)
│   ├── speech-recognition/                #   CLI: microphone -> recognition -> console
│   └── speech-recognition-server/         #   gRPC streaming server
└── internal/
    ├── domain/
    │   └── vad/                           # entities/domain logic: adaptive VAD (pure, no I/O)
    ├── usecase/
    │   └── recognition/                   # application: Service (mic) + Streamer (stream) + ports
    ├── gateway/
    │   ├── grpcserver/                    # interface adapter: gRPC controller
    │   ├── output/                        # interface adapter: console presenter
    │   └── genproto/speechv1/             # generated protobuf / gRPC transport types
    └── infrastructure/
        ├── audio/                         # driver: microphone capture via PortAudio (cgo)
        ├── recognizer/                    # driver: speech-to-text via VOSK (cgo)
        └── config/                        # flag/env parsing
```

Responsibilities and the dependency rule:

| Layer | Packages | May import |
| --- | --- | --- |
| `domain` | `vad` | standard library only |
| `usecase` | `recognition` | standard library only (it owns its ports) |
| `gateway` | `grpcserver`, `output`, `genproto` | `usecase`, `domain` |
| `infrastructure` | `audio`, `recognizer`, `config` | external libraries |
| `cmd` | the two binaries | everything (composition root) |

`usecase/recognition` declares the `AudioSource`, `VoiceDetector`, `Recognizer`
and `Printer` ports it consumes, so the concrete PortAudio/VOSK/console drivers
are injected at start-up and can be replaced with fakes in tests.

## Requirements

- Go 1.26+
- macOS or Ubuntu
- PortAudio and the VOSK C library (installed by the setup script)

## Install

After installing Go:

```bash
bash shell/setup.sh
```

This installs PortAudio and the VOSK C library, configures the required `CGO_*`
environment variables, and downloads the Japanese acoustic model.

## Usage

```bash
bash shell/run.sh
# or directly:
go run ./cmd/speech-recognition
```

Flags:

| Flag | Default | Description |
| --- | --- | --- |
| `-models` | `./models/vosk-model-ja-0.22` | Path to the VOSK model |
| `-vad-mode` | `1` | VAD sensitivity (0=lenient .. 3=strict) |
| `-calibration-time` | `5s` | Ambient-noise calibration duration |

## gRPC streaming server

Besides the local microphone CLI, the engine is exposed as a streaming gRPC
service so it can be used as a microservice.

```bash
go run ./cmd/speech-recognition-server   # listens on :50051 by default
```

Flags:

| Flag | Default | Description |
| --- | --- | --- |
| `-addr` | `:50051` | gRPC listen address |
| `-models` | `./models/vosk-model-ja-0.22` | Path to the VOSK model |
| `-sample-rate` | `16000` | Default sample rate when the client omits one |
| `-calibration-time` | `5s` | Calibration duration for `RecognizeMicrophone` |
| `-tls-cert` / `-tls-key` | _(none)_ | PEM cert+key; enables TLS. Without them the server logs a warning and serves plaintext |
| `-auth-token` | `$STT_AUTH_TOKEN` | Bearer token clients must send; empty disables auth (with a warning) |
| `-max-streams` | `64` | Maximum concurrent recognition streams (`ResourceExhausted` beyond it) |
| `-max-conn-idle` | `5m` | Close a connection after it has been idle this long |

### Security & reliability

The server is wrapped with gRPC interceptors (`internal/gateway/interceptor`):

- **Authentication** — when `-auth-token` (or `$STT_AUTH_TOKEN`) is set, every
  RPC must carry `authorization: Bearer <token>` metadata (constant-time
  compared); otherwise it is rejected with `Unauthenticated`.
- **TLS** — supply `-tls-cert`/`-tls-key` to serve over TLS. **Run with both TLS
  and a token in production**; without them the server prints a warning.
- **Panic recovery** — a handler panic becomes an `Internal` error instead of
  crashing the process.
- **Concurrency limit** — at most `-max-streams` live streams; excess streams
  get `ResourceExhausted`, bounding memory (one VOSK recognizer per stream).
- **Idle reaping** — `-max-conn-idle` keepalive closes idle connections.

Example client metadata: `grpcurl -H "authorization: Bearer $STT_AUTH_TOKEN" ...`.

The service ([`proto/speech/v1/speech.proto`](proto/speech/v1/speech.proto))
offers two RPCs, matching the two deployment topologies:

- **`Recognize`** (bidirectional) — the **client** streams audio: a
  `RecognitionConfig` first, then `audio_content` chunks; the server streams
  back `RecognizeResponse` (`text` + `is_final`). Use this when the audio
  originates elsewhere (phone, browser, or an edge capture service forwarding to
  a shared cloud backend). Driven by `usecase/recognition.Streamer`.
- **`RecognizeMicrophone`** (server-streaming) — the **server** transcribes its
  own local microphone and streams results back. Use this for an edge /
  single-device deployment where the box running the service has the mic. Only
  one such stream is active at a time (one physical device). Driven by the same
  `usecase/recognition.Service` as the CLI (VAD + calibration), with a gRPC
  printer instead of the console.

Both share the VOSK adapter; only the audio source and result sink differ.

### Regenerating the gRPC stubs

The generated code under `internal/gateway/genproto/` is committed. Regenerate it after
editing the `.proto` with `protoc` plus `protoc-gen-go` / `protoc-gen-go-grpc`:

```bash
protoc -I proto \
  --go_out=. --go_opt=module=speech-recognition \
  --go-grpc_out=. --go-grpc_opt=module=speech-recognition \
  proto/speech/v1/speech.proto
```

## Development

```bash
go test ./...            # run the unit tests (pure packages require no cgo)
go build ./...           # build everything (requires PortAudio + VOSK)
```

## License

Released under the [MIT License](LICENSE).
