package render

import (
	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
)

// Vertex carries position, normal and UV (0..1 within the face).
type Vertex struct {
	Pos    Vec3
	Normal Vec3
	U, V   float32
}

// Face is a polygon (quad before clipping) plus the shape it came from, so the
// rasterizer can resolve the texture layout for the named face.
type Face struct {
	Vertices    []Vertex
	TextureFace string // front/back/left/right/top/bottom
	Shape       *blockymodel.Shape
}

// quadUVs are the standard per-corner UVs (counter-clockwise from bottom-left).
// Actual texture-layout mapping happens in the sampler via the face's
// TextureFace + the shape's textureLayout.
var quadUVs = [4][2]float32{
	{0, 1}, // bottom-left
	{1, 1}, // bottom-right
	{1, 0}, // top-right
	{0, 0}, // top-left
}

// shapeSize reads settings.size, defaulting to def.
func shapeSize(shape *blockymodel.Shape, def Vec3) Vec3 {
	if shape.Settings == nil {
		return def
	}
	raw, ok := shape.Settings["size"].(map[string]interface{})
	if !ok {
		return def
	}
	out := def
	if x, ok := raw["x"].(float64); ok {
		out.X = float32(x)
	}
	if y, ok := raw["y"].(float64); ok {
		out.Y = float32(y)
	}
	if z, ok := raw["z"].(float64); ok {
		out.Z = float32(z)
	}
	return out
}

func shapeOffset(shape *blockymodel.Shape) Vec3 {
	if shape.Offset == nil {
		return Vec3{}
	}
	return Vec3{float32(shape.Offset.X), float32(shape.Offset.Y), float32(shape.Offset.Z)}
}

func shapeStretch(shape *blockymodel.Shape) Vec3 {
	if shape.Stretch == nil {
		return Vec3{1, 1, 1}
	}
	return Vec3{float32(shape.Stretch.X), float32(shape.Stretch.Y), float32(shape.Stretch.Z)}
}

func shapeDoubleSided(shape *blockymodel.Shape) bool {
	return shape.DoubleSided != nil && *shape.DoubleSided
}

// generateGeometry converts a shape to faces in world space, given the node's
// bone transform. The shape's own offset+stretch are applied here.
func generateGeometry(shape *blockymodel.Shape, transform Mat4) []Face {
	switch shape.Type {
	case "box":
		return generateBox(shape, transform)
	case "quad":
		return generateQuad(shape, transform)
	default:
		return nil
	}
}

func finalTransform(shape *blockymodel.Shape, transform Mat4) Mat4 {
	shapeTransform := MatFromTranslation(shapeOffset(shape)).Mul(MatFromScale(shapeStretch(shape)))
	return transform.Mul(shapeTransform)
}

func generateBox(shape *blockymodel.Shape, transform Mat4) []Face {
	size := shapeSize(shape, Vec3{1, 1, 1})
	hx, hy, hz := size.X/2, size.Y/2, size.Z/2
	ft := finalTransform(shape, transform)

	faces := []Face{
		makeFace(shape, ft, "front", Vec3{0, 0, 1}, [4]Vec3{
			{-hx, -hy, hz}, {hx, -hy, hz}, {hx, hy, hz}, {-hx, hy, hz},
		}),
		makeFace(shape, ft, "back", Vec3{0, 0, -1}, [4]Vec3{
			{hx, -hy, -hz}, {-hx, -hy, -hz}, {-hx, hy, -hz}, {hx, hy, -hz},
		}),
		makeFace(shape, ft, "right", Vec3{1, 0, 0}, [4]Vec3{
			{hx, -hy, hz}, {hx, -hy, -hz}, {hx, hy, -hz}, {hx, hy, hz},
		}),
		makeFace(shape, ft, "left", Vec3{-1, 0, 0}, [4]Vec3{
			{-hx, -hy, -hz}, {-hx, -hy, hz}, {-hx, hy, hz}, {-hx, hy, -hz},
		}),
		makeFace(shape, ft, "top", Vec3{0, 1, 0}, [4]Vec3{
			{-hx, hy, hz}, {hx, hy, hz}, {hx, hy, -hz}, {-hx, hy, -hz},
		}),
		makeFace(shape, ft, "bottom", Vec3{0, -1, 0}, [4]Vec3{
			{-hx, -hy, -hz}, {hx, -hy, -hz}, {hx, -hy, hz}, {-hx, -hy, hz},
		}),
	}

	if shapeDoubleSided(shape) {
		faces = append(faces, reversedFaces(faces)...)
	}
	return faces
}

// quadNormalDir resolves the quad facing direction string.
func quadNormalDir(shape *blockymodel.Shape) string {
	if shape.Settings != nil {
		if n, ok := shape.Settings["normal"].(string); ok && n != "" {
			return n
		}
	}
	return "+Z"
}

func generateQuad(shape *blockymodel.Shape, transform Mat4) []Face {
	size := shapeSize(shape, Vec3{1, 1, 0})
	hx, hy, hz := size.X/2, size.Y/2, size.Z/2
	ft := finalTransform(shape, transform)

	var verts [4]Vec3
	var normal Vec3
	var faceName string

	switch quadNormalDir(shape) {
	case "+X":
		verts = [4]Vec3{{0, -hy, -hz}, {0, hy, -hz}, {0, hy, hz}, {0, -hy, hz}}
		normal, faceName = Vec3{1, 0, 0}, "right"
	case "-X":
		verts = [4]Vec3{{0, -hy, hz}, {0, hy, hz}, {0, hy, -hz}, {0, -hy, -hz}}
		normal, faceName = Vec3{-1, 0, 0}, "left"
	case "+Y":
		verts = [4]Vec3{{-hx, 0, -hz}, {hx, 0, -hz}, {hx, 0, hz}, {-hx, 0, hz}}
		normal, faceName = Vec3{0, 1, 0}, "top"
	case "-Y":
		verts = [4]Vec3{{-hx, 0, hz}, {hx, 0, hz}, {hx, 0, -hz}, {-hx, 0, -hz}}
		normal, faceName = Vec3{0, -1, 0}, "bottom"
	case "-Z":
		verts = [4]Vec3{{hx, -hy, 0}, {-hx, -hy, 0}, {-hx, hy, 0}, {hx, hy, 0}}
		normal, faceName = Vec3{0, 0, -1}, "back"
	default: // +Z
		verts = [4]Vec3{{-hx, -hy, 0}, {hx, -hy, 0}, {hx, hy, 0}, {-hx, hy, 0}}
		normal, faceName = Vec3{0, 0, 1}, "front"
	}

	// Fall back to the "front" layout if the natural face has no layout.
	finalName := faceName
	if !hasLayout(shape, faceName) && hasLayout(shape, "front") {
		finalName = "front"
	}

	face := makeFace(shape, ft, finalName, normal, verts)
	faces := []Face{face}

	if shapeDoubleSided(shape) {
		// Explicit horizontal-flip permutation (0<->1, 2<->3) for consistent winding.
		v := face.Vertices
		rev := Face{
			TextureFace: finalName,
			Shape:       shape,
			Vertices: []Vertex{
				{Pos: v[1].Pos, Normal: normal.Scale(-1), U: v[1].U, V: v[1].V},
				{Pos: v[0].Pos, Normal: normal.Scale(-1), U: v[0].U, V: v[0].V},
				{Pos: v[3].Pos, Normal: normal.Scale(-1), U: v[3].U, V: v[3].V},
				{Pos: v[2].Pos, Normal: normal.Scale(-1), U: v[2].U, V: v[2].V},
			},
		}
		faces = append(faces, rev)
	}
	return faces
}

func hasLayout(shape *blockymodel.Shape, name string) bool {
	if shape.TextureLayout == nil {
		return false
	}
	_, ok := shape.TextureLayout[name]
	return ok
}

func makeFace(shape *blockymodel.Shape, transform Mat4, name string, normal Vec3, positions [4]Vec3) Face {
	verts := make([]Vertex, 4)
	tn := transform.TransformVector3(normal).Normalize()
	for i := 0; i < 4; i++ {
		verts[i] = Vertex{
			Pos:    transform.TransformPoint3(positions[i]),
			Normal: tn,
			U:      quadUVs[i][0],
			V:      quadUVs[i][1],
		}
	}
	return Face{Vertices: verts, TextureFace: name, Shape: shape}
}

func reversedFaces(faces []Face) []Face {
	out := make([]Face, 0, len(faces))
	for _, f := range faces {
		n := len(f.Vertices)
		rev := make([]Vertex, n)
		revNormal := f.Vertices[0].Normal.Scale(-1)
		for i := 0; i < n; i++ {
			src := f.Vertices[n-1-i]
			src.Normal = revNormal
			rev[i] = src
		}
		out = append(out, Face{Vertices: rev, TextureFace: f.TextureFace, Shape: f.Shape})
	}
	return out
}

// faceUVDims returns the texture-region pixel dimensions for the named face,
// using the shape's original (pre-stretch) size.
func faceUVDims(shape *blockymodel.Shape, textureFace string) (float32, float32) {
	size := shapeSize(shape, Vec3{1, 1, 1})
	switch textureFace {
	case "front", "back":
		return size.X, size.Y
	case "left", "right":
		return size.Z, size.Y
	case "top", "bottom":
		return size.X, size.Z
	default:
		return 1, 1
	}
}

// negativeStretchCount counts axes with negative stretch (used for winding parity).
func negativeStretchCount(shape *blockymodel.Shape) int {
	s := shapeStretch(shape)
	count := 0
	for _, v := range []float32{s.X, s.Y, s.Z} {
		if v < 0 {
			count++
		}
	}
	return count
}
