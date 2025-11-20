#!/bin/sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
THIRD_PARTY_DIR="$PROJECT_ROOT/third_party"
WHISPER_DIR="$THIRD_PARTY_DIR/whisper.cpp"

echo "Whisper.cppをビルド中..."

# third_partyディレクトリ作成
mkdir -p "$THIRD_PARTY_DIR"

# Whisper.cppをクローン
if [ ! -d "$WHISPER_DIR" ]; then
    echo "Whisper.cppをクローン中..."
    cd "$THIRD_PARTY_DIR"
    git clone https://github.com/ggerganov/whisper.cpp.git
fi

# ビルド
cd "$WHISPER_DIR"
echo "CMakeでビルド中..."
cmake -B build
cmake --build build --config Release

# ライブラリとヘッダーをシステムにインストール
echo "ライブラリをインストール中..."

UNAME=$(uname)
if [ "$UNAME" = "Darwin" ]; then
    # macOS: .dylibをコピー
    sudo cp "$WHISPER_DIR/build/src/libwhisper.dylib"* /usr/local/lib/
    sudo cp "$WHISPER_DIR/build/ggml/src/libggml.dylib"* /usr/local/lib/
    sudo cp "$WHISPER_DIR/build/ggml/src/libggml-base.dylib"* /usr/local/lib/
    sudo cp "$WHISPER_DIR/build/ggml/src/libggml-cpu.dylib"* /usr/local/lib/
else
    # Linux: .soをコピー
    sudo cp "$WHISPER_DIR/build/src/libwhisper.so"* /usr/local/lib/
    sudo cp "$WHISPER_DIR/build/ggml/src/libggml.so"* /usr/local/lib/
    sudo cp "$WHISPER_DIR/build/ggml/src/libggml-base.so"* /usr/local/lib/
    sudo cp "$WHISPER_DIR/build/ggml/src/libggml-cpu.so"* /usr/local/lib/
    sudo ldconfig
fi
# ヘッダーファイルをコピー
sudo cp "$WHISPER_DIR/include/whisper.h" /usr/local/include/
sudo cp "$WHISPER_DIR/ggml/include"/*.h /usr/local/include/

sudo ldconfig

echo "✅"
