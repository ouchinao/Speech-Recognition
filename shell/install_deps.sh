#!/bin/bash

set -e

echo "システム依存関係をインストール中..."


# OS判定
UNAME=$(uname)
if [ "$UNAME" = "Darwin" ]; then
    echo "macOSを検出しました。Homebrewで依存関係をインストールします。"
    if ! command -v brew >/dev/null 2>&1; then
        echo "Homebrewが見つかりません。公式サイトの手順でインストールしてください: https://brew.sh/"
        exit 1
    fi
    brew update
    brew install portaudio sox ffmpeg cmake git
    echo "✅"
elif [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
    if [[ "$OS" == "ubuntu" ]] || [[ "$OS" == "debian" ]]; then
        echo "Ubuntu/Debianを検出しました"
        sudo apt-get update
        sudo apt-get install -y \
            build-essential \
            git \
            cmake \
            pkg-config \
            portaudio19-dev \
            libportaudio2 \
            wget \
            curl
        echo "✅"
    elif [[ "$OS" == "centos" ]] || [[ "$OS" == "rhel" ]] || [[ "$OS" == "fedora" ]]; then
        echo "CentOS/RHEL/Fedoraを検出しました"
        sudo yum install -y \
            gcc \
            gcc-c++ \
            git \
            cmake \
            pkgconfig \
            portaudio-devel \
            wget \
            curl
        echo "✅"
    else
        echo "⚠️"
        echo "手動で以下をインストールしてください："
        echo "  - build-essential/gcc/g++"
        echo "  - git"
        echo "  - cmake"
        echo "  - pkg-config"
        echo "  - portaudio (開発パッケージ)"
        exit 1
    fi
else
    echo "❌ 未対応のOS: $UNAME"
    exit 1
fi
