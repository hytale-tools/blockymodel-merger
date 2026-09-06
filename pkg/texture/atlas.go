package texture

import (
	"fmt"
	"image"
	"image/draw"
	"math"
	"sort"
)

// AtlasEntry represents a texture placed in the atlas
type AtlasEntry struct {
	Name   string
	Image  image.Image
	X, Y   int // Position in atlas
	Width  int
	Height int
}

// Atlas represents a packed texture atlas
type Atlas struct {
	Image   *image.RGBA
	Entries map[string]*AtlasEntry
	Width   int
	Height  int
}

// PackAtlas packs multiple textures into a single atlas image
// Uses a simple shelf/row packing algorithm with tight bounds
func PackAtlas(textures []*TintedTexture, padding int) (*Atlas, error) {
	if len(textures) == 0 {
		return nil, fmt.Errorf("no textures to pack")
	}

	// Sort textures by height (tallest first) for better packing
	sorted := make([]*TintedTexture, len(textures))
	copy(sorted, textures)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Image.Bounds().Dy() > sorted[j].Image.Bounds().Dy()
	})

	// Calculate total area and max dimensions
	totalArea := 0
	maxTexWidth := 0
	maxTexHeight := 0
	for _, tex := range sorted {
		bounds := tex.Image.Bounds()
		w, h := bounds.Dx()+padding, bounds.Dy()+padding
		totalArea += w * h
		if w > maxTexWidth {
			maxTexWidth = w
		}
		if h > maxTexHeight {
			maxTexHeight = h
		}
	}

	// Single-pass: use sqrt(totalArea) as width, clamped to maxTexWidth
	width := int(math.Sqrt(float64(totalArea) * 1.2))
	if width < maxTexWidth {
		width = maxTexWidth
	}
	maxHeight := totalArea/width + (maxTexHeight+padding)*len(sorted)
	atlas := tryPackTight(sorted, width, maxHeight, padding)
	if atlas == nil {
		return nil, fmt.Errorf("failed to pack textures into atlas")
	}

	return atlas, nil
}

// tryPackTight packs textures and crops to actual content bounds
func tryPackTight(textures []*TintedTexture, width, height, padding int) *Atlas {
	entries := make(map[string]*AtlasEntry)

	// Shelf packing: place textures in rows
	shelfY := 0
	shelfHeight := 0
	currentX := 0
	maxX := 0
	maxY := 0

	for _, tex := range textures {
		bounds := tex.Image.Bounds()
		texW, texH := bounds.Dx(), bounds.Dy()

		// Check if texture fits on current shelf
		if currentX+texW > width {
			// Move to next shelf
			shelfY += shelfHeight + padding
			shelfHeight = 0
			currentX = 0
		}

		// Check if we've run out of vertical space
		if shelfY+texH > height {
			return nil // Doesn't fit
		}

		// Place texture
		entry := &AtlasEntry{
			Name:   tex.Name,
			Image:  tex.Image,
			X:      currentX,
			Y:      shelfY,
			Width:  texW,
			Height: texH,
		}
		entries[tex.Name] = entry

		// Track actual bounds
		if currentX+texW > maxX {
			maxX = currentX + texW
		}
		if shelfY+texH > maxY {
			maxY = shelfY + texH
		}

		// Update shelf tracking
		currentX += texW + padding
		if texH > shelfHeight {
			shelfHeight = texH
		}
	}

	// Create atlas with tight bounds (no wasted space)
	// Pad the canvas so the exported texture satisfies Hytale's 32px
	// alignment rule. Entries keep their pixel positions, so padding on the
	// bottom/right edge never shifts any offsets.
	maxX, maxY = alignSize(maxX), alignSize(maxY)
	atlas := &Atlas{
		Image:   image.NewRGBA(image.Rect(0, 0, maxX, maxY)),
		Entries: entries,
		Width:   maxX,
		Height:  maxY,
	}

	// Draw all textures
	for _, entry := range entries {
		bounds := entry.Image.Bounds()
		destRect := image.Rect(entry.X, entry.Y, entry.X+entry.Width, entry.Y+entry.Height)
		draw.Draw(atlas.Image, destRect, entry.Image, bounds.Min, draw.Over)
	}

	return atlas
}

// TextureAlignment is the pixel granularity Hytale requires for model
// textures: width and height must be multiples of 32 and at least 32.
const TextureAlignment = 32

// alignSize rounds n up to the next multiple of TextureAlignment (min 32).
func alignSize(n int) int {
	if n < TextureAlignment {
		return TextureAlignment
	}
	return (n + TextureAlignment - 1) / TextureAlignment * TextureAlignment
}

func nextPowerOf2(n int) int {
	if n <= 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}

// GetUVCoords returns normalized UV coordinates (0-1) for a texture in the atlas
func (a *Atlas) GetUVCoords(name string) (u0, v0, u1, v1 float64, ok bool) {
	entry, exists := a.Entries[name]
	if !exists {
		return 0, 0, 0, 0, false
	}

	// Convert pixel coords to normalized UV (0-1)
	u0 = float64(entry.X) / float64(a.Width)
	v0 = float64(entry.Y) / float64(a.Height)
	u1 = float64(entry.X+entry.Width) / float64(a.Width)
	v1 = float64(entry.Y+entry.Height) / float64(a.Height)

	return u0, v0, u1, v1, true
}

// GetPixelCoords returns pixel coordinates for a texture in the atlas
func (a *Atlas) GetPixelCoords(name string) (x, y, width, height int, ok bool) {
	entry, exists := a.Entries[name]
	if !exists {
		return 0, 0, 0, 0, false
	}
	return entry.X, entry.Y, entry.Width, entry.Height, true
}

// PackAtlasSimple packs textures with base at (0,0) and others stacked below
// No optimization - just simple vertical stacking for debugging
func PackAtlasSimple(textures []*TintedTexture, padding int) (*Atlas, error) {
	if len(textures) == 0 {
		return nil, fmt.Errorf("no textures to pack")
	}

	// First texture is the base, placed at (0,0)
	baseTex := textures[0]
	baseBounds := baseTex.Image.Bounds()
	baseW := baseBounds.Dx()
	baseH := baseBounds.Dy()

	entries := make(map[string]*AtlasEntry)
	entries[baseTex.Name] = &AtlasEntry{
		Name:   baseTex.Name,
		Image:  baseTex.Image,
		X:      0,
		Y:      0,
		Width:  baseW,
		Height: baseH,
	}

	// Stack remaining textures below the base, wrapping at base width
	currentX := 0
	currentY := baseH + padding
	rowHeight := 0
	maxWidth := baseW

	for _, tex := range textures[1:] {
		bounds := tex.Image.Bounds()
		texW := bounds.Dx()
		texH := bounds.Dy()

		// Wrap to next row if needed
		if currentX+texW > baseW && currentX > 0 {
			currentY += rowHeight + padding
			currentX = 0
			rowHeight = 0
		}

		entries[tex.Name] = &AtlasEntry{
			Name:   tex.Name,
			Image:  tex.Image,
			X:      currentX,
			Y:      currentY,
			Width:  texW,
			Height: texH,
		}

		if currentX+texW > maxWidth {
			maxWidth = currentX + texW
		}
		if texH > rowHeight {
			rowHeight = texH
		}

		currentX += texW + padding
	}

	// Final height
	totalHeight := currentY + rowHeight

	// Create atlas image
	// Pad the canvas so the exported texture satisfies Hytale's 32px
	// alignment rule. Entries keep their pixel positions, so padding on the
	// bottom/right edge never shifts any offsets.
	maxWidth, totalHeight = alignSize(maxWidth), alignSize(totalHeight)
	atlas := &Atlas{
		Image:   image.NewRGBA(image.Rect(0, 0, maxWidth, totalHeight)),
		Entries: entries,
		Width:   maxWidth,
		Height:  totalHeight,
	}

	// Draw all textures
	for _, entry := range entries {
		bounds := entry.Image.Bounds()
		destRect := image.Rect(entry.X, entry.Y, entry.X+entry.Width, entry.Y+entry.Height)
		draw.Draw(atlas.Image, destRect, entry.Image, bounds.Min, draw.Over)
	}

	return atlas, nil
}

// PackAtlasWithBase packs textures with the first texture (base) fixed at (0,0)
// Other textures are packed to the right of or below the base
func PackAtlasWithBase(textures []*TintedTexture, padding int) (*Atlas, error) {
	if len(textures) == 0 {
		return nil, fmt.Errorf("no textures to pack")
	}

	// First texture is the base, placed at (0,0)
	baseTex := textures[0]
	baseBounds := baseTex.Image.Bounds()
	baseW := baseBounds.Dx()

	// Sort remaining textures by height (tallest first) for better packing
	remaining := make([]*TintedTexture, len(textures)-1)
	copy(remaining, textures[1:])
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].Image.Bounds().Dy() > remaining[j].Image.Bounds().Dy()
	})

	// Single-pass with base width
	atlas := tryPackWithBase(baseTex, remaining, baseW, padding)
	if atlas == nil {
		return nil, fmt.Errorf("failed to pack textures into atlas")
	}

	return atlas, nil
}

func tryPackWithBase(baseTex *TintedTexture, remaining []*TintedTexture, targetWidth, padding int) *Atlas {
	baseBounds := baseTex.Image.Bounds()
	baseW := baseBounds.Dx()
	baseH := baseBounds.Dy()

	entries := make(map[string]*AtlasEntry)
	entries[baseTex.Name] = &AtlasEntry{
		Name:   baseTex.Name,
		Image:  baseTex.Image,
		X:      0,
		Y:      0,
		Width:  baseW,
		Height: baseH,
	}

	// Pack remaining textures below the base texture
	shelfY := baseH + padding
	shelfHeight := 0
	currentX := 0
	maxX := baseW
	maxY := baseH

	for _, tex := range remaining {
		bounds := tex.Image.Bounds()
		texW, texH := bounds.Dx(), bounds.Dy()

		// Check if texture fits on current shelf
		if currentX+texW > targetWidth {
			// Move to next shelf
			shelfY += shelfHeight + padding
			shelfHeight = 0
			currentX = 0
		}

		// Place texture
		entry := &AtlasEntry{
			Name:   tex.Name,
			Image:  tex.Image,
			X:      currentX,
			Y:      shelfY,
			Width:  texW,
			Height: texH,
		}
		entries[tex.Name] = entry

		// Track actual bounds
		if currentX+texW > maxX {
			maxX = currentX + texW
		}
		if shelfY+texH > maxY {
			maxY = shelfY + texH
		}

		// Update shelf tracking
		currentX += texW + padding
		if texH > shelfHeight {
			shelfHeight = texH
		}
	}

	// Create atlas with tight bounds
	// Pad the canvas so the exported texture satisfies Hytale's 32px
	// alignment rule. Entries keep their pixel positions, so padding on the
	// bottom/right edge never shifts any offsets.
	maxX, maxY = alignSize(maxX), alignSize(maxY)
	atlas := &Atlas{
		Image:   image.NewRGBA(image.Rect(0, 0, maxX, maxY)),
		Entries: entries,
		Width:   maxX,
		Height:  maxY,
	}

	// Draw all textures
	for _, entry := range entries {
		bounds := entry.Image.Bounds()
		destRect := image.Rect(entry.X, entry.Y, entry.X+entry.Width, entry.Y+entry.Height)
		draw.Draw(atlas.Image, destRect, entry.Image, bounds.Min, draw.Over)
	}

	return atlas
}
