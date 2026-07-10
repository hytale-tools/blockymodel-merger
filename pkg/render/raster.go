package render

import (
	"image"
	"math"
)

// depthBias prevents z-fighting between coplanar surfaces.
const depthBias = 0.001

// LightConfig controls optional directional lighting.
type LightConfig struct {
	Enabled   bool
	Direction Vec3
	Ambient   float32
	Diffuse   float32
}

// DefaultLight returns a soft front-top directional light.
func DefaultLight() LightConfig {
	return LightConfig{
		Enabled:   true,
		Direction: Vec3{0.4, 0.8, 0.6}.Normalize(),
		Ambient:   0.55,
		Diffuse:   0.45,
	}
}

// RenderConfig controls sampling, shading and parallelism. The zero value is the
// fastest single-threaded path (nearest-neighbour, no lighting).
type RenderConfig struct {
	Bilinear bool
	Light    LightConfig
	// Threads controls intra-render parallelism (rows are split into bands).
	//   0  -> auto (runtime.NumCPU())
	//   1  -> single-threaded (best when many renders run concurrently)
	//   >1 -> fixed band count (lowest latency for a single large render)
	Threads int
}

// projectedTri is a screen-space triangle with everything the fill loop needs
// precomputed: positions+depth, face-local UVs, the UV-layout constants, an
// optional lighting LUT, and a vertical bounding box for band skipping.
type projectedTri struct {
	x, y, z [3]float32
	u, v    [3]float32

	// Precomputed UV-layout constants (see transformUVCoords):
	// texU = ox + faceU*muU, texV = oy + faceV*muV, then rotation by angle.
	ox, oy, muU, muV float32
	angle            int

	lut        *[256]uint8 // gamma-shade LUT when lit, else nil
	minY, maxY int
}

// fillTri rasterizes one triangle into the rows [yStart, yEnd) using incremental
// edge functions (barycentric weights stepped by constants per pixel).
func fillTri(img *image.RGBA, depth []float32, width, height int, yStart, yEnd int, t *projectedTri, tex *Texture, bilinear bool) {
	if t.maxY < yStart || t.minY >= yEnd {
		return
	}

	x0, y0, z0 := t.x[0], t.y[0], t.z[0]
	x1, y1, z1 := t.x[1], t.y[1], t.z[1]
	x2, y2, z2 := t.x[2], t.y[2], t.z[2]

	area := (x1-x0)*(y2-y0) - (x2-x0)*(y1-y0)
	if area < 1e-7 && area > -1e-7 {
		return
	}
	invArea := 1.0 / area

	minX := int(maxF(minF(minF(x0, x1), x2), 0))
	maxX := int(minF(maxF(maxF(x0, x1), x2), float32(width-1)))
	minY := t.minY
	if minY < yStart {
		minY = yStart
	}
	maxY := t.maxY
	if maxY > yEnd-1 {
		maxY = yEnd - 1
	}
	if minX > maxX || minY > maxY {
		return
	}

	// Per-pixel x-steps of the (normalized) barycentric weights.
	stepX0 := (y1 - y2) * invArea
	stepX1 := (y2 - y0) * invArea
	stepX2 := (y0 - y1) * invArea

	u0, u1, u2 := t.u[0], t.u[1], t.u[2]
	v0, v1, v2 := t.v[0], t.v[1], t.v[2]
	ox, oy, muU, muV := t.ox, t.oy, t.muU, t.muV
	angle := t.angle
	lut := t.lut

	pxStart := float32(minX) + 0.5
	for y := minY; y <= maxY; y++ {
		py := float32(y) + 0.5
		// Edge functions at the row's first pixel center.
		e0 := (x2-x1)*(py-y1) - (y2-y1)*(pxStart-x1)
		e1 := (x0-x2)*(py-y2) - (y0-y2)*(pxStart-x2)
		e2 := (x1-x0)*(py-y0) - (y1-y0)*(pxStart-x0)
		w0 := e0 * invArea
		w1 := e1 * invArea
		w2 := e2 * invArea

		rowOff := y * width
		for x := minX; x <= maxX; x++ {
			if w0 >= 0 && w1 >= 0 && w2 >= 0 {
				idx := rowOff + x
				d := w0*z0 + w1*z1 + w2*z2
				if d < depth[idx]-depthBias {
					faceU := w0*u0 + w1*u1 + w2*u2
					faceV := w0*v0 + w1*v1 + w2*v2

					var r, g, b, a uint8
					if bilinear {
						r, g, b, a = sampleLayoutBilinear(tex, ox, oy, muU, muV, angle, faceU, faceV)
					} else {
						r, g, b, a = sampleLayoutNearest(tex, ox, oy, muU, muV, angle, faceU, faceV)
					}
					if a != 0 {
						if lut != nil {
							r, g, b = lut[r], lut[g], lut[b]
						}
						depth[idx] = d
						o := idx << 2
						pix := img.Pix[o : o+4 : o+4]
						pix[0], pix[1], pix[2], pix[3] = r, g, b, a
					}
				}
			}
			w0 += stepX0
			w1 += stepX1
			w2 += stepX2
		}
	}
}

// sampleLayoutNearest resolves face-local (u,v) to a texture pixel using the
// precomputed layout constants, then samples nearest-neighbour.
func sampleLayoutNearest(tex *Texture, ox, oy, muU, muV float32, angle int, u, v float32) (uint8, uint8, uint8, uint8) {
	tu := ox + u*muU
	tv := oy + v*muV
	tu, tv = rotateUV(tu, tv, ox, oy, angle)
	return tex.sampleRGBA(tu, tv)
}

func sampleLayoutBilinear(tex *Texture, ox, oy, muU, muV float32, angle int, u, v float32) (uint8, uint8, uint8, uint8) {
	const eps = 0.001
	u = clamp32(u, eps, 1-eps)
	v = clamp32(v, eps, 1-eps)
	tu := ox + u*muU
	tv := oy + v*muV
	tu, tv = rotateUV(tu, tv, ox, oy, angle)
	c := tex.bilinearAt(tu, tv)
	return c.R, c.G, c.B, c.A
}

func rotateUV(u, v, ox, oy float32, angle int) (float32, float32) {
	if angle == 0 {
		return u, v
	}
	relU := u - ox
	relV := v - oy
	switch angle {
	case 90:
		return ox - relV, oy + relU
	case 180:
		return ox - relU, oy - relV
	case 270:
		return ox + relV, oy - relU
	}
	return u, v
}

// buildLightLUT precomputes a gamma-correct shading table for a fixed lighting
// factor.
func buildLightLUT(lighting float32) *[256]uint8 {
	var lut [256]uint8
	for i := 0; i < 256; i++ {
		linear := float32(math.Pow(float64(i)/255.0, 2.2)) * lighting
		out := float32(math.Pow(float64(linear), 1.0/2.2)) * 255.0
		lut[i] = uint8(clamp32(out, 0, 255))
	}
	return &lut
}
