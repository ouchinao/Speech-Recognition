#!/bin/bash

set -e
PROJECT_DIR="$(pwd)"
echo "🔧 VOSKリアルタイム音声認識 セットアップ"
echo ""

# OS検出
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "📱 macOS 検出"
    
    if ! command -v brew &> /dev/null; then
        echo "❌ Homebrewがインストールされていません"
        exit 1
    fi
    
    echo "📦 依存関係インストール中..."
    brew install pkg-config portaudio
    
    # Pythonパッケージ経由でVOSKをインストール
    echo "📥 VOSK C API インストール中..."
    pip3 install vosk
    
    # libvoskを探す
    VOSK_LIB=$(find ~/Library/Python -name "libvosk.dylib" 2>/dev/null | head -1)
    if [ -z "$VOSK_LIB" ]; then
        VOSK_LIB=$(python3 -c "import vosk; import os; print(os.path.dirname(vosk.__file__))")/libvosk.dylib
    fi
    
    # ヘッダー取得
    wget -O /tmp/vosk_api.h https://raw.githubusercontent.com/alphacep/vosk-api/v0.3.50/src/vosk_api.h
    
    # インストール
    sudo cp "$VOSK_LIB" /usr/local/lib/
    sudo cp /tmp/vosk_api.h /usr/local/include/
    
    # 環境変数
    SHELL_RC="${HOME}/.zshrc"
    if [ ! -f "$SHELL_RC" ]; then
        SHELL_RC="${HOME}/.bash_profile"
    fi
    
    if ! grep -q "VOSK環境変数" "$SHELL_RC"; then
        echo "" >> "$SHELL_RC"
        echo "# VOSK環境変数" >> "$SHELL_RC"
        echo 'export CGO_CFLAGS="-I/usr/local/include"' >> "$SHELL_RC"
        echo 'export CGO_LDFLAGS="-L/usr/local/lib -lvosk"' >> "$SHELL_RC"
        echo 'export DYLD_LIBRARY_PATH="/usr/local/lib:$DYLD_LIBRARY_PATH"' >> "$SHELL_RC"
    fi
    
    export CGO_CFLAGS="-I/usr/local/include"
    export CGO_LDFLAGS="-L/usr/local/lib -lvosk"
    export DYLD_LIBRARY_PATH="/usr/local/lib:$DYLD_LIBRARY_PATH"
    
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "🐧 Linux 検出"
    
    echo "📦 依存関係インストール中..."
    sudo apt-get update
    sudo apt-get install -y \
        portaudio19-dev \
        pkg-config \
        build-essential \
        wget \
        unzip \
        python3 \
        python3-pip
    
    # Pythonパッケージ経由でVOSKをインストール
    echo "📥 VOSK C API インストール中（Pythonパッケージ経由）..."
    pip3 install vosk
    
    # libvoskを探す
    VOSK_LIB=$(find ~/.local/lib -name "libvosk.so" 2>/dev/null | head -1)
    if [ -z "$VOSK_LIB" ]; then
        VOSK_LIB=$(python3 -c "import vosk; import os; print(os.path.dirname(vosk.__file__))")/libvosk.so
    fi
    
    if [ ! -f "$VOSK_LIB" ]; then
        echo "❌ libvosk.soが見つかりません"
        echo "手動でインストールしてください"
        exit 1
    fi
    
    # ヘッダー取得
    wget -O /tmp/vosk_api.h https://raw.githubusercontent.com/alphacep/vosk-api/v0.3.50/src/vosk_api.h
    
    # インストール
    sudo cp "$VOSK_LIB" /usr/local/lib/
    sudo cp /tmp/vosk_api.h /usr/local/include/
    sudo ldconfig
    
    echo "✅ VOSK C API インストール完了"
    
    # 環境変数
    SHELL_RC="${HOME}/.bashrc"
    
    if ! grep -q "VOSK環境変数" "$SHELL_RC"; then
        echo "" >> "$SHELL_RC"
        echo "# VOSK環境変数" >> "$SHELL_RC"
        echo 'export CGO_CFLAGS="-I/usr/local/include"' >> "$SHELL_RC"
        echo 'export CGO_LDFLAGS="-L/usr/local/lib -lvosk"' >> "$SHELL_RC"
        echo 'export LD_LIBRARY_PATH="/usr/local/lib:$LD_LIBRARY_PATH"' >> "$SHELL_RC"
    fi
    
    export CGO_CFLAGS="-I/usr/local/include"
    export CGO_LDFLAGS="-L/usr/local/lib -lvosk"
    export LD_LIBRARY_PATH="/usr/local/lib:$LD_LIBRARY_PATH"
    
else
    echo "❌ サポートされていないOS: $OSTYPE"
    exit 1
fi

# 元のディレクトリに戻る
cd "$PROJECT_DIR"

# Goモジュール初期化
echo "📦 Goモジュール初期化中..."
rm -f go.mod go.sum
go mod init speech-recognition
go get github.com/alphacep/vosk-api/go
go get github.com/gordonklaus/portaudio
go mod tidy

# VOSKモデルダウンロード
if [ ! -d "models" ]; then
    echo "📥 VOSKモデルダウンロード中..."
    echo "ビッグモデル (1GB) をダウンロードします..."
    
    wget https://alphacephei.com/vosk/models/vosk-model-ja-0.22.zip
    unzip vosk-model-ja-0.22.zip
    mv vosk-model-ja-0.22 models
    rm vosk-model-ja-0.22.zip
    
    echo "✅ モデルダウンロード完了"
else
    echo "✅ モデルは既に存在します"
fi

echo ""
echo "🎉 セットアップ完了!"
echo ""
echo "⚠️  重要: 新しいターミナルを開くか、以下を実行してください:"
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "  source ~/.zshrc"
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "  source ~/.bashrc"
fi
echo ""
echo "実行するには:"
echo "  bash shell/run.sh"
