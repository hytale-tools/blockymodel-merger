package render

import (
	"image"
	"math"
	"runtime"
	"sync"
)

// RenderScene renders world-space faces to an RGBA image using a per-pixel
// z-buffer. Faces may be supplied in any order.
//
// It runs in two phases: a single-threaded projection/clip pass that produces
// screen-space triangles, then a fill pass that can be split across row bands
// (cfg.Threads). The background is left transparent; tex is the (pre-tinted) atlas.
func RenderScene(faces []Face, tex *Texture, camera CameraProjection, width, height int, cfg RenderConfig) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	depth := make([]float32, width*height)
	for i := range depth {
		depth[i] = math.MaxFloat32
	}

	tris := projectFaces(faces, camera, width, height, cfg)

	threads := cfg.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	if threads > height {
		threads = height
	}
	if threads < 1 {
		threads = 1
	}

	if threads == 1 {
		for i := range tris {
			fillTri(img, depth, width, height, 0, height, &tris[i], tex, cfg.Bilinear)
		}
		return img
	}

	// Split rows into contiguous bands; each band owns disjoint rows of img/depth,
	// so the goroutines never contend.
	var wg sync.WaitGroup
	bandHeight := (height + threads - 1) / threads
	for b := 0; b < threads; b++ {
		yStart := b * bandHeight
		if yStart >= height {
			break
		}
		yEnd := yStart + bandHeight
		if yEnd > height {
			yEnd = height
		}
		wg.Add(1)
		go func(yStart, yEnd int) {
			defer wg.Done()
			for i := range tris {
				fillTri(img, depth, width, height, yStart, yEnd, &tris[i], tex, cfg.Bilinear)
			}
		}(yStart, yEnd)
	}
	wg.Wait()
	return img
}

// projectFaces transforms, clips and projects faces into screen-space triangles
// with all per-triangle constants precomputed.
func projectFaces(faces []Face, camera CameraProjection, width, height int, cfg RenderConfig) []projectedTri {
	vp := camera.ViewProjectionMatrix(uint32(width), uint32(height))
	fw, fh := float32(width), float32(height)

	tris := make([]projectedTri, 0, len(faces)*2)
	lightCache := map[float32]*[256]uint8{}

	for fi := range faces {
		face := faces[fi]

		layout, ok := layoutForFace(face.Shape, face.TextureFace)
		if !ok {
			continue // shapes that don't define this face have no texture to sample
		}

		clipped := clipFaceToFrustum(face, vp)
		if clipped == nil {
			continue
		}

		// Project clipped vertices to screen space. Depth and UVs are
		// premultiplied by 1/w for perspective-correct interpolation in the
		// fill loop (w is 1 under orthographic cameras).
		n := len(clipped)
		sx := make([]float32, n)
		sy := make([]float32, n)
		szi := make([]float32, n)
		siw := make([]float32, n)
		for i, cv := range clipped {
			invW := 1.0 / cv.clipPos.W
			ndcX := cv.clipPos.X * invW
			ndcY := cv.clipPos.Y * invW
			sx[i] = (ndcX + 1) * 0.5 * fw
			sy[i] = (1 - ndcY) * 0.5 * fh
			szi[i] = camera.CalculateDepth(cv.worldPos) * invW
			siw[i] = invW
		}

		if n < 3 {
			continue
		}

		// Backface culling (skipped for double-sided shapes).
		if !shapeDoubleSided(face.Shape) {
			windingFlipped := negativeStretchCount(face.Shape)%2 == 1
			signedArea := (sx[1]-sx[0])*(sy[2]-sy[0]) - (sx[2]-sx[0])*(sy[1]-sy[0])
			backfacing := signedArea > 0
			if windingFlipped {
				backfacing = signedArea < 0
			}
			if backfacing {
				continue
			}
		}

		faceW, faceH := faceUVDims(face.Shape, face.TextureFace)
		ox := float32(layout.Offset.X)
		oy := float32(layout.Offset.Y)
		muU := faceW
		if layout.Mirror.X {
			muU = -faceW
		}
		muV := faceH
		if layout.Mirror.Y {
			muV = -faceH
		}
		angle := int(layout.Angle)

		var lut *[256]uint8
		if cfg.Light.Enabled {
			lut = lightLUTForNormal(clipped[0].normal, cfg.Light, lightCache)
		}

		// Triangle fan: (v0, vi, vi+1).
		for i := 1; i < n-1; i++ {
			a, b, c := 0, i, i+1
			t := projectedTri{
				x:     [3]float32{sx[a], sx[b], sx[c]},
				y:     [3]float32{sy[a], sy[b], sy[c]},
				zi:    [3]float32{szi[a], szi[b], szi[c]},
				iw:    [3]float32{siw[a], siw[b], siw[c]},
				u:     [3]float32{clipped[a].u * siw[a], clipped[b].u * siw[b], clipped[c].u * siw[c]},
				v:     [3]float32{clipped[a].v * siw[a], clipped[b].v * siw[b], clipped[c].v * siw[c]},
				ox:    ox,
				oy:    oy,
				muU:   muU,
				muV:   muV,
				angle: angle,
				lut:   lut,
			}
			t.minY, t.maxY = triRowBounds(t.y, height)
			tris = append(tris, t)
		}
	}
	return tris
}

func triRowBounds(y [3]float32, height int) (int, int) {
	minY := int(maxF(minF(minF(y[0], y[1]), y[2]), 0))
	maxY := int(minF(maxF(maxF(y[0], y[1]), y[2]), float32(height-1)))
	return minY, maxY
}

// lightLUTForNormal returns a cached gamma-shade LUT for a face normal, building
// one only for each distinct lighting factor.
func lightLUTForNormal(normal Vec3, light LightConfig, cache map[float32]*[256]uint8) *[256]uint8 {
	nDotL := normal.Dot(light.Direction)
	if nDotL < 0 {
		nDotL = 0
	}
	lighting := light.Ambient + light.Diffuse*nDotL
	if lighting > 1 {
		lighting = 1
	}
	// Quantize so near-identical normals share a LUT.
	key := float32(int(lighting*1024)) / 1024
	if lut, ok := cache[key]; ok {
		return lut
	}
	lut := buildLightLUT(key)
	cache[key] = lut
	return lut
}
