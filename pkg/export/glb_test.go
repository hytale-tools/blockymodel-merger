package export

import (
	"image"
	"image/color"
	"testing"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
)

// testAtlas builds an 8x8 opaque atlas with an optional transparent pixel.
func testAtlas(transparentAt *[2]int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{200, 100, 50, 255})
		}
	}
	if transparentAt != nil {
		img.Set(transparentAt[0], transparentAt[1], color.RGBA{0, 0, 0, 0})
	}
	return img
}

// closedBox returns a 2x2x2 box with all six faces mapped at the given offset.
func closedBox(ox, oy float64) *blockymodel.Shape {
	layout := map[string]blockymodel.TextureFace{}
	for _, face := range []string{"front", "back", "left", "right", "top", "bottom"} {
		layout[face] = blockymodel.TextureFace{Offset: blockymodel.Vec2{X: ox, Y: oy}}
	}
	return &blockymodel.Shape{
		Type:          "box",
		Settings:      map[string]interface{}{"size": map[string]interface{}{"x": 2.0, "y": 2.0, "z": 2.0}},
		TextureLayout: layout,
	}
}

func exporterWithAtlas(img image.Image) *GLBExporter {
	e := NewGLBExporter()
	e.atlasImage = img
	return e
}

func TestClosedOpaqueBoxIsSingleSided(t *testing.T) {
	e := exporterWithAtlas(testAtlas(nil))
	if e.shapeNeedsDoubleSided(closedBox(0, 0)) {
		t.Error("fully closed opaque box should not need backfaces")
	}
}

func TestFlaggedShapeIsDoubleSided(t *testing.T) {
	e := exporterWithAtlas(testAtlas(nil))
	ds := true
	shape := closedBox(0, 0)
	shape.DoubleSided = &ds
	if !e.shapeNeedsDoubleSided(shape) {
		t.Error("doubleSided-flagged shape must render backfaces")
	}
}

func TestQuadIsDoubleSided(t *testing.T) {
	e := exporterWithAtlas(testAtlas(nil))
	if !e.shapeNeedsDoubleSided(&blockymodel.Shape{Type: "quad"}) {
		t.Error("quads are flat and must render from both sides")
	}
}

func TestOpenBoxIsDoubleSided(t *testing.T) {
	e := exporterWithAtlas(testAtlas(nil))
	shape := closedBox(0, 0)
	delete(shape.TextureLayout, "top")
	if !e.shapeNeedsDoubleSided(shape) {
		t.Error("box with a missing face exposes its interior and must render backfaces")
	}
}

func TestCutoutBoxIsDoubleSided(t *testing.T) {
	e := exporterWithAtlas(testAtlas(&[2]int{1, 1}))
	if !e.shapeNeedsDoubleSided(closedBox(0, 0)) {
		t.Error("box with a transparent texel in a face must render backfaces")
	}
}

func TestCutoutOutsideFaceRectIsIgnored(t *testing.T) {
	// Transparent pixel at (7,7); faces at offset (0,0) span only 2x2 texels.
	e := exporterWithAtlas(testAtlas(&[2]int{7, 7}))
	if e.shapeNeedsDoubleSided(closedBox(0, 0)) {
		t.Error("transparency outside the face rects must not force backfaces")
	}
}

func TestMirroredFaceRectExtendsBackwards(t *testing.T) {
	// Mirrored X face at offset (4,0) samples [2,4), where the transparent
	// pixel sits; the unmirrored rect [4,6) is opaque.
	e := exporterWithAtlas(testAtlas(&[2]int{3, 0}))
	shape := closedBox(4, 0)
	front := shape.TextureLayout["front"]
	front.Mirror = blockymodel.Vec2Bool{X: true}
	shape.TextureLayout["front"] = front
	if !e.shapeNeedsDoubleSided(shape) {
		t.Error("mirrored face rect must be scanned backwards from its offset")
	}
	if e.shapeNeedsDoubleSided(closedBox(4, 0)) {
		t.Error("unmirrored rect at the same offset is opaque and must stay single-sided")
	}
}

func TestRotatedFaceRectSwapsDimensions(t *testing.T) {
	// A 4x2x2 box's front face is 4x2; rotated 90 degrees it occupies 2x4.
	// Transparent pixel at (1,3) is inside the rotated rect only.
	shape := &blockymodel.Shape{
		Type:     "box",
		Settings: map[string]interface{}{"size": map[string]interface{}{"x": 4.0, "y": 2.0, "z": 2.0}},
		TextureLayout: map[string]blockymodel.TextureFace{
			"front":  {Offset: blockymodel.Vec2{X: 0, Y: 0}, Angle: 90},
			"back":   {Offset: blockymodel.Vec2{X: 4, Y: 4}},
			"left":   {Offset: blockymodel.Vec2{X: 4, Y: 4}},
			"right":  {Offset: blockymodel.Vec2{X: 4, Y: 4}},
			"top":    {Offset: blockymodel.Vec2{X: 4, Y: 4}},
			"bottom": {Offset: blockymodel.Vec2{X: 4, Y: 4}},
		},
	}
	e := exporterWithAtlas(testAtlas(&[2]int{1, 3}))
	if !e.shapeNeedsDoubleSided(shape) {
		t.Error("rotated face rect must swap width and height when scanning")
	}

	front := shape.TextureLayout["front"]
	front.Angle = -90 // negative angles must normalize like positive ones
	shape.TextureLayout["front"] = front
	if !e.shapeNeedsDoubleSided(shape) {
		t.Error("negative rotation must normalize and still swap dimensions")
	}
}

func TestNilAtlasSkipsTransparencyCheck(t *testing.T) {
	e := NewGLBExporter()
	if e.shapeNeedsDoubleSided(closedBox(0, 0)) {
		t.Error("without an atlas, closed boxes must default to single-sided")
	}
}
