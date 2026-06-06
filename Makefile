.PHONY: build test test-pure lint fmt vet run tidy

# Build every package (requires the PortAudio and VOSK native libraries).
build:
	go build -o bin/speech-recognition ./cmd/speech-recognition

# Run all tests.
test:
	go test -race ./...

# Run only the cgo-free packages (no native libraries required).
test-pure:
	go test ./internal/config/... ./internal/vad/... ./internal/output/... ./internal/recognition/...

lint:
	golangci-lint run

fmt:
	gofmt -w .

vet:
	go vet ./...

run:
	go run ./cmd/speech-recognition

tidy:
	go mod tidy
