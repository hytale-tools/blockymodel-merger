package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Path mappings: zip path -> output path
var pathMappings = map[string]string{
	"Common/Characters":     "assets/Characters",
	"Common/Cosmetics":      "assets/Cosmetics",
	"Common/TintGradients":  "assets/TintGradients",
	"Cosmetics/CharacterCreator": "data",
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <path-to-assets.zip>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExtracts required folders from Hytale server assets.zip to current directory\n")
		fmt.Fprintf(os.Stderr, "Maps:\n")
		fmt.Fprintf(os.Stderr, "  Common/Characters -> assets/Characters\n")
		fmt.Fprintf(os.Stderr, "  Common/Cosmetics -> assets/Cosmetics\n")
		fmt.Fprintf(os.Stderr, "  Common/TintGradients -> assets/TintGradients\n")
		fmt.Fprintf(os.Stderr, "  Cosmetics/CharacterCreator -> data/\n")
		os.Exit(1)
	}

	zipPath := os.Args[1]

	// Remove existing assets/ and data/ directories for clean extraction
	assetsPath := "assets"
	dataPath := "data"
	
	if err := os.RemoveAll(assetsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not remove existing assets/ directory: %v\n", err)
	} else {
		fmt.Printf("Removed existing assets/ directory...\n")
	}
	
	if err := os.RemoveAll(dataPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not remove existing data/ directory: %v\n", err)
	} else {
		fmt.Printf("Removed existing data/ directory...\n")
	}

	// Check if file exists first
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: File not found: %s\n", zipPath)
		fmt.Fprintf(os.Stderr, "Please provide the full path to the assets.zip file\n")
		os.Exit(1)
	}

	fmt.Printf("Opening %s...\n", zipPath)
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening zip file: %v\n", err)
		fmt.Fprintf(os.Stderr, "\nTroubleshooting:\n")
		fmt.Fprintf(os.Stderr, "  - Make sure the file path is correct\n")
		fmt.Fprintf(os.Stderr, "  - Use absolute path if relative path doesn't work: /full/path/to/assets.zip\n")
		fmt.Fprintf(os.Stderr, "  - Verify the file is a valid ZIP: file %s\n", zipPath)
		fmt.Fprintf(os.Stderr, "  - Check file permissions: ls -l %s\n", zipPath)
		os.Exit(1)
	}
	defer r.Close()

	extracted := 0
	skipped := 0

	// Extract files that match required paths
	for _, f := range r.File {
		// Find matching path mapping
		var outputPrefix string
		var relativePath string
		var found bool
		
		for zipPath, outPath := range pathMappings {
			if strings.HasPrefix(f.Name, zipPath+"/") {
				outputPrefix = outPath
				relativePath = strings.TrimPrefix(f.Name, zipPath+"/")
				found = true
				break
			} else if f.Name == zipPath {
				// It's the directory itself
				outputPrefix = outPath
				relativePath = ""
				found = true
				break
			}
		}

		if !found {
			skipped++
			continue
		}

		// Create output path (always relative to current directory)
		var outPath string
		if relativePath == "" {
			// It's the directory itself
			outPath = outputPrefix
		} else {
			outPath = filepath.Join(outputPrefix, relativePath)
		}

		// Create directory if needed
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(outPath, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Could not create directory %s: %v\n", outPath, err)
				continue
			}
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not create directory for %s: %v\n", outPath, err)
			continue
		}

		// Open file from zip
		rc, err := f.Open()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not open %s from zip: %v\n", f.Name, err)
			continue
		}

		// Create output file
		outFile, err := os.Create(outPath)
		if err != nil {
			rc.Close()
			fmt.Fprintf(os.Stderr, "Warning: Could not create %s: %v\n", outPath, err)
			continue
		}

		// Copy file contents
		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Error extracting %s: %v\n", f.Name, err)
			os.Remove(outPath) // Clean up partial file
			continue
		}

		extracted++
		if extracted%100 == 0 {
			fmt.Printf("Extracted %d files...\n", extracted)
		}
	}

	fmt.Printf("\nDone! Extracted %d files (skipped %d)\n", extracted, skipped)
	fmt.Printf("Files extracted to: assets/ and data/ directories\n")
}
