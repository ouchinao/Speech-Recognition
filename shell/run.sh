#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"


# OS判定
UNAME=$(uname)
if [ "$UNAME" = "Darwin" ]; then
    # macOS: DYLD_LIBRARY_PATHを設定
    export CGO_CFLAGS="-I/usr/local/include"
    export CGO_LDFLAGS="-L/usr/local/lib -lwhisper -lstdc++ -lm -pthread"
    export DYLD_LIBRARY_PATH="/usr/local/lib:$DYLD_LIBRARY_PATH"
else
    # Linux: LD_LIBRARY_PATHを設定
    export CGO_CFLAGS="-I/usr/local/include"
    export CGO_LDFLAGS="-L/usr/local/lib -lwhisper -lstdc++ -lm -pthread"
    export LD_LIBRARY_PATH="/usr/local/lib:$LD_LIBRARY_PATH"
fi

# モデルファイルをチェック
if [ ! -f "../models/ggml-tiny.bin" ]; then
    echo "❌ モデルファイルが見つかりません"
    echo "以下を実行してください："
    echo "  ./shell/setup.sh"
    exit 1
fi

echo "======================================"
echo "音声認識プログラムを起動中..."
echo "======================================"
echo ""

# 実行
go run ../main.go ../models/ggml-tiny.bin
