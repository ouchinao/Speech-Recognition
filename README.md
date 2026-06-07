# Speech Recognition

Real-time Japanese speech recognition CLI built on [VOSK](https://alphacephei.com/vosk/)
and [PortAudio](http://www.portaudio.com/). Audio is captured from the default
microphone, gated by an adaptive voice-activity detector (VAD), and transcribed
incrementally.

## Architecture

The project follows a clean-architecture-inspired layout: dependencies point
inward toward the use case, and the entrypoint wires concrete drivers into the
interfaces the use case defines.

```
.
├── proto/speech/v1/             # gRPC service definition (.proto)
├── cmd/
│   ├── speech-recognition/       # CLI: microphone -> recognition -> console
│   └── speech-recognition-server/# gRPC streaming server (composition root)
└── internal/
    ├── config/               # runtime configuration + flag parsing
    ├── vad/                  # domain: adaptive voice activity detection (pure, no I/O)
    ├── recognition/          # use cases: mic loop (Service) + streaming (Streamer)
    ├── audio/                # adapter: microphone capture via PortAudio (cgo)
    ├── recognizer/           # adapter: speech-to-text via VOSK (cgo)
    ├── output/               # adapter: console presentation
    ├── grpcserver/           # adapter: gRPC transport for the streaming use case
    └── genproto/             # generated protobuf / gRPC code
```

Responsibilities:

| Layer | Package | Depends on |
| --- | --- | --- |
| Entrypoints | `cmd/speech-recognition`, `cmd/speech-recognition-server` | everything (composition roots) |
| Use case | `internal/recognition` | its own interfaces only |
| Domain | `internal/vad` | standard library only |
| Adapters | `internal/audio`, `internal/recognizer`, `internal/output`, `internal/grpcserver` | external libraries |
| Config | `internal/config` | standard library only |

`internal/recognition` declares the `AudioSource`, `VoiceDetector`, `Recognizer`
and `Printer` interfaces it consumes, so the concrete PortAudio/VOSK/console
drivers are injected at start-up and can be replaced with fakes in tests.

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

The service ([`proto/speech/v1/speech.proto`](proto/speech/v1/speech.proto))
offers two RPCs, matching the two deployment topologies:

- **`Recognize`** (bidirectional) — the **client** streams audio: a
  `RecognitionConfig` first, then `audio_content` chunks; the server streams
  back `RecognizeResponse` (`text` + `is_final`). Use this when the audio
  originates elsewhere (phone, browser, or an edge capture service forwarding to
  a shared cloud backend). Driven by `internal/recognition.Streamer`.
- **`RecognizeMicrophone`** (server-streaming) — the **server** transcribes its
  own local microphone and streams results back. Use this for an edge /
  single-device deployment where the box running the service has the mic. Only
  one such stream is active at a time (one physical device). Driven by the same
  `internal/recognition.Service` as the CLI (VAD + calibration), with a gRPC
  printer instead of the console.

Both share the VOSK adapter; only the audio source and result sink differ.

### Regenerating the gRPC stubs

The generated code under `internal/genproto/` is committed. Regenerate it after
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
