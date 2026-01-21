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
)

const (
	assetsDir = "assets"
	dataDir   = "data"
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
}

// GradientSets holds all loaded gradient sets
type GradientSets struct {
	sets map[string]GradientSet
}

// LoadGradientSets loads gradient sets from data/GradientSets.json
func LoadGradientSets() (*GradientSets, error) {
	path := filepath.Join(dataDir, "GradientSets.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load gradient sets: %w", err)
	}

	var sets []GradientSet
	if err := json.Unmarshal(data, &sets); err != nil {
		return nil, fmt.Errorf("failed to parse gradient sets: %w", err)
	}

	gs := &GradientSets{sets: make(map[string]GradientSet)}
	for _, set := range sets {
		gs.sets[set.ID] = set
	}

	return gs, nil
}

// GetGradient returns a gradient entry for a set and color name
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
func LoadImage(relativePath string) (image.Image, error) {
	path := filepath.Join(assetsDir, relativePath)
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

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	return nil
}

// EncodePNG encodes an image to PNG bytes
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
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

// isGreyscaleImage checks if an image should use direct gradient replacement
// Returns true for textures without pre-baked colors AND without pure white/black that needs preserving
// Returns false for:
//   - Textures with significant color (like beige gloves)
//   - Textures with high contrast (pure white/black) that should be preserved
func isGreyscaleImage(img image.Image) bool {
	bounds := img.Bounds()
	const colorTolerance = 25 // Detect STRONG color differences

	coloredPixels := 0
	pureWhitePixels := 0
	pureBlackPixels := 0
	totalPixels := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.At(x, y)
			r, g, b, a := c.RGBA()
			if a == 0 {
				continue // Skip fully transparent pixels
			}
			r8, g8, b8 := r>>8, g>>8, b>>8
			totalPixels++

			// Check for very light (> 200) or very dark (< 50)
			lum := (r8 + g8 + b8) / 3
			if lum > 200 {
				pureWhitePixels++
			} else if lum < 50 {
				pureBlackPixels++
			}

			// Check if R, G, B are approximately equal
			maxDiff := max(max(absDiff(r8, g8), absDiff(g8, b8)), absDiff(r8, b8))
			if maxDiff > colorTolerance {
				coloredPixels++
			}
		}
	}

	if totalPixels == 0 {
		return true
	}

	// If texture has significant pre-baked colors, use soft light
	colorRatio := float64(coloredPixels) / float64(totalPixels)
	if colorRatio >= 0.40 {
		return false
	}

	// If texture has BOTH pure white AND pure black (high contrast pattern like stripes)
	// use soft light to preserve the white/black
	whiteRatio := float64(pureWhitePixels) / float64(totalPixels)
	blackRatio := float64(pureBlackPixels) / float64(totalPixels)
	if whiteRatio > 0.05 && blackRatio > 0.05 {
		return false // High contrast pattern - preserve white/black
	}

	return true
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

func max(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

// ApplyGradientTint applies a gradient tint to a greyscale image
// If gradientPath is available, uses gradient lookup
// Otherwise falls back to baseColor tinting
func ApplyGradientTint(greyscale image.Image, gradientPath string, baseColor string) (image.Image, error) {
	return ApplyGradientTintWithSet(greyscale, gradientPath, baseColor, "")
}

// ApplyGradientTintWithSet applies gradient tint using the blockymodel algorithm:
// - Greyscale pixels (R==G==B): apply gradient lookup
// - Colored pixels (R≠G or G≠B): keep original color unchanged
func ApplyGradientTintWithSet(greyscale image.Image, gradientPath string, baseColor string, gradientSet string) (image.Image, error) {
	bounds := greyscale.Bounds()
	result := image.NewRGBA(bounds)

	// Try to load gradient texture
	var gradient image.Image
	if gradientPath != "" {
		var err error
		gradient, err = LoadImage(gradientPath)
		if err != nil {
			// Fall back to base color
			fmt.Printf("    Note: Gradient file not found (%s), using base color\n", gradientPath)
			gradient = nil
		} else {
			fmt.Printf("    → Loaded gradient: %dx%d\n", gradient.Bounds().Dx(), gradient.Bounds().Dy())
		}
	}

	// Parse base color as fallback
	var baseR, baseG, baseB uint8 = 128, 128, 128
	if baseColor != "" {
		var err error
		baseR, baseG, baseB, err = ParseHexColor(baseColor)
		if err != nil {
			fmt.Printf("  Warning: Invalid base color %s\n", baseColor)
		}
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			origColor := greyscale.At(x, y)
			origRGBA := color.RGBAModel.Convert(origColor).(color.RGBA)

			var r, g, b uint8

			// Blockymodel gradient tinting algorithm:
			// Greyscale pixels (R==G==B): apply gradient lookup
			// Colored pixels (R≠G or G≠B): keep original pixel unchanged
			isGreyscale := origRGBA.R == origRGBA.G && origRGBA.G == origRGBA.B

			if gradient != nil && isGreyscale {
				// Greyscale pixel: use gradient lookup
				// The red channel value (0-255) maps to X position in gradient
				gradBounds := gradient.Bounds()
				gradX := int(float64(origRGBA.R) / 255.0 * float64(gradBounds.Max.X-1))
				if gradX >= gradBounds.Max.X {
					gradX = gradBounds.Max.X - 1
				}
				if gradX < 0 {
					gradX = 0
				}
				gradColor := gradient.At(gradX, gradBounds.Min.Y)
				rr, gg, bb, _ := gradColor.RGBA()
				r, g, b = uint8(rr>>8), uint8(gg>>8), uint8(bb>>8)
			} else if !isGreyscale {
				// Colored pixel: keep original color unchanged
				r, g, b = origRGBA.R, origRGBA.G, origRGBA.B
			} else {
				// No gradient available, use base color tint
				grey := origRGBA.R // For greyscale, R=G=B
				r = uint8(float64(grey) * float64(baseR) / 255.0)
				g = uint8(float64(grey) * float64(baseG) / 255.0)
				b = uint8(float64(grey) * float64(baseB) / 255.0)
			}

			// Threshold alpha to avoid semi-transparent edge artifacts
		// Alpha >= 128 becomes fully opaque, alpha < 128 becomes fully transparent
		a := origRGBA.A
		if a >= 128 {
			a = 255
		} else {
			a = 0
		}
		result.Set(x, y, color.RGBA{R: r, G: g, B: b, A: a})
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
	// Load greyscale texture
	greyscale, err := LoadImage(greyscalePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load greyscale texture for %s: %w", name, err)
	}

	// Get gradient info
	var gradientPath, baseColor string
	if gradientSet != "" && colorName != "" && gradientSets != nil {
		gradient, err := gradientSets.GetGradient(gradientSet, colorName)
		if err != nil {
			fmt.Printf("    Warning: %v, using default\n", err)
		} else {
			gradientPath = gradient.Texture
			if len(gradient.BaseColor) > 0 {
				baseColor = gradient.BaseColor[0]
			}
			fmt.Printf("    → Using gradient: %s\n", gradientPath)
		}
	} else {
		fmt.Printf("    → No gradient (set=%q, color=%q)\n", gradientSet, colorName)
	}

	// Apply tinting (pass gradient set name to determine blend mode)
	tinted, err := ApplyGradientTintWithSet(greyscale, gradientPath, baseColor, gradientSet)
	if err != nil {
		return nil, fmt.Errorf("failed to apply tint to %s: %w", name, err)
	}

	return &TintedTexture{
		Name:         name,
		Image:        tinted,
		OriginalPath: greyscalePath,
	}, nil
}
