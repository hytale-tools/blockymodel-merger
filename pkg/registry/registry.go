package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultDataDir   = "data"
	defaultAssetsDir = "assets"
)

// TextureEntry represents a pre-colored texture option
type TextureEntry struct {
	Texture   string   `json:"Texture"`
	BaseColor []string `json:"BaseColor"`
}

// VariantEntry represents a variant option within an accessory
type VariantEntry struct {
	Model            string                  `json:"Model"`
	GreyscaleTexture string                  `json:"GreyscaleTexture"`
	Textures         map[string]TextureEntry `json:"Textures"` // For pre-colored textures
}

// AccessoryEntry represents an entry in a registry JSON file
type AccessoryEntry struct {
	ID               string                  `json:"Id"`
	Name             string                  `json:"Name"`
	Model            string                  `json:"Model"`
	GreyscaleTexture string                  `json:"GreyscaleTexture"`
	GradientSet      string                  `json:"GradientSet"`
	Variants         map[string]VariantEntry `json:"Variants"`
	Textures         map[string]TextureEntry `json:"Textures"` // For pre-colored textures at top level
}

// ResolvedTexture contains the resolved texture info for an accessory
type ResolvedTexture struct {
	GreyscaleTexture string   // Path to greyscale texture (for tinting)
	GradientSet      string   // Gradient set name (for tinting)
	DirectTexture    string   // Path to pre-colored texture (no tinting needed)
	BaseColor        []string // Base colors for display
}

// Registry holds all accessory registries loaded from data/*.json
type Registry struct {
	entries    map[string]map[string]AccessoryEntry // registryName -> id -> entry
	dataDir    string                                // Base path for data directory
	assetsDir  string                                // Base path for assets directory
}

// Mapping from character data field names to registry file names
var fieldToRegistry = map[string]string{
	"face":          "Faces",
	"ears":          "Ears",
	"eyes":          "Eyes",
	"eyebrows":      "Eyebrows",
	"mouth":         "Mouths",
	"facialHair":    "FacialHair",
	"haircut":       "Haircuts",
	"underwear":     "Underwear",
	"pants":         "Pants",
	"overpants":     "Overpants",
	"undertop":      "Undertops",
	"overtop":       "Overtops",
	"shoes":         "Shoes",
	"gloves":        "Gloves",
	"cape":          "Capes",
	"headAccessory": "HeadAccessory",
	"faceAccessory": "FaceAccessory",
	"earAccessory":  "EarAccessory",
	"skinFeature":   "SkinFeatures",
}

// Load reads all registry files from the data directory
// If basePath is empty, uses default "data" and "assets" directories
// Optional dataDirName and assetsDirName allow custom directory names (defaults: "data", "assets")
func Load(basePath ...string) (*Registry, error) {
	var base, dataDirName, assetsDirName string
	if len(basePath) > 0 {
		base = basePath[0]
	}
	if len(basePath) > 1 {
		dataDirName = basePath[1]
	}
	if len(basePath) > 2 {
		assetsDirName = basePath[2]
	}
	
	if dataDirName == "" {
		dataDirName = defaultDataDir
	}
	if assetsDirName == "" {
		assetsDirName = defaultAssetsDir
	}
	
	dataDir := dataDirName
	assetsDir := assetsDirName
	
	if base != "" {
		dataDir = filepath.Join(base, dataDirName)
		assetsDir = filepath.Join(base, assetsDirName)
	}
	
	r := &Registry{
		entries: make(map[string]map[string]AccessoryEntry),
		dataDir: dataDir,
		assetsDir: assetsDir,
	}

	for _, registryName := range fieldToRegistry {
		path := filepath.Join(r.dataDir, registryName+".json")
		if err := r.loadRegistry(registryName, path); err != nil {
			// Skip missing registries with a warning
			fmt.Printf("Warning: Could not load registry %s: %v\n", registryName, err)
			continue
		}
	}

	return r, nil
}

func (r *Registry) loadRegistry(name, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var entries []AccessoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}

	r.entries[name] = make(map[string]AccessoryEntry)
	for _, entry := range entries {
		if entry.ID != "" {
			r.entries[name][entry.ID] = entry
		}
	}

	return nil
}

// LookupWithVariant finds an accessory by field type, ID, and optional variant
func (r *Registry) LookupWithVariant(fieldType, id, variant string) (string, error) {
	registryName, ok := fieldToRegistry[fieldType]
	if !ok {
		return "", fmt.Errorf("unknown field type: %s", fieldType)
	}

	registry, ok := r.entries[registryName]
	if !ok {
		return "", fmt.Errorf("registry not loaded: %s", registryName)
	}

	entry, ok := registry[id]
	if !ok {
		return "", fmt.Errorf("accessory '%s' not found in %s registry", id, registryName)
	}

	var modelPath string

	// Check for variant first if specified
	if variant != "" && entry.Variants != nil {
		if variantEntry, ok := entry.Variants[variant]; ok {
			modelPath = variantEntry.Model
		}
	}

	// Fall back to top-level model
	if modelPath == "" {
		modelPath = entry.Model
	}

	// If still no model, try first variant as default
	if modelPath == "" && entry.Variants != nil {
		for _, v := range entry.Variants {
			if v.Model != "" {
				modelPath = v.Model
				break
			}
		}
	}

	if modelPath == "" {
		return "", fmt.Errorf("accessory '%s' has no model path (variant: %s)", id, variant)
	}

	// Return path relative to base directory (e.g., "{assetsDir}/Cosmetics/...")
	// Use the actual assetsDir from registry (which may be custom)
	assetsDirName := filepath.Base(r.assetsDir)
	if assetsDirName == "" {
		assetsDirName = defaultAssetsDir
	}
	return filepath.Join(assetsDirName, modelPath), nil
}

// Lookup finds an accessory by field type and ID (no variant)
func (r *Registry) Lookup(fieldType, id string) (string, error) {
	return r.LookupWithVariant(fieldType, id, "")
}

// GetEntry returns the full entry for an accessory (for texture info, etc.)
func (r *Registry) GetEntry(fieldType, id string) (*AccessoryEntry, error) {
	registryName, ok := fieldToRegistry[fieldType]
	if !ok {
		return nil, fmt.Errorf("unknown field type: %s", fieldType)
	}

	registry, ok := r.entries[registryName]
	if !ok {
		return nil, fmt.Errorf("registry not loaded: %s", registryName)
	}

	entry, ok := registry[id]
	if !ok {
		return nil, fmt.Errorf("accessory '%s' not found in %s registry", id, registryName)
	}

	return &entry, nil
}

// ResolveTexture finds the texture for an accessory given color and variant
// Returns either a greyscale texture (for tinting) or a direct texture path
// Paths are returned relative to base directory (e.g., "assets/Cosmetics/...")
func (r *Registry) ResolveTexture(fieldType, id, color, variant string) (*ResolvedTexture, error) {
	entry, err := r.GetEntry(fieldType, id)
	if err != nil {
		return nil, err
	}

	result := &ResolvedTexture{
		GradientSet: entry.GradientSet,
	}

	// Get the actual assets directory name (may be custom)
	assetsDirName := filepath.Base(r.assetsDir)
	if assetsDirName == "" {
		assetsDirName = defaultAssetsDir
	}

	// First check variant-level textures if variant is specified
	if variant != "" && entry.Variants != nil {
		if variantEntry, ok := entry.Variants[variant]; ok {
			// Check for variant's Textures map (pre-colored)
			if color != "" && variantEntry.Textures != nil {
				if texEntry, ok := variantEntry.Textures[color]; ok {
					// Ensure path starts with assets directory name (JSON paths are relative to assets directory)
					texturePath := texEntry.Texture
					if !strings.HasPrefix(texturePath, assetsDirName+"/") && !strings.HasPrefix(texturePath, assetsDirName+string(filepath.Separator)) {
						texturePath = filepath.Join(assetsDirName, texturePath)
					}
					result.DirectTexture = texturePath
					result.BaseColor = texEntry.BaseColor
					return result, nil
				}
			}
			// Check for variant's greyscale texture
			if variantEntry.GreyscaleTexture != "" {
				// Ensure path starts with assets directory name (JSON paths are relative to assets directory)
				texturePath := variantEntry.GreyscaleTexture
				if !strings.HasPrefix(texturePath, assetsDirName+"/") && !strings.HasPrefix(texturePath, assetsDirName+string(filepath.Separator)) {
					texturePath = filepath.Join(assetsDirName, texturePath)
				}
				result.GreyscaleTexture = texturePath
				return result, nil
			}
		}
	}

	// Check top-level Textures map (pre-colored)
	if color != "" && entry.Textures != nil {
		if texEntry, ok := entry.Textures[color]; ok {
			// Ensure path starts with assets directory name (JSON paths are relative to assets directory)
			texturePath := texEntry.Texture

			if !strings.HasPrefix(texturePath, assetsDirName+"/") && !strings.HasPrefix(texturePath, assetsDirName+string(filepath.Separator)) {
				texturePath = filepath.Join(assetsDirName, texturePath)
			}

			result.DirectTexture = texturePath
			result.BaseColor = texEntry.BaseColor
			return result, nil
		}
	}

	// Fall back to top-level greyscale texture
	if entry.GreyscaleTexture != "" {
		// Ensure path starts with assets directory name (JSON paths are relative to assets directory)
		texturePath := entry.GreyscaleTexture
		if !strings.HasPrefix(texturePath, assetsDirName+"/") && !strings.HasPrefix(texturePath, assetsDirName+string(filepath.Separator)) {
			texturePath = filepath.Join(assetsDirName, texturePath)
		}
		result.GreyscaleTexture = texturePath
		return result, nil
	}

	return nil, fmt.Errorf("no texture found for %s (color: %s, variant: %s)", id, color, variant)
}
