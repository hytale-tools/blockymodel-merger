package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
	"github.com/hytale-tools/blockymodel-merger/pkg/character"
	"github.com/hytale-tools/blockymodel-merger/pkg/export"
	"github.com/hytale-tools/blockymodel-merger/pkg/merger"
	"github.com/hytale-tools/blockymodel-merger/pkg/registry"
	"github.com/hytale-tools/blockymodel-merger/pkg/texture"
	"github.com/hytale-tools/blockymodel-merger/pkg/util"
)

const (
	basePath        = "assets/Characters/Player.blockymodel"
	baseTexturePath = "assets/Characters/Player_Textures/Player_Greyscale.png"
	outputDir       = "output"
	defaultOutput   = "merged"
)

func main() {
	debug := flag.Bool("debug", false, "Print debug output showing node tree")
	verbose := flag.Bool("verbose", false, "Print verbose output (info messages)")
	charFile := flag.String("char", "", "Path to character JSON file")
	outputName := flag.String("out", defaultOutput, "Output file name (without extension)")
	formatFlag := flag.String("format", "both", "Output format: glb, blockymodel, or both")
	noTint := flag.Bool("no-tint", false, "Skip texture tinting (output raw greyscale)")
	flag.Parse()

	// Initialize verbose mode (checks env var, CLI flag takes precedence)
	verboseEnabled := *verbose || os.Getenv("BLOCKYMERGE_VERBOSE") != ""
	util.SetVerbose(&verboseEnabled)

	var accessoryPaths []character.AccessoryPath
	var gradientSets *texture.GradientSets
	var charData *character.CharacterData

	// Determine input mode
	if *charFile != "" {
		// Load gradient sets for tinting
		var err error
		gradientSets, err = texture.LoadGradientSets()
		if err != nil {
			util.Logger.Warn("Could not load gradient sets", "error", err)
		}

		// Load accessory registry
		util.Logger.Info("Loading accessory registry")
		reg, err := registry.Load()
		if err != nil {
			util.Logger.Error("Error loading registry", "error", err)
			os.Exit(1)
		}

		// Load character data
		util.Logger.Info("Loading character data", "file", *charFile)
		charData, err = character.Load(*charFile)
		if err != nil {
			util.Logger.Error("Error loading character file", "file", *charFile, "error", err)
			os.Exit(1)
		}

		result, err := charData.ResolveAccessories(reg)
		if err != nil {
			util.Logger.Error("Error resolving accessories", "error", err)
			os.Exit(1)
		}

		if len(result.Warnings) > 0 {
			util.Logger.Warn("Warnings encountered", "count", len(result.Warnings))
			for _, warn := range result.Warnings {
				util.Logger.Warn("Warning", "message", warn)
			}
		}

		util.Logger.Info("Resolved accessories", "count", len(result.Accessories))
		for _, acc := range result.Accessories {
			util.Logger.Debug("Accessory resolved",
				"type", acc.Type,
				"id", acc.Spec.ID,
				"path", acc.Path)
		}

		accessoryPaths = result.Accessories
	} else {
		// Direct paths from command line
		args := flag.Args()
		if len(args) == 0 {
			printUsage()
			os.Exit(1)
		}
		for _, p := range args {
			accessoryPaths = append(accessoryPaths, character.AccessoryPath{
				Path: p,
				Spec: character.AccessorySpec{ID: filepath.Base(p)},
			})
		}
	}

	// Load base player model
	util.Logger.Info("Loading base model", "path", basePath)
	base, err := blockymodel.Load(basePath)
	if err != nil {
		util.Logger.Error("Error loading base model", "path", basePath, "error", err)
		os.Exit(1)
	}

	// Create merger
	m, err := merger.New(base)
	if err != nil {
		util.Logger.Error("Error creating merger", "error", err)
		os.Exit(1)
	}

	// Merge each accessory
	for _, acc := range accessoryPaths {
		util.Logger.Info("Merging accessory", "path", acc.Path)
		accessory, err := blockymodel.Load(acc.Path)
		if err != nil {
			util.Logger.Error("Error loading accessory", "path", acc.Path, "error", err)
			os.Exit(1)
		}

		// Use accessory spec ID as the identifier for tracking texture offsets
		if err := m.Merge(accessory, acc.Spec.ID); err != nil {
			util.Logger.Error("Error merging accessory", "path", acc.Path, "error", err)
			os.Exit(1)
		}
	}

	// Get result
	result := m.Result()

	// Debug output
	if *debug {
		fmt.Println("\n--- Merged Node Tree ---")
		printNodeTree(result.Nodes, 0)
		fmt.Println("------------------------")
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		util.Logger.Error("Error creating output directory", "dir", outputDir, "error", err)
		os.Exit(1)
	}

	// Determine output formats
	outputGLB := *formatFlag == "glb" || *formatFlag == "both"
	outputBlocky := *formatFlag == "blockymodel" || *formatFlag == "both"

	// Process textures if we have character data
	var tintedTextures []*texture.TintedTexture
	var atlas *texture.Atlas
	if *charFile != "" && !*noTint && charData != nil {
		// Get skin tone for base player texture
		skinTone := charData.GetSkinTone()

		// Load and tint base player texture
		util.Logger.Info("Loading base texture")
		if skinTone != "" {
			util.Logger.Info("Tinting base with skin tone", "skinTone", skinTone)
			baseTinted, err := texture.ProcessAccessoryTexture(
				"_base",
				baseTexturePath,
				"Skin",
				skinTone,
				gradientSets,
			)
			if err != nil {
				util.Logger.Warn("Could not tint base texture", "error", err)
			} else {
				util.Logger.Info("Base texture tinted",
					"width", baseTinted.Image.Bounds().Dx(),
					"height", baseTinted.Image.Bounds().Dy())
				tintedTextures = append(tintedTextures, baseTinted)
			}
		} else {
			// No skin tone - load base texture without tinting
			baseImg, err := texture.LoadImage(baseTexturePath)
			if err != nil {
				util.Logger.Warn("Could not load base texture", "error", err)
			} else {
				baseTex := &texture.TintedTexture{
					Name:         "_base",
					Image:        baseImg,
					OriginalPath: baseTexturePath,
				}
				util.Logger.Info("Base texture loaded (no skin tone)",
					"width", baseImg.Bounds().Dx(),
					"height", baseImg.Bounds().Dy())
				tintedTextures = append(tintedTextures, baseTex)
			}
		}

		util.Logger.Info("Processing accessory textures")
		for _, acc := range accessoryPaths {
			if acc.ResolvedTexture == nil {
				util.Logger.Debug("Skipping accessory: no texture info", "id", acc.Spec.ID)
				continue
			}

			var tinted *texture.TintedTexture
			var err error

			if acc.ResolvedTexture.DirectTexture != "" {
				// Pre-colored texture - load directly without tinting
				util.Logger.Debug("Loading direct texture", "id", acc.Spec.ID, "path", acc.ResolvedTexture.DirectTexture)
				img, loadErr := texture.LoadImage(acc.ResolvedTexture.DirectTexture)
				if loadErr != nil {
					util.Logger.Warn("Failed to load direct texture", "id", acc.Spec.ID, "error", loadErr)
					continue
				}
				tinted = &texture.TintedTexture{
					Name:         acc.Spec.ID,
					Image:        img,
					OriginalPath: acc.ResolvedTexture.DirectTexture,
				}
			} else if acc.ResolvedTexture.GreyscaleTexture != "" {
				// Greyscale texture - apply tinting
				util.Logger.Debug("Tinting texture",
					"id", acc.Spec.ID,
					"gradientSet", acc.ResolvedTexture.GradientSet,
					"color", acc.Spec.Color)
				tinted, err = texture.ProcessAccessoryTexture(
					acc.Spec.ID,
					acc.ResolvedTexture.GreyscaleTexture,
					acc.ResolvedTexture.GradientSet,
					acc.Spec.Color,
					gradientSets,
				)
				if err != nil {
					util.Logger.Warn("Failed to process accessory texture", "id", acc.Spec.ID, "error", err)
					continue
				}
			} else {
				util.Logger.Debug("Skipping accessory: no texture path", "id", acc.Spec.ID)
				continue
			}

			tintedTextures = append(tintedTextures, tinted)
		}

		// Pack textures into atlas using simple linear layout (base texture at 0,0)
		if len(tintedTextures) > 0 {
			util.Logger.Info("Packing texture atlas")
			var err error
			atlas, err = texture.PackAtlasSimple(tintedTextures, 1)
			if err != nil {
				util.Logger.Warn("Failed to pack atlas", "error", err)
			} else {
				util.Logger.Info("Atlas packed",
					"width", atlas.Width,
					"height", atlas.Height,
					"textureCount", len(atlas.Entries))

				// Update texture offsets in the merged model for each accessory
				util.Logger.Info("Updating texture offsets")
				for _, tex := range tintedTextures {
					if tex.Name == "_base" {
						continue // Base texture stays at origin
					}

					x, y, _, _, ok := atlas.GetPixelCoords(tex.Name)
					if !ok {
						continue
					}

					// Find all node IDs that came from this accessory
					nodeIDs := make(map[string]bool)
					for nodeID, accessoryID := range m.NodeSources {
						if accessoryID == tex.Name {
							nodeIDs[nodeID] = true
						}
					}

					if len(nodeIDs) > 0 {
						offset := blockymodel.AtlasOffset{X: float64(x), Y: float64(y)}
						blockymodel.UpdateTextureOffsets(result.Nodes, nodeIDs, offset)
						util.Logger.Debug("Applied texture offset",
							"texture", tex.Name,
							"offsetX", x,
							"offsetY", y,
							"nodeCount", len(nodeIDs))
					}
				}
			}
		}
	}

	// Export GLB
	if outputGLB {
		glbPath := filepath.Join(outputDir, *outputName+".glb")
		util.Logger.Info("Exporting GLB", "path", glbPath)

		exporter := export.NewGLBExporter()

		// Set atlas size for proper UV calculation
		var materialIdx uint32
		if atlas != nil {
			w, h := atlas.Image.Bounds().Dx(), atlas.Image.Bounds().Dy()
			util.Logger.Debug("Atlas dimensions for UV", "width", w, "height", h)
			exporter.SetAtlasSize(float64(w), float64(h))

			// Encode atlas to PNG bytes
			atlasBytes, err := texture.EncodePNG(atlas.Image)
			if err != nil {
				util.Logger.Error("Error encoding atlas", "error", err)
				os.Exit(1)
			}

			texIdx := exporter.AddTexture(atlasBytes)
			materialIdx = exporter.AddMaterial("textured", texIdx)
		} else {
			// No atlas - use a default size
			exporter.SetAtlasSize(64, 64)
			materialIdx = 0 // Will need a default material
		}

		if err := exporter.ExportModel(result, materialIdx); err != nil {
			util.Logger.Error("Error exporting GLB", "error", err)
			os.Exit(1)
		}

		if err := exporter.Save(glbPath); err != nil {
			util.Logger.Error("Error saving GLB", "path", glbPath, "error", err)
			os.Exit(1)
		}
		util.Logger.Info("GLB saved", "path", glbPath)
	}

	// Export BlockyModel
	if outputBlocky {
		blockyPath := filepath.Join(outputDir, *outputName+".blockymodel")
		util.Logger.Info("Exporting BlockyModel", "path", blockyPath)

		if err := blockymodel.Save(result, blockyPath); err != nil {
			util.Logger.Error("Error saving BlockyModel", "path", blockyPath, "error", err)
			os.Exit(1)
		}
		util.Logger.Info("BlockyModel saved", "path", blockyPath)
	}

	// Save atlas
	if atlas != nil {
		atlasPath := filepath.Join(outputDir, *outputName+"_atlas.png")
		util.Logger.Info("Saving texture atlas", "path", atlasPath)
		if err := texture.SaveImage(atlas.Image, atlasPath); err != nil {
			util.Logger.Warn("Failed to save atlas", "path", atlasPath, "error", err)
		} else {
			util.Logger.Info("Atlas saved",
				"path", atlasPath,
				"width", atlas.Width,
				"height", atlas.Height)
		}
	}

	util.Logger.Info("Done")
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  blockymerge -char <character.json> [options]")
	fmt.Println("  blockymerge <accessory1.blockymodel> [accessory2...] [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -char      Path to character JSON file")
	fmt.Println("  -out       Output file name without extension (default: merged)")
	fmt.Println("  -format    Output format: glb, blockymodel, or both (default: both)")
	fmt.Println("  -no-tint   Skip texture tinting")
	fmt.Println("  -verbose   Print verbose output (info messages)")
	fmt.Println("  -debug     Print merged node tree")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  BLOCKYMERGE_VERBOSE  Set to any value to enable verbose output")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  blockymerge -char example-character-data.json")
	fmt.Println("  blockymerge -char example-character-data.json -format glb")
	fmt.Println("  blockymerge -char example-character-data.json -out my-avatar")
}

func printNodeTree(nodes []blockymodel.Node, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, node := range nodes {
		shapeType := "none"
		if node.Shape != nil && node.Shape.Type != "" {
			shapeType = node.Shape.Type
		}
		marker := ""
		if node.IsSkeletonReference() {
			marker = " [skeleton-ref]"
		}
		fmt.Printf("%s%s (id:%s, shape:%s)%s\n", indent, node.Name, node.ID, shapeType, marker)
		printNodeTree(node.Children, depth+1)
	}
}
