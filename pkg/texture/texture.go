package texture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hytale-tools/blockymodel-merger/pkg/util"
)

const (
	defaultAssetsDir = "assets"
	defaultDataDir   = "data"
)

// GradientEntry represents a single gradient option
type GradientEntry struct {
	BaseColor []string `json:"BaseColor"`
	Texture   string   `json:"Texture"`
}

// GradientSet represents a set of gradients (e.g., Hair, Skin)
type GradientSet struct {
	ID        string                   `json:"Id"`
	Gradients map[string]GradientEntry `json:"Gradients"`
	// colorOrder preserves the JSON declaration order of Gradients; the first
	// entry is the set's default color.
	colorOrder []string
}

// UnmarshalJSON decodes a gradient set while capturing the declaration order
// of its colors, which a plain map would lose.
func (s *GradientSet) UnmarshalJSON(data []byte) error {
	type alias GradientSet
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*s = GradientSet(a)

	var raw struct {
		Gradients json.RawMessage `json:"Gradients"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || len(raw.Gradients) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw.Gradients))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil
		}
		if key, ok := tok.(string); ok {
			s.colorOrder = append(s.colorOrder, key)
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil
		}
	}
	return nil
}

// GradientSets holds all loaded gradient sets
type GradientSets struct {
	sets      map[string]GradientSet
	assetsDir string // Base path for assets directory
}

// LoadGradientSets loads gradient sets from data/GradientSets.json
// If basePath is empty, uses default "data" and "assets" directories
// Optional dataDirName and assetsDirName allow custom directory names (defaults: "data", "assets")
func LoadGradientSets(basePath ...string) (*GradientSets, error) {
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
	
	path := filepath.Join(dataDir, "GradientSets.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load gradient sets: %w", err)
	}

	var sets []GradientSet
	if err := json.Unmarshal(data, &sets); err != nil {
		return nil, fmt.Errorf("failed to parse gradient sets: %w", err)
	}

	gs := &GradientSets{
		sets:      make(map[string]GradientSet),
		assetsDir: assetsDir,
	}

	for _, set := range sets {
		gs.sets[set.ID] = set
	}

	return gs, nil
}

// GetGradient returns a gradient entry for a set and color name
// HasGradient reports whether the named color exists in a gradient set. A nil
// receiver accepts every color, since without gradient data nothing can be
// refuted.
func (gs *GradientSets) HasGradient(setName, colorName string) bool {
	if gs == nil {
		return true
	}
	set, ok := gs.sets[setName]
	if !ok {
		return false
	}
	_, ok = set.Gradients[colorName]
	return ok
}

// DefaultColor returns the first color declared in a gradient set, or "" if
// the set is unknown or empty.
func (gs *GradientSets) DefaultColor(setName string) string {
	if gs == nil {
		return ""
	}
	set, ok := gs.sets[setName]
	if !ok || len(set.colorOrder) == 0 {
		return ""
	}
	return set.colorOrder[0]
}

func (gs *GradientSets) GetGradient(setName, colorName string) (*GradientEntry, error) {
	set, ok := gs.sets[setName]
	if !ok {
		return nil, fmt.Errorf("gradient set not found: %s", setName)
	}

	gradient, ok := set.Gradients[colorName]
	if !ok {
		return nil, fmt.Errorf("gradient color '%s' not found in set '%s'", colorName, setName)
	}

	return &gradient, nil
}

// LoadImage loads a PNG image from the assets directory
// If basePath is provided, uses it; otherwise uses default "assets" directory
// relativePath may already include "assets/" prefix - if basePath is provided, it will be joined correctly
func LoadImage(relativePath string, basePath ...string) (image.Image, error) {
	var path string
	
	// Check if path already starts with assets/ (handle both / and filepath.Separator)
	hasAssetsPrefix := strings.HasPrefix(relativePath, "assets/") || 
		strings.HasPrefix(relativePath, "assets"+string(filepath.Separator))
	
	if filepath.IsAbs(relativePath) {
		// Already a full path (e.g. an asset pack resolved to an absolute
		// root); joining it onto a base would strip the leading separator.
		path = relativePath
	} else if len(basePath) > 0 && basePath[0] != "" {
		// Base path provided - join directly
		path = filepath.Join(basePath[0], relativePath)
	} else {
		// No base path - check if relativePath already has assets/ prefix
		if hasAssetsPrefix {
			// Already has assets/ prefix, use as-is
			path = relativePath
		} else {
			// No assets/ prefix, add it
			path = filepath.Join(defaultAssetsDir, relativePath)
		}
	}
	
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open image %s: %w", path, err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PNG %s: %w", path, err)
	}

	return img, nil
}

// SaveImage saves an image as PNG
func SaveImage(img image.Image, path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer f.Close()

	enc := &png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(f, img); err != nil {
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	return nil
}

// EncodePNG encodes an image to PNG bytes with no compression (optimized for GLB embedding)
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := &png.Encoder{CompressionLevel: png.NoCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("failed to encode PNG: %w", err)
	}
	return buf.Bytes(), nil
}

// ParseHexColor parses a hex color string like "#090a1f" into RGB values
func ParseHexColor(hex string) (r, g, b uint8, err error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0, fmt.Errorf("invalid hex color: %s", hex)
	}

	rVal, err := strconv.ParseUint(hex[0:2], 16, 8)
	if err != nil {
		return 0, 0, 0, err
	}
	gVal, err := strconv.ParseUint(hex[2:4], 16, 8)
	if err != nil {
		return 0, 0, 0, err
	}
	bVal, err := strconv.ParseUint(hex[4:6], 16, 8)
	if err != nil {
		return 0, 0, 0, err
	}

	return uint8(rVal), uint8(gVal), uint8(bVal), nil
}

// ApplyGradientTint applies a gradient tint to a greyscale image
// If gradientPath is available, uses gradient lookup
// Otherwise falls back to baseColor tinting
func ApplyGradientTint(greyscale image.Image, gradientPath string, baseColor string, baseAssetsPath ...string) (image.Image, error) {
	return ApplyGradientTintWithSet(greyscale, gradientPath, baseColor, "", baseAssetsPath...)
}

// ApplyGradientTintWithSet applies gradient tint using the blockymodel algorithm:
// - Greyscale pixels (R==G==B): apply gradient lookup
// - Colored pixels (R≠G or G≠B): keep original color unchanged
// baseAssetsPath is optional - if provided, should be the full path to the assets directory
func ApplyGradientTintWithSet(greyscale image.Image, gradientPath string, baseColor string, gradientSet string, baseAssetsPath ...string) (image.Image, error) {
	bounds := greyscale.Bounds()
	result := image.NewRGBA(bounds)

	// Try to load gradient texture
	var gradient image.Image
	if gradientPath != "" {
		var err error
		gradient, err = LoadImage(gradientPath, baseAssetsPath...)
		if err != nil {
			// Fall back to base color
			util.Logger.Debug("Gradient file not found, using base color",
				"path", gradientPath,
				"error", err)
			gradient = nil
		} else {
			util.Logger.Debug("Loaded gradient",
				"path", gradientPath,
				"width", gradient.Bounds().Dx(),
				"height", gradient.Bounds().Dy())
		}
	}

	// Parse base color as fallback
	var baseR, baseG, baseB uint8 = 128, 128, 128
	if baseColor != "" {
		var err error
		baseR, baseG, baseB, err = ParseHexColor(baseColor)
		if err != nil {
			util.Logger.Warn("Invalid base color", "color", baseColor, "error", err)
		}
	}

	// Try direct pixel access for performance
	srcRGBA, srcIsRGBA := greyscale.(*image.RGBA)
	srcNRGBA, srcIsNRGBA := greyscale.(*image.NRGBA)

	// Pre-compute gradient lookup table if gradient is available
	var gradLUT [256][3]uint8
	if gradient != nil {
		gradBounds := gradient.Bounds()
		gradW := gradBounds.Max.X - 1
		if gradRGBA, ok := gradient.(*image.RGBA); ok {
			for i := 0; i < 256; i++ {
				gradX := i * gradW / 255
				gi := gradRGBA.PixOffset(gradX+gradBounds.Min.X, gradBounds.Min.Y)
				gradLUT[i] = [3]uint8{gradRGBA.Pix[gi], gradRGBA.Pix[gi+1], gradRGBA.Pix[gi+2]}
			}
		} else if gradNRGBA, ok := gradient.(*image.NRGBA); ok {
			for i := 0; i < 256; i++ {
				gradX := i * gradW / 255
				gi := gradNRGBA.PixOffset(gradX+gradBounds.Min.X, gradBounds.Min.Y)
				gradLUT[i] = [3]uint8{gradNRGBA.Pix[gi], gradNRGBA.Pix[gi+1], gradNRGBA.Pix[gi+2]}
			}
		} else {
			for i := 0; i < 256; i++ {
				gradX := i * gradW / 255
				c := gradient.At(gradX+gradBounds.Min.X, gradBounds.Min.Y)
				rr, gg, bb, _ := c.RGBA()
				gradLUT[i] = [3]uint8{uint8(rr >> 8), uint8(gg >> 8), uint8(bb >> 8)}
			}
		}
	}

	stride := result.Stride
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		// Read source pixels directly from Pix slice
		var srcR, srcG, srcB, srcA uint8
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if srcIsRGBA {
				si := srcRGBA.PixOffset(x, y)
				srcR, srcG, srcB, srcA = srcRGBA.Pix[si], srcRGBA.Pix[si+1], srcRGBA.Pix[si+2], srcRGBA.Pix[si+3]
			} else if srcIsNRGBA {
				si := srcNRGBA.PixOffset(x, y)
				srcR, srcG, srcB, srcA = srcNRGBA.Pix[si], srcNRGBA.Pix[si+1], srcNRGBA.Pix[si+2], srcNRGBA.Pix[si+3]
			} else {
				c := greyscale.At(x, y)
				rgba := color.RGBAModel.Convert(c).(color.RGBA)
				srcR, srcG, srcB, srcA = rgba.R, rgba.G, rgba.B, rgba.A
			}

			var r, g, b uint8
			isGrey := srcR == srcG && srcG == srcB

			if gradient != nil && isGrey {
				lut := gradLUT[srcR]
				r, g, b = lut[0], lut[1], lut[2]
			} else if !isGrey {
				r, g, b = srcR, srcG, srcB
			} else {
				r = uint8(uint16(srcR) * uint16(baseR) / 255)
				g = uint8(uint16(srcR) * uint16(baseG) / 255)
				b = uint8(uint16(srcR) * uint16(baseB) / 255)
			}

			// Threshold alpha
			a := uint8(0)
			if srcA >= 128 {
				a = 255
			}

			// Write directly to result Pix slice
			di := (y-bounds.Min.Y)*stride + (x-bounds.Min.X)*4
			result.Pix[di] = r
			result.Pix[di+1] = g
			result.Pix[di+2] = b
			result.Pix[di+3] = a
		}
	}

	return result, nil
}

// TintedTexture represents a processed texture with tinting applied
type TintedTexture struct {
	Name        string      // Accessory name
	Image       image.Image // The tinted image
	OriginalPath string     // Original greyscale path
}

// ProcessAccessoryTexture loads and tints an accessory's texture
func ProcessAccessoryTexture(
	name string,
	greyscalePath string,
	gradientSet string,
	colorName string,
	gradientSets *GradientSets,
) (*TintedTexture, error) {
	// Get base path and assets directory name from gradientSets if available
	var basePath string
	var assetsDirName string
	if gradientSets != nil {
		// Extract base path from assetsDir (remove assets directory name suffix)
		assetsDir := gradientSets.assetsDir
		assetsDirName = filepath.Base(assetsDir)
		if assetsDirName == "" {
			assetsDirName = defaultAssetsDir
		}
		basePath = filepath.Dir(assetsDir)
	} else {
		assetsDirName = defaultAssetsDir
	}
	
	// Load greyscale texture
	greyscale, err := LoadImage(greyscalePath, basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load greyscale texture for %s: %w", name, err)
	}

	// Get gradient info
	var gradientPath, baseColor string
	if gradientSet != "" && colorName != "" && gradientSets != nil {
		gradient, err := gradientSets.GetGradient(gradientSet, colorName)
		if err != nil {
			util.Logger.Warn("Failed to get gradient, using default",
				"gradientSet", gradientSet,
				"color", colorName,
				"error", err)
		} else {
			// Prepend assets/ to gradient path (gradient paths from JSON are relative to assets)
			gradientPath = filepath.Join(assetsDirName, gradient.Texture)
			if len(gradient.BaseColor) > 0 {
				baseColor = gradient.BaseColor[0]
			}
			util.Logger.Debug("Using gradient",
				"path", gradientPath,
				"gradientSet", gradientSet,
				"color", colorName)
		}
	} else {
		util.Logger.Debug("No gradient specified",
			"gradientSet", gradientSet,
			"color", colorName)
	}

	// Apply tinting (pass basePath - LoadImage will handle the assets/ prefix in gradientPath)
	tinted, err := ApplyGradientTintWithSet(greyscale, gradientPath, baseColor, gradientSet, basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to apply tint to %s: %w", name, err)
	}

	return &TintedTexture{
		Name:         name,
		Image:        tinted,
		OriginalPath: greyscalePath,
	}, nil
}
