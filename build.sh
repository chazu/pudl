#!/bin/bash

# Build script for PUDL utility

echo "Building PUDL utility..."

# Ensure dependencies are up to date
go mod tidy

# Build the binary
go build -o pudl .

if [ $? -eq 0 ]; then
    echo "✅ Build successful! Binary created: ./pudl"
    echo ""
    echo "Usage: ./pudl <cue-file>"
    echo ""
    echo "Examples:"
    echo "  ./pudl example.cue"
    echo "  ./pudl simple_test.cue"
else
    echo "❌ Build failed!"
    exit 1
fi
