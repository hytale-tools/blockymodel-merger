#!/bin/bash

set -e

echo "Building blockymerge..."
go build -o blockymerge ./cmd/blockymerge

if [ -d "output" ]; then
    echo "Removing output folder..."
    rm -rf output
fi

mkdir -p output

echo "Processing character files..."
for char_file in characters/*.json; do
    if [ -f "$char_file" ]; then
        filename=$(basename "$char_file" .json)
        echo "Processing $filename..."
        ./blockymerge -char "$char_file" -out "$filename"
    fi
done

echo "Done processing all characters!"
