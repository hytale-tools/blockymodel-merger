package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
	"github.com/hytale-tools/blockymodel-merger/pkg/export"
	"github.com/hytale-tools/blockymodel-merger/pkg/texture"
	"github.com/hytale-tools/blockymodel-merger/pkg/util"
)

const defaultOutputDir = "output"

func main() {
	verbose := flag.Bool("verbose", false, "Print verbose output")
	modelPath := flag.String("model", "", "Path to item .blockymodel (or pass as positional arg)")
	texturePath := flag.String("texture", "", "Path to texture PNG (default: <model>_Texture.png next to the model)")
	outputPath := flag.String("out", "", "Output GLB path (default: output/<basename>.glb)")
	flag.Parse()

	verboseEnabled := *verbose || os.Getenv("BLOCKYMERGE_VERBOSE") != ""
	util.SetVerbose(&verboseEnabled)

	if *modelPath == "" {
		if args := flag.Args(); len(args) > 0 {
			*modelPath = args[0]
		} else {
			printUsage()
			os.Exit(1)
		}
	}

	modelBase := strings.TrimSuffix(*modelPath, filepath.Ext(*modelPath))
	if *texturePath == "" {
		*texturePath = modelBase + "_Texture.png"
	}
	if *outputPath == "" {
		*outputPath = filepath.Join(defaultOutputDir, filepath.Base(modelBase)+".glb")
	}

	util.Logger.Info("Loading blockymodel", "path", *modelPath)
	model, err := blockymodel.Load(*modelPath)
	if err != nil {
		util.Logger.Error("Failed to load model", "error", err)
		os.Exit(1)
	}

	util.Logger.Info("Loading texture", "path", *texturePath)
	f, err := os.Open(*texturePath)
	if err != nil {
		util.Logger.Error("Failed to open texture", "path", *texturePath, "error", err)
		os.Exit(1)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		util.Logger.Error("Failed to decode texture", "path", *texturePath, "error", err)
		os.Exit(1)
	}

	pngBytes, err := texture.EncodePNG(img)
	if err != nil {
		util.Logger.Error("Failed to encode texture PNG", "error", err)
		os.Exit(1)
	}

	w, h := img.Bounds().Dx(), img.Bounds().Dy()

	exporter := export.NewGLBExporter()
	exporter.SetAtlasSize(float64(w), float64(h))
	texIdx := exporter.AddTexture(pngBytes)
	matIdx := exporter.AddMaterial("textured", texIdx)

	if err := exporter.ExportModel(model, matIdx); err != nil {
		util.Logger.Error("Failed to export GLB", "error", err)
		os.Exit(1)
	}

	if dir := filepath.Dir(*outputPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			util.Logger.Error("Failed to create output directory", "dir", dir, "error", err)
			os.Exit(1)
		}
	}

	if err := exporter.Save(*outputPath); err != nil {
		util.Logger.Error("Failed to save GLB", "path", *outputPath, "error", err)
		os.Exit(1)
	}

	util.Logger.Info("GLB saved", "path", *outputPath, "textureSize", fmt.Sprintf("%dx%d", w, h))
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  item-to-glb -model <path.blockymodel> [-texture <path.png>] [-out <path.glb>]")
	fmt.Println("  item-to-glb <path.blockymodel>")
	fmt.Println()
	fmt.Println("Defaults:")
	fmt.Println("  -texture  <model-without-extension>_Texture.png (sibling file)")
	fmt.Println("  -out      output/<basename>.glb")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  item-to-glb assets/Items/Weapons/Dagger/Bronze.blockymodel")
	fmt.Println("  item-to-glb -model assets/Items/Weapons/Bow/Adamantite_Triple.blockymodel \\")
	fmt.Println("              -texture assets/Items/Weapons/Bow/Adamantite_Texture.png")
}
