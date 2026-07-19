#!/bin/bash

set -e

echo "Building blockymerge..."
go build -o blockymerge ./cmd/blockymerge

echo "Building blockyrender..."
go build -o blockyrender ./cmd/blockyrender

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
        ./blockyrender -char "$char_file" -view full-body -rotation 30 -o "output/$filename.png"
    fi
done

echo "Done processing all characters!"
