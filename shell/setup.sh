#!/bin/bash

set -e

echo "======================================"
echo "Speech Recognition - 自動セットアップ"
echo "======================================"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"


# 1. システム依存関係のインストール
echo ""
echo "[1/4] システム依存関係をインストール中..."

UNAME=$(uname)
if [ "$UNAME" = "Darwin" ]; then
    echo "macOSを検出しました。Homebrewで依存関係をインストールします。"
    if ! command -v brew >/dev/null 2>&1; then
        echo "Homebrewが見つかりません。公式サイトの手順でインストールしてください: https://brew.sh/"
        exit 1
    fi
    brew update
    brew install portaudio sox ffmpeg
    echo "✅"
elif [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
    if [[ "$OS" == "ubuntu" ]] || [[ "$OS" == "debian" ]]; then
        ./install_deps.sh
        echo "✅"
    else
        echo "❌ 未対応のLinuxディストリビューション: $OS"
        exit 1
    fi
else
    echo "❌ 未対応のOS: $UNAME"
    exit 1
fi

# 2. Whisper.cppのビルド
echo ""
echo "[2/4] Whisper.cppをビルド中..."
./build_whisper.sh

# 3. Goモジュールのインストール
echo ""
echo "[3/4] Goモジュールをインストール中..."
if [ ! -f "go.mod" ]; then
    go mod init speech-recognition
fi
go mod tidy
go get github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper
go get github.com/gordonklaus/portaudio

# 4. Whisperモデルのダウンロード
echo ""
echo "[4/4] Whisperモデルをダウンロード中..."
./download_model.sh

echo ""
echo "======================================"
echo "✅"
echo "======================================"
echo ""
echo "以下のコマンドで実行できます："
echo "  ./shell/run.sh"
echo ""
