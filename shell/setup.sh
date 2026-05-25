#!/bin/bash

set -e
PROJECT_DIR="$(pwd)"
echo "VOSK real-time speech recognition setup"
echo ""

if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "macOS detected"

    if ! command -v brew &> /dev/null; then
        echo "Homebrew is not installed"
        exit 1
    fi

    echo "Installing dependencies..."
    brew install pkg-config portaudio

    echo "Installing VOSK C API..."
    pip3 install vosk

    VOSK_LIB=$(find ~/Library/Python -name "libvosk.dylib" 2>/dev/null | head -1)
    if [ -z "$VOSK_LIB" ]; then
        VOSK_LIB=$(python3 -c "import vosk; import os; print(os.path.dirname(vosk.__file__))")/libvosk.dylib
    fi

    wget -O /tmp/vosk_api.h https://raw.githubusercontent.com/alphacep/vosk-api/v0.3.50/src/vosk_api.h

    sudo cp "$VOSK_LIB" /usr/local/lib/
    sudo cp /tmp/vosk_api.h /usr/local/include/

    SHELL_RC="${HOME}/.zshrc"
    if [ ! -f "$SHELL_RC" ]; then
        SHELL_RC="${HOME}/.bash_profile"
    fi

    if ! grep -q "VOSK env vars" "$SHELL_RC"; then
        echo "" >> "$SHELL_RC"
        echo "# VOSK env vars" >> "$SHELL_RC"
        echo 'export CGO_CFLAGS="-I/usr/local/include"' >> "$SHELL_RC"
        echo 'export CGO_LDFLAGS="-L/usr/local/lib -lvosk"' >> "$SHELL_RC"
        echo 'export DYLD_LIBRARY_PATH="/usr/local/lib:$DYLD_LIBRARY_PATH"' >> "$SHELL_RC"
    fi

    export CGO_CFLAGS="-I/usr/local/include"
    export CGO_LDFLAGS="-L/usr/local/lib -lvosk"
    export DYLD_LIBRARY_PATH="/usr/local/lib:$DYLD_LIBRARY_PATH"

elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "Linux detected"

    echo "Installing dependencies..."
    sudo apt-get update
    sudo apt-get install -y \
        portaudio19-dev \
        pkg-config \
        build-essential \
        wget \
        unzip \
        python3 \
        python3-pip

    echo "Installing VOSK C API (via Python package)..."
    pip3 install vosk

    VOSK_LIB=$(find ~/.local/lib -name "libvosk.so" 2>/dev/null | head -1)
    if [ -z "$VOSK_LIB" ]; then
        VOSK_LIB=$(python3 -c "import vosk; import os; print(os.path.dirname(vosk.__file__))")/libvosk.so
    fi

    if [ ! -f "$VOSK_LIB" ]; then
        echo "libvosk.so not found"
        echo "Please install manually"
        exit 1
    fi

    wget -O /tmp/vosk_api.h https://raw.githubusercontent.com/alphacep/vosk-api/v0.3.50/src/vosk_api.h

    sudo cp "$VOSK_LIB" /usr/local/lib/
    sudo cp /tmp/vosk_api.h /usr/local/include/
    sudo ldconfig

    echo "VOSK C API installed"

    SHELL_RC="${HOME}/.bashrc"

    if ! grep -q "VOSK env vars" "$SHELL_RC"; then
        echo "" >> "$SHELL_RC"
        echo "# VOSK env vars" >> "$SHELL_RC"
        echo 'export CGO_CFLAGS="-I/usr/local/include"' >> "$SHELL_RC"
        echo 'export CGO_LDFLAGS="-L/usr/local/lib -lvosk"' >> "$SHELL_RC"
        echo 'export LD_LIBRARY_PATH="/usr/local/lib:$LD_LIBRARY_PATH"' >> "$SHELL_RC"
    fi

    export CGO_CFLAGS="-I/usr/local/include"
    export CGO_LDFLAGS="-L/usr/local/lib -lvosk"
    export LD_LIBRARY_PATH="/usr/local/lib:$LD_LIBRARY_PATH"

else
    echo "Unsupported OS: $OSTYPE"
    exit 1
fi

cd "$PROJECT_DIR"

echo "Initializing Go module..."
rm -f go.mod go.sum
go mod init speech-recognition
go get github.com/alphacep/vosk-api/go
go get github.com/gordonklaus/portaudio
go mod tidy

if [ ! -d "models" ]; then
    echo "Downloading VOSK model..."
    echo "Downloading big model (~1GB)..."

    wget https://alphacephei.com/vosk/models/vosk-model-ja-0.22.zip
    unzip vosk-model-ja-0.22.zip
    mv vosk-model-ja-0.22 models
    rm vosk-model-ja-0.22.zip

    echo "Model downloaded"
else
    echo "Model already exists"
fi

echo ""
echo "Setup complete!"
echo ""
echo "Important: open a new terminal or run the following:"
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "  source ~/.zshrc"
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "  source ~/.bashrc"
fi
echo ""
echo "To run:"
echo "  bash shell/run.sh"
