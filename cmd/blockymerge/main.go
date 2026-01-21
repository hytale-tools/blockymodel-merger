package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"blockymerge/pkg/blockymodel"
	"blockymerge/pkg/character"
	"blockymerge/pkg/export"
	"blockymerge/pkg/merger"
	"blockymerge/pkg/registry"
	"blockymerge/pkg/texture"
)

const (
	basePath        = "assets/Characters/Player.blockymodel"
	baseTexturePath = "Characters/Player_Textures/Player_Greyscale.png"
	outputDir       = "output"
	defaultOutput   = "merged"
)

func main() {
	debug := flag.Bool("debug", false, "Print debug output showing node tree")
	charFile := flag.String("char", "", "Path to character JSON file")
	outputName := flag.String("out", defaultOutput, "Output file name (without extension)")
	formatFlag := flag.String("format", "both", "Output format: glb, blockymodel, or both")
	noTint := flag.Bool("no-tint", false, "Skip texture tinting (output raw greyscale)")
	flag.Parse()

	var accessoryPaths []character.AccessoryPath
	var gradientSets *texture.GradientSets
	var charData *character.CharacterData

	// Determine input mode
	if *charFile != "" {
		// Load gradient sets for tinting
		var err error
		gradientSets, err = texture.LoadGradientSets()
		if err != nil {
			fmt.Printf("Warning: Could not load gradient sets: %v\n", err)
		}

		// Load accessory registry
		fmt.Println("Loading accessory registry...")
		reg, err := registry.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
			os.Exit(1)
		}

		// Load character data
		fmt.Printf("Loading character data: %s\n", *charFile)
		charData, err = character.Load(*charFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading character file: %v\n", err)
			os.Exit(1)
		}

		result, err := charData.ResolveAccessories(reg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving accessories: %v\n", err)
			os.Exit(1)
		}

		if len(result.Warnings) > 0 {
			fmt.Println("Warnings:")
			for _, warn := range result.Warnings {
				fmt.Printf("  ! %s\n", warn)
			}
			fmt.Println()
		}

		fmt.Printf("Resolved %d accessories:\n", len(result.Accessories))
		for _, acc := range result.Accessories {
			fmt.Printf("  - %s: %s → %s\n", acc.Type, acc.Spec.ID, acc.Path)
		}
		fmt.Println()

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
	fmt.Printf("Loading base model: %s\n", basePath)
	base, err := blockymodel.Load(basePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading base model: %v\n", err)
		os.Exit(1)
	}

	// Create merger
	m, err := merger.New(base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating merger: %v\n", err)
		os.Exit(1)
	}

	// Merge each accessory
	for _, acc := range accessoryPaths {
		fmt.Printf("Merging: %s\n", acc.Path)
		accessory, err := blockymodel.Load(acc.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading accessory %s: %v\n", acc.Path, err)
			os.Exit(1)
		}

		// Use accessory spec ID as the identifier for tracking texture offsets
		if err := m.Merge(accessory, acc.Spec.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Error merging accessory %s: %v\n", acc.Path, err)
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
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
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
		fmt.Println("\nLoading base texture...")
		if skinTone != "" {
			fmt.Printf("  Tinting base with skin tone: %s\n", skinTone)
			baseTinted, err := texture.ProcessAccessoryTexture(
				"_base",
				baseTexturePath,
				"Skin",
				skinTone,
				gradientSets,
			)
			if err != nil {
				fmt.Printf("  Warning: Could not tint base texture: %v\n", err)
			} else {
				fmt.Printf("  ✓ Base texture tinted: %dx%d\n", baseTinted.Image.Bounds().Dx(), baseTinted.Image.Bounds().Dy())
				tintedTextures = append(tintedTextures, baseTinted)
			}
		} else {
			// No skin tone - load base texture without tinting
			baseImg, err := texture.LoadImage(baseTexturePath)
			if err != nil {
				fmt.Printf("  Warning: Could not load base texture: %v\n", err)
			} else {
				baseTex := &texture.TintedTexture{
					Name:         "_base",
					Image:        baseImg,
					OriginalPath: baseTexturePath,
				}
				fmt.Printf("  ✓ Base texture: %dx%d (no skin tone)\n", baseImg.Bounds().Dx(), baseImg.Bounds().Dy())
				tintedTextures = append(tintedTextures, baseTex)
			}
		}

		fmt.Println("\nProcessing accessory textures...")
		for _, acc := range accessoryPaths {
			if acc.ResolvedTexture == nil {
				fmt.Printf("  Skipping %s: no texture info\n", acc.Spec.ID)
				continue
			}

			var tinted *texture.TintedTexture
			var err error

			if acc.ResolvedTexture.DirectTexture != "" {
				// Pre-colored texture - load directly without tinting
				fmt.Printf("  Loading direct: %s\n", acc.Spec.ID)
				img, loadErr := texture.LoadImage(acc.ResolvedTexture.DirectTexture)
				if loadErr != nil {
					fmt.Printf("    Warning: %v\n", loadErr)
					continue
				}
				tinted = &texture.TintedTexture{
					Name:         acc.Spec.ID,
					Image:        img,
					OriginalPath: acc.ResolvedTexture.DirectTexture,
				}
			} else if acc.ResolvedTexture.GreyscaleTexture != "" {
				// Greyscale texture - apply tinting
				fmt.Printf("  Tinting: %s (gradient=%s, color=%s)\n", acc.Spec.ID, acc.ResolvedTexture.GradientSet, acc.Spec.Color)
				tinted, err = texture.ProcessAccessoryTexture(
					acc.Spec.ID,
					acc.ResolvedTexture.GreyscaleTexture,
					acc.ResolvedTexture.GradientSet,
					acc.Spec.Color,
					gradientSets,
				)
				if err != nil {
					fmt.Printf("    Warning: %v\n", err)
					continue
				}
			} else {
				fmt.Printf("  Skipping %s: no texture path\n", acc.Spec.ID)
				continue
			}

			tintedTextures = append(tintedTextures, tinted)
		}

		// Pack textures into atlas using simple linear layout (base texture at 0,0)
		if len(tintedTextures) > 0 {
			fmt.Println("\nPacking texture atlas...")
			var err error
			atlas, err = texture.PackAtlasSimple(tintedTextures, 1)
			if err != nil {
				fmt.Printf("  Warning: Failed to pack atlas: %v\n", err)
			} else {
				fmt.Printf("  ✓ Atlas packed: %dx%d (%d textures)\n", atlas.Width, atlas.Height, len(atlas.Entries))

				// Update texture offsets in the merged model for each accessory
				fmt.Println("\nUpdating texture offsets...")
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
						fmt.Printf("  ✓ %s: offset +(%d, %d) applied to %d nodes\n", tex.Name, x, y, len(nodeIDs))
					}
				}
			}
		}
	}

	// Export GLB
	if outputGLB {
		glbPath := filepath.Join(outputDir, *outputName+".glb")
		fmt.Printf("\nExporting GLB: %s\n", glbPath)

		exporter := export.NewGLBExporter()

		// Set atlas size for proper UV calculation
		var materialIdx uint32
		if atlas != nil {
			w, h := atlas.Image.Bounds().Dx(), atlas.Image.Bounds().Dy()
			fmt.Printf("  Atlas dimensions for UV: %dx%d\n", w, h)
			exporter.SetAtlasSize(float64(w), float64(h))

			// Encode atlas to PNG bytes
			atlasBytes, err := texture.EncodePNG(atlas.Image)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error encoding atlas: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "Error exporting GLB: %v\n", err)
			os.Exit(1)
		}

		if err := exporter.Save(glbPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving GLB: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  ✓ GLB saved\n")
	}

	// Export BlockyModel
	if outputBlocky {
		blockyPath := filepath.Join(outputDir, *outputName+".blockymodel")
		fmt.Printf("\nExporting BlockyModel: %s\n", blockyPath)

		if err := blockymodel.Save(result, blockyPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving BlockyModel: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  ✓ BlockyModel saved\n")
	}

	// Save atlas
	if atlas != nil {
		atlasPath := filepath.Join(outputDir, *outputName+"_atlas.png")
		fmt.Printf("\nSaving texture atlas: %s\n", atlasPath)
		if err := texture.SaveImage(atlas.Image, atlasPath); err != nil {
			fmt.Printf("  Warning: Failed to save atlas: %v\n", err)
		} else {
			fmt.Printf("  ✓ Atlas saved (%dx%d)\n", atlas.Width, atlas.Height)
		}
	}

	fmt.Println("\nDone!")
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
	fmt.Println("  -debug     Print merged node tree")
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
