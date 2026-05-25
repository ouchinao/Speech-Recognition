#!/bin/bash

echo "VOSK real-time speech recognition starting..."
echo ""

if [ ! -d "models" ]; then
    echo "Model not found"
    echo "Please run setup: bash shell/setup.sh"
    exit 1
fi

go run main.go "$@"
