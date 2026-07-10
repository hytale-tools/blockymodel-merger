package render

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
)

// Texture is a fast-access RGBA copy of the (already-tinted) atlas image.
//
// Note: tinting is NOT applied at sample time. The pipeline bakes tint gradients
// into the atlas before rendering, so the atlas pixels are already final colours.
type Texture struct {
	pix    []uint8
	stride int
	width  int
	height int
}

// NewTexture wraps an image as an RGBA texture (copying if necessary).
func NewTexture(src image.Image) *Texture {
	b := src.Bounds()
	rgba, ok := src.(*image.RGBA)
	if !ok || b.Min.X != 0 || b.Min.Y != 0 {
		rgba = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)
	}
	return &Texture{
		pix:    rgba.Pix,
		stride: rgba.Stride,
		width:  rgba.Bounds().Dx(),
		height: rgba.Bounds().Dy(),
	}
}

func (t *Texture) Dimensions() (int, int) { return t.width, t.height }

// sampleRGBA samples at absolute (floored) pixel coordinates, clamped to bounds,
// returning channels directly to avoid a color.RGBA allocation in the hot loop.
func (t *Texture) sampleRGBA(x, y float32) (uint8, uint8, uint8, uint8) {
	px := int(x)
	py := int(y)
	// int() truncates toward zero; for the small negatives we see, clamp handles it.
	if px < 0 {
		px = 0
	} else if px >= t.width {
		px = t.width - 1
	}
	if py < 0 {
		py = 0
	} else if py >= t.height {
		py = t.height - 1
	}
	o := py*t.stride + px*4
	return t.pix[o], t.pix[o+1], t.pix[o+2], t.pix[o+3]
}

func (t *Texture) at(px, py int) color.RGBA {
	if px < 0 {
		px = 0
	} else if px >= t.width {
		px = t.width - 1
	}
	if py < 0 {
		py = 0
	} else if py >= t.height {
		py = t.height - 1
	}
	o := py*t.stride + px*4
	return color.RGBA{t.pix[o], t.pix[o+1], t.pix[o+2], t.pix[o+3]}
}

// bilinearAt samples with alpha-aware bilinear filtering at absolute pixel
// coordinates (ported from texture.rs sample_uv_bilinear).
func (t *Texture) bilinearAt(x, y float32) color.RGBA {
	x0 := int(clamp32(floor32(x), 0, float32(t.width-1)))
	y0 := int(clamp32(floor32(y), 0, float32(t.height-1)))
	x1 := x0 + 1
	if x1 > t.width-1 {
		x1 = t.width - 1
	}
	y1 := y0 + 1
	if y1 > t.height-1 {
		y1 = t.height - 1
	}
	fx := x - float32(x0)
	fy := y - float32(y0)

	p00 := t.at(x0, y0)
	p10 := t.at(x1, y0)
	p01 := t.at(x0, y1)
	p11 := t.at(x1, y1)

	const alphaThreshold = 128
	hasOpaque, hasTransparent := false, false
	for _, a := range []uint8{p00.A, p10.A, p01.A, p11.A} {
		if a >= alphaThreshold {
			hasOpaque = true
		} else {
			hasTransparent = true
		}
	}
	if hasOpaque && hasTransparent {
		nx, ny := x0, y0
		if fx >= 0.5 {
			nx = x1
		}
		if fy >= 0.5 {
			ny = y1
		}
		return t.at(nx, ny)
	}

	top := lerpRGBA(p00, p10, fx)
	bottom := lerpRGBA(p01, p11, fx)
	return lerpRGBA(top, bottom, fy)
}

func lerpU8(a, b uint8, t float32) uint8 {
	return uint8(float32(a)*(1-t) + float32(b)*t)
}

func lerpRGBA(a, b color.RGBA, t float32) color.RGBA {
	return color.RGBA{
		lerpU8(a.R, b.R, t),
		lerpU8(a.G, b.G, t),
		lerpU8(a.B, b.B, t),
		lerpU8(a.A, b.A, t),
	}
}

// layoutForFace returns the texture layout for a named face, if present.
func layoutForFace(shape *blockymodel.Shape, textureFace string) (blockymodel.TextureFace, bool) {
	if shape == nil || shape.TextureLayout == nil {
		return blockymodel.TextureFace{}, false
	}
	l, ok := shape.TextureLayout[textureFace]
	return l, ok
}
