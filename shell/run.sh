#!/bin/bash

echo "🎤 VOSKリアルタイム音声認識 起動中..."
echo ""

# モデル存在確認
if [ ! -d "models" ]; then
    echo "❌ モデルが見つかりません"
    echo "セットアップを実行してください: bash shell/setup.sh"
    exit 1
fi

# アプリケーション実行
go run main.go "$@"
