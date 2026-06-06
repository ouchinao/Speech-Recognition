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
├── cmd/
│   └── speech-recognition/   # entrypoint / composition root (flag wiring, signals)
└── internal/
    ├── config/               # runtime configuration + flag parsing
    ├── vad/                  # domain: adaptive voice activity detection (pure, no I/O)
    ├── recognition/          # use case: calibration + recognition loop (owns its interfaces)
    ├── audio/                # adapter: microphone capture via PortAudio (cgo)
    ├── recognizer/           # adapter: speech-to-text via VOSK (cgo)
    └── output/               # adapter: console presentation
```

Responsibilities:

| Layer | Package | Depends on |
| --- | --- | --- |
| Entrypoint | `cmd/speech-recognition` | everything (composition root) |
| Use case | `internal/recognition` | its own interfaces only |
| Domain | `internal/vad` | standard library only |
| Adapters | `internal/audio`, `internal/recognizer`, `internal/output` | external libraries |
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

## Development

```bash
go test ./...            # run the unit tests (pure packages require no cgo)
go build ./...           # build everything (requires PortAudio + VOSK)
```

## License

Released under the [MIT License](LICENSE).
