package export

import (
	"bytes"
	"image"
	"image/png"
	"strings"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
	"math"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
	"github.com/hytale-tools/blockymodel-merger/pkg/util"
)

// MeshNodeSuffix distinguishes the geometry node split out of a blockymodel
// node that is both a bone and a mesh, so every bone name in the exported
// scene resolves to exactly one node.
const MeshNodeSuffix = ".mesh"

// GLBExporter exports BlockyModel to GLB format matching Blockbench's output exactly
type GLBExporter struct {
	doc         *gltf.Document
	materials   map[string]int
	dsMaterials map[uint32]uint32 // base material index -> double-sided clone index
	atlasWidth  float64
	atlasHeight float64
	atlasImage  image.Image // decoded atlas, for per-face alpha inspection
}

// NewGLBExporter creates a new GLB exporter
func NewGLBExporter() *GLBExporter {
	doc := gltf.NewDocument()
	return &GLBExporter{
		doc:         doc,
		materials:   make(map[string]int),
		dsMaterials: make(map[uint32]uint32),
	}
}

// SetAtlasSize sets the atlas dimensions for UV calculations
func (e *GLBExporter) SetAtlasSize(width, height float64) {
	e.atlasWidth = width
	e.atlasHeight = height
}

// AddTexture adds a texture from PNG data
func (e *GLBExporter) AddTexture(imageData []byte) uint32 {
	// Keep a decoded copy so material sidedness can inspect per-face alpha.
	if img, err := png.Decode(bytes.NewReader(imageData)); err == nil {
		e.atlasImage = img
	} else {
		util.Logger.Warn("Could not decode atlas for sidedness detection; cutout shapes may render single-sided", "error", err)
	}
	imgIdx := uint32(len(e.doc.Images))
	e.doc.Images = append(e.doc.Images, &gltf.Image{
		MimeType:   "image/png",
		BufferView: gltf.Index(modeler.WriteBufferView(e.doc, gltf.TargetNone, imageData)),
	})

	// Create sampler with nearest filtering for pixel art
	samplerIdx := len(e.doc.Samplers)
	e.doc.Samplers = append(e.doc.Samplers, &gltf.Sampler{
		MagFilter: gltf.MagNearest,
		MinFilter: gltf.MinNearest,
		WrapS:     gltf.WrapClampToEdge,
		WrapT:     gltf.WrapClampToEdge,
	})

	texIdx := uint32(len(e.doc.Textures))
	e.doc.Textures = append(e.doc.Textures, &gltf.Texture{
		Sampler: gltf.Index(samplerIdx),
		Source:  gltf.Index(int(imgIdx)),
	})

	return texIdx
}

// AddMaterial adds a material using the given texture
func (e *GLBExporter) AddMaterial(name string, textureIdx uint32) uint32 {
	if idx, ok := e.materials[name]; ok {
		return uint32(idx)
	}

	materialIdx := len(e.doc.Materials)
	e.doc.Materials = append(e.doc.Materials, &gltf.Material{
		Name: name,
		PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
			BaseColorTexture: &gltf.TextureInfo{
				Index: int(textureIdx),
			},
			MetallicFactor:  gltf.Float(0),
			RoughnessFactor: gltf.Float(1),
		},
		AlphaMode:   gltf.AlphaMask,
		AlphaCutoff: gltf.Float(0.05),
		Extensions: gltf.Extensions{
			"KHR_materials_unlit": map[string]interface{}{},
		},
	})

	// Register the extension as used
	if e.doc.ExtensionsUsed == nil {
		e.doc.ExtensionsUsed = []string{}
	}
	hasExt := false
	for _, ext := range e.doc.ExtensionsUsed {
		if ext == "KHR_materials_unlit" {
			hasExt = true
			break
		}
	}
	if !hasExt {
		e.doc.ExtensionsUsed = append(e.doc.ExtensionsUsed, "KHR_materials_unlit")
	}

	e.materials[name] = materialIdx
	return uint32(materialIdx)
}

// materialFor returns the material to use for a shape: the base material for
// fully closed opaque boxes, or a double-sided clone for shapes whose
// backfaces can legitimately show. Blockbench renders everything double-sided;
// culling closed opaque boxes is a safe deviation that keeps hairline gaps
// between adjacent pieces from exposing bright interior faces in multisampled
// viewers.
func (e *GLBExporter) materialFor(baseIdx uint32, shape *blockymodel.Shape) uint32 {
	if shape == nil || !e.shapeNeedsDoubleSided(shape) {
		return baseIdx
	}
	if idx, ok := e.dsMaterials[baseIdx]; ok {
		return idx
	}
	return e.doubleSidedClone(baseIdx)
}

// shapeNeedsDoubleSided reports whether a shape's backfaces must render:
// explicitly flagged shapes, quads (flat, visible from both sides), open
// boxes (fewer than six textured faces), and boxes with transparent texels in
// a face (cutouts expose the interior).
func (e *GLBExporter) shapeNeedsDoubleSided(shape *blockymodel.Shape) bool {
	if shape.DoubleSided != nil && *shape.DoubleSided {
		return true
	}
	if shape.Type != "box" {
		return true
	}
	if len(shape.TextureLayout) < 6 {
		return true
	}
	return e.boxHasTransparentTexels(shape)
}

// transparentAlphaCutoff mirrors the material's alphaCutoff of 0.05 (13/255):
// texels below it are discarded by the alpha mask, forming cutout holes.
const transparentAlphaCutoff = 13

// boxHasTransparentTexels scans each textured face's atlas rect for alpha
// below the mask cutoff.
func (e *GLBExporter) boxHasTransparentTexels(shape *blockymodel.Shape) bool {
	if e.atlasImage == nil {
		return false
	}
	sizeX, sizeY, sizeZ := 1.0, 1.0, 1.0
	if shape.Settings != nil {
		if size, ok := shape.Settings["size"].(map[string]interface{}); ok {
			if x, ok := size["x"].(float64); ok {
				sizeX = x
			}
			if y, ok := size["y"].(float64); ok {
				sizeY = y
			}
			if z, ok := size["z"].(float64); ok {
				sizeZ = z
			}
		}
	}
	dims := map[string][2]float64{
		"right": {sizeZ, sizeY}, "left": {sizeZ, sizeY},
		"top": {sizeX, sizeZ}, "bottom": {sizeX, sizeZ},
		"front": {sizeX, sizeY}, "back": {sizeX, sizeY},
	}
	bounds := e.atlasImage.Bounds()
	for name, layout := range shape.TextureLayout {
		d, ok := dims[name]
		if !ok {
			continue
		}
		w, h := d[0], d[1]
		if angle := ((int(layout.Angle)%180)+180)%180; angle == 90 {
			w, h = h, w
		}
		// Mirrored faces extend backwards from the offset (texU = ox - w).
		x0, y0 := int(layout.Offset.X), int(layout.Offset.Y)
		x1, y1 := x0+int(w), y0+int(h)
		if layout.Mirror.X {
			x0, x1 = x0-int(w), x0
		}
		if layout.Mirror.Y {
			y0, y1 = y0-int(h), y0
		}
		x0, y0 = max(x0, bounds.Min.X), max(y0, bounds.Min.Y)
		x1, y1 = min(x1, bounds.Max.X), min(y1, bounds.Max.Y)
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				_, _, _, a := e.atlasImage.At(x, y).RGBA()
				if a>>8 < transparentAlphaCutoff {
					return true
				}
			}
		}
	}
	return false
}

func (e *GLBExporter) doubleSidedClone(baseIdx uint32) uint32 {
	base := e.doc.Materials[baseIdx]
	clone := *base
	clone.Name = base.Name + "-doublesided"
	clone.DoubleSided = true
	materialIdx := uint32(len(e.doc.Materials))
	e.doc.Materials = append(e.doc.Materials, &clone)
	e.dsMaterials[baseIdx] = materialIdx
	return materialIdx
}

// ExportModel exports a BlockyModel to GLB with hierarchical nodes
// Matches Blockbench's export: each node becomes a bone + mesh child pair
func (e *GLBExporter) ExportModel(model *blockymodel.BlockyModel, materialIdx uint32) error {
	var rootChildren []int
	for i := range model.Nodes {
		// Root nodes have no parent offset
		nodeIdx := e.processNodeBlockbench(&model.Nodes[i], materialIdx, [3]float64{0, 0, 0})
		if nodeIdx >= 0 {
			rootChildren = append(rootChildren, nodeIdx)
		}
	}

	if len(rootChildren) > 0 {
		e.doc.Scenes[0].Nodes = rootChildren
	}

	return nil
}

// processNodeBlockbench creates GLB nodes matching Blockbench's export pattern:
// Each blockymodel node becomes TWO GLB nodes:
// 1. A "bone" node with translation = parentOffset + position
// 2. A "mesh" child node with translation = shapeOffset (mesh geometry centered at origin)
func (e *GLBExporter) processNodeBlockbench(node *blockymodel.Node, materialIdx uint32, parentOffset [3]float64) int {
	scale := 1.0 / 16.0

	// Get this node's position
	posX, posY, posZ := 0.0, 0.0, 0.0
	if node.Position != nil {
		posX = node.Position.X * scale
		posY = node.Position.Y * scale
		posZ = node.Position.Z * scale
	}

	// Get this node's shape offset (for mesh node and for child bones)
	offsetX, offsetY, offsetZ := 0.0, 0.0, 0.0
	if node.Shape != nil && node.Shape.Offset != nil {
		offsetX = node.Shape.Offset.X * scale
		offsetY = node.Shape.Offset.Y * scale
		offsetZ = node.Shape.Offset.Z * scale
	}

	// Create bone node with translation = parentOffset + position
	boneNode := &gltf.Node{
		Name: node.Name,
		Translation: [3]float64{
			parentOffset[0] + posX,
			parentOffset[1] + posY,
			parentOffset[2] + posZ,
		},
	}

	// Set rotation from orientation (quaternion)
	if node.Orientation != nil {
		boneNode.Rotation = [4]float64{
			node.Orientation.X,
			node.Orientation.Y,
			node.Orientation.Z,
			node.Orientation.W,
		}
	}

	var boneChildren []int

	// Create mesh node if this node has a visible shape
	if node.Shape != nil && node.Shape.Type != "none" {
		meshIdx := e.createMeshCentered(node, materialIdx)
		if meshIdx >= 0 {
			// Create mesh node with translation = shape offset.
			// It is deliberately NOT named after the node: a blockymodel node
			// can be both a bone and geometry, which splits into two glTF
			// nodes here, and a player binding animation tracks by bone name
			// would then have two candidates - picking the mesh child rotates
			// the geometry in place while the limb below it stays put.
			meshNode := &gltf.Node{
				Name: node.Name + MeshNodeSuffix,
				Translation: [3]float64{offsetX, offsetY, offsetZ},
				Mesh: gltf.Index(meshIdx),
			}
			meshNodeIdx := len(e.doc.Nodes)
			e.doc.Nodes = append(e.doc.Nodes, meshNode)
			boneChildren = append(boneChildren, meshNodeIdx)
		}
	}

	// Process children recursively
	// Child bones use THIS node's shape offset as their parentOffset
	childOffset := [3]float64{offsetX, offsetY, offsetZ}
	for i := range node.Children {
		childIdx := e.processNodeBlockbench(&node.Children[i], materialIdx, childOffset)
		if childIdx >= 0 {
			boneChildren = append(boneChildren, childIdx)
		}
	}

	if len(boneChildren) > 0 {
		boneNode.Children = boneChildren
	}

	boneNodeIdx := len(e.doc.Nodes)
	e.doc.Nodes = append(e.doc.Nodes, boneNode)
	return boneNodeIdx
}

// createMesh creates a mesh for the given node (with offset baked in)
func (e *GLBExporter) createMesh(node *blockymodel.Node, materialIdx uint32) int {
	if node.Shape.Type == "box" {
		return e.createBoxMesh(node, materialIdx, true)
	} else if node.Shape.Type == "quad" {
		return e.createQuadMesh(node, materialIdx, true)
	}
	return -1
}

// createMeshCentered creates a mesh centered at origin (offset handled by node translation)
func (e *GLBExporter) createMeshCentered(node *blockymodel.Node, materialIdx uint32) int {
	if node.Shape.Type == "box" {
		return e.createBoxMesh(node, materialIdx, false)
	} else if node.Shape.Type == "quad" {
		return e.createQuadMesh(node, materialIdx, false)
	}
	return -1
}

// Face data structure
type faceInfo struct {
	name     string
	axis     int     // 0=X, 1=Y, 2=Z
	positive bool    // true=positive direction
	uvWidth  float64 // texture width for this face
	uvHeight float64 // texture height for this face
}

// createBoxMesh creates a box mesh following Blockbench's exact format
// If applyOffset is false, mesh is centered at origin (offset handled by node translation)
func (e *GLBExporter) createBoxMesh(node *blockymodel.Node, materialIdx uint32, applyOffset bool) int {
	// Get size from settings
	sizeX, sizeY, sizeZ := 1.0, 1.0, 1.0
	if node.Shape.Settings != nil {
		if size, ok := node.Shape.Settings["size"].(map[string]interface{}); ok {
			if x, ok := size["x"].(float64); ok {
				sizeX = x
			}
			if y, ok := size["y"].(float64); ok {
				sizeY = y
			}
			if z, ok := size["z"].(float64); ok {
				sizeZ = z
			}
		}
	}

	// Store original sizes for UV calculation (before stretch)
	origSizeX, origSizeY, origSizeZ := sizeX, sizeY, sizeZ

	// Get stretch values (negative values flip geometry)
	stretchX, stretchY, stretchZ := 1.0, 1.0, 1.0
	if node.Shape.Stretch != nil {
		stretchX = node.Shape.Stretch.X
		stretchY = node.Shape.Stretch.Y
		stretchZ = node.Shape.Stretch.Z
	}

	// Apply absolute stretch for sizing
	sizeX *= math.Abs(stretchX)
	sizeY *= math.Abs(stretchY)
	sizeZ *= math.Abs(stretchZ)

	// Get offset - only apply if requested (otherwise mesh is centered at origin)
	ox, oy, oz := 0.0, 0.0, 0.0
	if applyOffset && node.Shape.Offset != nil {
		scale := 1.0 / 16.0
		ox = node.Shape.Offset.X * scale
		oy = node.Shape.Offset.Y * scale
		oz = node.Shape.Offset.Z * scale
	}

	scale := 1.0 / 16.0
	hx := sizeX * scale / 2
	hy := sizeY * scale / 2
	hz := sizeZ * scale / 2

	// Flip signs for negative stretch (mirrors geometry)
	flipX := stretchX < 0
	flipY := stretchY < 0
	flipZ := stretchZ < 0

	// Pre-allocate: max 6 faces × 4 verts = 24 verts, 6 faces × 6 indices = 36
	positions := make([][3]float32, 0, 24)
	normals := make([][3]float32, 0, 24)
	uvs := make([][2]float32, 0, 24)
	indices := make([]uint16, 0, 36)

	// Blockbench face order: east, west, up, down, south, north
	// This maps to: X+, X-, Y+, Y-, Z+, Z-
	faces := []faceInfo{
		{"right", 0, true, origSizeZ, origSizeY},   // east (X+)
		{"left", 0, false, origSizeZ, origSizeY},   // west (X-)
		{"top", 1, true, origSizeX, origSizeZ},     // up (Y+)
		{"bottom", 1, false, origSizeX, origSizeZ}, // down (Y-)
		{"front", 2, true, origSizeX, origSizeY},   // south (Z+)
		{"back", 2, false, origSizeX, origSizeY},   // north (Z-)
	}

	for _, face := range faces {
		if node.Shape.TextureLayout == nil {
			continue
		}
		layout, hasLayout := node.Shape.TextureLayout[face.name]
		if !hasLayout {
			continue
		}

		// Generate vertices for this face following Blockbench's BoxGeometry order
		// Each face has 4 vertices in order: 0=TL, 1=TR, 2=BL, 3=BR (from face's perspective)
		var faceVerts [4][3]float32
		var faceNormal [3]float32

		switch {
		case face.axis == 0 && face.positive: // east (X+)
			faceNormal = [3]float32{1, 0, 0}
			faceVerts = [4][3]float32{
				{float32(hx + ox), float32(hy + oy), float32(hz + oz)},   // TL
				{float32(hx + ox), float32(hy + oy), float32(-hz + oz)},  // TR
				{float32(hx + ox), float32(-hy + oy), float32(hz + oz)},  // BL
				{float32(hx + ox), float32(-hy + oy), float32(-hz + oz)}, // BR
			}
		case face.axis == 0 && !face.positive: // west (X-)
			faceNormal = [3]float32{-1, 0, 0}
			faceVerts = [4][3]float32{
				{float32(-hx + ox), float32(hy + oy), float32(-hz + oz)}, // TL
				{float32(-hx + ox), float32(hy + oy), float32(hz + oz)},  // TR
				{float32(-hx + ox), float32(-hy + oy), float32(-hz + oz)}, // BL
				{float32(-hx + ox), float32(-hy + oy), float32(hz + oz)}, // BR
			}
		case face.axis == 1 && face.positive: // up (Y+)
			faceNormal = [3]float32{0, 1, 0}
			faceVerts = [4][3]float32{
				{float32(-hx + ox), float32(hy + oy), float32(-hz + oz)}, // TL
				{float32(hx + ox), float32(hy + oy), float32(-hz + oz)},  // TR
				{float32(-hx + ox), float32(hy + oy), float32(hz + oz)},  // BL
				{float32(hx + ox), float32(hy + oy), float32(hz + oz)},   // BR
			}
		case face.axis == 1 && !face.positive: // down (Y-)
			faceNormal = [3]float32{0, -1, 0}
			faceVerts = [4][3]float32{
				{float32(-hx + ox), float32(-hy + oy), float32(hz + oz)},  // TL
				{float32(hx + ox), float32(-hy + oy), float32(hz + oz)},   // TR
				{float32(-hx + ox), float32(-hy + oy), float32(-hz + oz)}, // BL
				{float32(hx + ox), float32(-hy + oy), float32(-hz + oz)},  // BR
			}
		case face.axis == 2 && face.positive: // south (Z+)
			faceNormal = [3]float32{0, 0, 1}
			faceVerts = [4][3]float32{
				{float32(-hx + ox), float32(hy + oy), float32(hz + oz)},  // TL
				{float32(hx + ox), float32(hy + oy), float32(hz + oz)},   // TR
				{float32(-hx + ox), float32(-hy + oy), float32(hz + oz)}, // BL
				{float32(hx + ox), float32(-hy + oy), float32(hz + oz)},  // BR
			}
		case face.axis == 2 && !face.positive: // north (Z-)
			faceNormal = [3]float32{0, 0, -1}
			faceVerts = [4][3]float32{
				{float32(hx + ox), float32(hy + oy), float32(-hz + oz)},   // TL
				{float32(-hx + ox), float32(hy + oy), float32(-hz + oz)},  // TR
				{float32(hx + ox), float32(-hy + oy), float32(-hz + oz)},  // BL
				{float32(-hx + ox), float32(-hy + oy), float32(-hz + oz)}, // BR
			}
		}

		// Calculate UVs following Blockbench's getUVArray logic
		// face.uv = [left, top, right, bottom] in pixel coords
		// UV = [u, 1-v] where v is flipped
		faceUVs := e.calculateUVs(layout, face.uvWidth, face.uvHeight)

		// Apply stretch flip to vertices (mirrors geometry)
		for i := range faceVerts {
			if flipX {
				faceVerts[i][0] = -faceVerts[i][0]
			}
			if flipY {
				faceVerts[i][1] = -faceVerts[i][1]
			}
			if flipZ {
				faceVerts[i][2] = -faceVerts[i][2]
			}
		}

		// Flip normal if odd number of axes are flipped (to maintain correct lighting)
		flippedNormal := faceNormal
		if flipX {
			flippedNormal[0] = -flippedNormal[0]
		}
		if flipY {
			flippedNormal[1] = -flippedNormal[1]
		}
		if flipZ {
			flippedNormal[2] = -flippedNormal[2]
		}

		// Add vertices
		baseIdx := uint16(len(positions))
		for i := 0; i < 4; i++ {
			positions = append(positions, faceVerts[i])
			normals = append(normals, flippedNormal)
			uvs = append(uvs, faceUVs[i])
		}

		// Triangle indices - reverse winding if odd number of flips
		oddFlips := (flipX != flipY) != flipZ // XOR chain
		if oddFlips {
			// Reversed winding: 0,1,2 and 2,1,3
			indices = append(indices, baseIdx+0, baseIdx+1, baseIdx+2, baseIdx+2, baseIdx+1, baseIdx+3)
		} else {
			// Normal winding: 0,2,1 and 2,3,1
			indices = append(indices, baseIdx+0, baseIdx+2, baseIdx+1, baseIdx+2, baseIdx+3, baseIdx+1)
		}
	}

	if len(positions) == 0 {
		return -1
	}

	return e.createGLTFMesh(node.Name, positions, normals, uvs, indices, e.materialFor(materialIdx, node.Shape))
}

// calculateUVs converts blockymodel textureLayout to GLB UV coordinates
// This follows the exact logic from the blockymodel Blockbench plugin
func (e *GLBExporter) calculateUVs(layout blockymodel.TextureFace, uvWidth, uvHeight float64) [4][2]float32 {
	// Step 1: Convert blockymodel textureLayout to Blockbench face.uv format
	// Based on blockymodel-blockbench-plugin/src/blockymodel.ts lines 685-730
	
	uvOffset := [2]float64{layout.Offset.X, layout.Offset.Y}
	uvSize := [2]float64{uvWidth, uvHeight}
	uvMirror := [2]float64{1, 1}
	if layout.Mirror.X {
		uvMirror[0] = -1
	}
	if layout.Mirror.Y {
		uvMirror[1] = -1
	}
	
	var faceUV [4]float64 // [u0, v0, u1, v1] in Blockbench format
	
	switch int(layout.Angle) {
	case 90:
		// Swap size and mirror, negate mirror[0]
		uvSize[0], uvSize[1] = uvSize[1], uvSize[0]
		uvMirror[0], uvMirror[1] = uvMirror[1], uvMirror[0]
		uvMirror[0] *= -1
		faceUV = [4]float64{
			uvOffset[0],
			uvOffset[1] + uvSize[1]*uvMirror[1],
			uvOffset[0] + uvSize[0]*uvMirror[0],
			uvOffset[1],
		}
	case 270:
		// Swap size and mirror, negate mirror[1]
		uvSize[0], uvSize[1] = uvSize[1], uvSize[0]
		uvMirror[0], uvMirror[1] = uvMirror[1], uvMirror[0]
		uvMirror[1] *= -1
		faceUV = [4]float64{
			uvOffset[0] + uvSize[0]*uvMirror[0],
			uvOffset[1],
			uvOffset[0],
			uvOffset[1] + uvSize[1]*uvMirror[1],
		}
	case 180:
		// Negate both mirrors
		uvMirror[0] *= -1
		uvMirror[1] *= -1
		faceUV = [4]float64{
			uvOffset[0] + uvSize[0]*uvMirror[0],
			uvOffset[1] + uvSize[1]*uvMirror[1],
			uvOffset[0],
			uvOffset[1],
		}
	default: // case 0
		faceUV = [4]float64{
			uvOffset[0],
			uvOffset[1],
			uvOffset[0] + uvSize[0]*uvMirror[0],
			uvOffset[1] + uvSize[1]*uvMirror[1],
		}
	}
	
	// Step 2: Apply Blockbench's getUVArray logic
	// Based on blockbench-js/preview/preview_scenes.js lines 265-281
	// This creates UV coordinates with V flipped (1-v) for WebGL
	u0 := faceUV[0] / e.atlasWidth
	v0 := faceUV[1] / e.atlasHeight
	u1 := faceUV[2] / e.atlasWidth
	v1 := faceUV[3] / e.atlasHeight
	
	// Nudge UVs inward by 1/8 px so MSAA edge fragments, which standard glTF
	// viewers interpolate at the pixel center (possibly outside the triangle),
	// don't extrapolate into neighbouring atlas texels. Blockbench hides the
	// same artifact with centroid-sampling shaders, which glTF cannot express.
	// 1/8 px keeps texel density visually intact, unlike a half-pixel inset.
	insetU := (1.0 / 8.0) / e.atlasWidth
	insetV := (1.0 / 8.0) / e.atlasHeight
	if u0 < u1 {
		u0 += insetU
		u1 -= insetU
	} else {
		u0 -= insetU
		u1 += insetU
	}
	if v0 < v1 {
		v0 += insetV
		v1 -= insetV
	} else {
		v0 -= insetV
		v1 += insetV
	}

	// getUVArray creates: TL=(u0, 1-v0), TR=(u1, 1-v0), BL=(u0, 1-v1), BR=(u1, 1-v1)
	arr := [4][2]float32{
		{float32(u0), float32(1 - v0)}, // TL
		{float32(u1), float32(1 - v0)}, // TR
		{float32(u0), float32(1 - v1)}, // BL
		{float32(u1), float32(1 - v1)}, // BR
	}
	
	// Apply rotation (same as getUVArray)
	angle := int(layout.Angle)
	for angle > 0 {
		tmp := arr[0]
		arr[0] = arr[2]
		arr[2] = arr[3]
		arr[3] = arr[1]
		arr[1] = tmp
		angle -= 90
	}
	
	// Step 3: Apply GLTFExporter's V flip (line 1309-1310 in GLTFExporter.js)
	// This "undoes" the V flip from step 2: v = 1 - v
	for i := 0; i < 4; i++ {
		arr[i][1] = 1 - arr[i][1]
	}
	
	return arr
}

// createQuadMesh creates a quad mesh
func (e *GLBExporter) createQuadMesh(node *blockymodel.Node, materialIdx uint32, applyOffset bool) int {
	sizeX, sizeY := 1.0, 1.0
	if node.Shape.Settings != nil {
		if size, ok := node.Shape.Settings["size"].(map[string]interface{}); ok {
			if x, ok := size["x"].(float64); ok {
				sizeX = x
			}
			if y, ok := size["y"].(float64); ok {
				sizeY = y
			}
		}
	}

	origSizeX, origSizeY := sizeX, sizeY

	// Get stretch values (negative values flip geometry)
	stretchX, stretchY, stretchZ := 1.0, 1.0, 1.0
	if node.Shape.Stretch != nil {
		stretchX = node.Shape.Stretch.X
		stretchY = node.Shape.Stretch.Y
		stretchZ = node.Shape.Stretch.Z
	}

	// Apply absolute stretch for sizing
	sizeX *= math.Abs(stretchX)
	sizeY *= math.Abs(stretchY)

	scale := 1.0 / 16.0
	hx := sizeX * scale / 2
	hy := sizeY * scale / 2

	// Get offset - only apply if requested
	ox, oy, oz := 0.0, 0.0, 0.0
	if applyOffset && node.Shape.Offset != nil {
		ox = node.Shape.Offset.X * scale
		oy = node.Shape.Offset.Y * scale
		oz = node.Shape.Offset.Z * scale
	}

	// Get the normal direction from settings
	normalDir := "+Z"
	if node.Shape.Settings != nil {
		if n, ok := node.Shape.Settings["normal"].(string); ok {
			normalDir = n
		}
	}

	// Create quad vertices based on normal direction
	var positions [][3]float32
	var normal [3]float32

	switch normalDir {
	case "+Z": // south face
		positions = [][3]float32{
			{float32(-hx + ox), float32(hy + oy), float32(oz)},  // TL
			{float32(hx + ox), float32(hy + oy), float32(oz)},   // TR
			{float32(-hx + ox), float32(-hy + oy), float32(oz)}, // BL
			{float32(hx + ox), float32(-hy + oy), float32(oz)},  // BR
		}
		normal = [3]float32{0, 0, 1}
	case "-Z": // north face
		positions = [][3]float32{
			{float32(hx + ox), float32(hy + oy), float32(oz)},   // TL
			{float32(-hx + ox), float32(hy + oy), float32(oz)},  // TR
			{float32(hx + ox), float32(-hy + oy), float32(oz)},  // BL
			{float32(-hx + ox), float32(-hy + oy), float32(oz)}, // BR
		}
		normal = [3]float32{0, 0, -1}
	case "+X": // east face
		positions = [][3]float32{
			{float32(ox), float32(hy + oy), float32(hx + oz)},   // TL
			{float32(ox), float32(hy + oy), float32(-hx + oz)},  // TR
			{float32(ox), float32(-hy + oy), float32(hx + oz)},  // BL
			{float32(ox), float32(-hy + oy), float32(-hx + oz)}, // BR
		}
		normal = [3]float32{1, 0, 0}
	case "-X": // west face
		positions = [][3]float32{
			{float32(ox), float32(hy + oy), float32(-hx + oz)},  // TL
			{float32(ox), float32(hy + oy), float32(hx + oz)},   // TR
			{float32(ox), float32(-hy + oy), float32(-hx + oz)}, // BL
			{float32(ox), float32(-hy + oy), float32(hx + oz)},  // BR
		}
		normal = [3]float32{-1, 0, 0}
	case "+Y": // up face
		positions = [][3]float32{
			{float32(-hx + ox), float32(oy), float32(-hy + oz)}, // TL
			{float32(hx + ox), float32(oy), float32(-hy + oz)},  // TR
			{float32(-hx + ox), float32(oy), float32(hy + oz)},  // BL
			{float32(hx + ox), float32(oy), float32(hy + oz)},   // BR
		}
		normal = [3]float32{0, 1, 0}
	case "-Y": // down face
		positions = [][3]float32{
			{float32(-hx + ox), float32(oy), float32(hy + oz)},  // TL
			{float32(hx + ox), float32(oy), float32(hy + oz)},   // TR
			{float32(-hx + ox), float32(oy), float32(-hy + oz)}, // BL
			{float32(hx + ox), float32(oy), float32(-hy + oz)},  // BR
		}
		normal = [3]float32{0, -1, 0}
	default:
		positions = [][3]float32{
			{float32(-hx + ox), float32(hy + oy), float32(oz)},  // TL
			{float32(hx + ox), float32(hy + oy), float32(oz)},   // TR
			{float32(-hx + ox), float32(-hy + oy), float32(oz)}, // BL
			{float32(hx + ox), float32(-hy + oy), float32(oz)},  // BR
		}
		normal = [3]float32{0, 0, 1}
	}

	// Apply stretch flip to vertices (mirrors geometry)
	flipX := stretchX < 0
	flipY := stretchY < 0
	flipZ := stretchZ < 0
	for i := range positions {
		if flipX {
			positions[i][0] = -positions[i][0]
		}
		if flipY {
			positions[i][1] = -positions[i][1]
		}
		if flipZ {
			positions[i][2] = -positions[i][2]
		}
	}

	// Flip normal if axes are flipped (to maintain correct lighting)
	flippedNormal := normal
	if flipX {
		flippedNormal[0] = -flippedNormal[0]
	}
	if flipY {
		flippedNormal[1] = -flippedNormal[1]
	}
	if flipZ {
		flippedNormal[2] = -flippedNormal[2]
	}

	var uvs [4][2]float32
	if node.Shape.TextureLayout != nil {
		if layout, ok := node.Shape.TextureLayout["front"]; ok {
			uvs = e.calculateUVs(layout, origSizeX, origSizeY)
			// Debug: log eyelid UVs
			if strings.Contains(strings.ToLower(node.Name), "eyelid") {
				util.Logger.Debug("Eyelid texture layout",
					"node", node.Name,
					"layout", layout,
					"sizeX", origSizeX,
					"sizeY", origSizeY,
					"uvs", uvs)
			}
		}
	} else {
		// Debug: log when textureLayout is nil
		if strings.Contains(strings.ToLower(node.Name), "eyelid") {
			util.Logger.Debug("Eyelid texture layout is nil", "node", node.Name)
		}
	}

	normalsArr := make([][3]float32, 4)
	uvsArr := make([][2]float32, 4)
	for i := 0; i < 4; i++ {
		normalsArr[i] = flippedNormal
		uvsArr[i] = uvs[i]
	}

	// Triangle indices - reverse winding if odd number of flips
	oddFlips := (flipX != flipY) != flipZ // XOR chain
	var indices []uint16
	if oddFlips {
		// Reversed winding: 0,1,2 and 2,1,3
		indices = []uint16{0, 1, 2, 2, 1, 3}
	} else {
		// Normal winding: 0,2,1 and 2,3,1
		indices = []uint16{0, 2, 1, 2, 3, 1}
	}

	// Note: DoubleSided is handled by the material property, not by creating duplicate geometry
	// Creating duplicate back-face geometry at the same position causes z-fighting artifacts

	return e.createGLTFMesh(node.Name, positions, normalsArr, uvsArr, indices, e.materialFor(materialIdx, node.Shape))
}

// createGLTFMesh creates the actual GLTF mesh from geometry data
func (e *GLBExporter) createGLTFMesh(name string, positions [][3]float32, normals [][3]float32, uvs [][2]float32, indices []uint16, materialIdx uint32) int {
	positionAccessor := modeler.WritePosition(e.doc, positions)
	normalAccessor := modeler.WriteNormal(e.doc, normals)
	uvAccessor := modeler.WriteTextureCoord(e.doc, uvs)
	indicesAccessor := modeler.WriteIndices(e.doc, indices)

	meshIdx := len(e.doc.Meshes)
	e.doc.Meshes = append(e.doc.Meshes, &gltf.Mesh{
		Name: name,
		Primitives: []*gltf.Primitive{
			{
				Attributes: gltf.PrimitiveAttributes{
					gltf.POSITION:   positionAccessor,
					gltf.NORMAL:     normalAccessor,
					gltf.TEXCOORD_0: uvAccessor,
				},
				Indices:  gltf.Index(indicesAccessor),
				Material: gltf.Index(int(materialIdx)),
			},
		},
	})

	return meshIdx
}

// Save writes the GLB to a file (binary format)
func (e *GLBExporter) Save(path string) error {
	return gltf.SaveBinary(e.doc, path)
}

// Bytes returns the GLB as bytes
func (e *GLBExporter) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := gltf.NewEncoder(&buf).Encode(e.doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
