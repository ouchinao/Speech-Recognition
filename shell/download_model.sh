#!/bin/sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
MODELS_DIR="$PROJECT_ROOT/models"

echo "Whisperモデルをダウンロード中..."

mkdir -p "$MODELS_DIR"
cd "$MODELS_DIR"

if [ ! -f "ggml-tiny.bin" ]; then
    echo "ggml-tiny.binをダウンロード中..."
    curl -L https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin -o ggml-tiny.bin
fi

echo "✅"
