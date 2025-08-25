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
    echo "Usage: ./pudl [command]"
    echo ""
    echo "Available commands:"
    echo "  ./pudl --help                 # Show all available commands"
    echo "  ./pudl version                # Show version information"
    echo "  ./pudl process <cue-file>     # Process CUE files with custom functions"
    echo ""
    echo "Examples:"
    echo "  ./pudl process example.cue"
    echo "  ./pudl process simple_test.cue"
else
    echo "❌ Build failed!"
    exit 1
fi
