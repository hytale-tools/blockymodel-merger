package render

// Sutherland-Hodgman frustum clipping in clip space. Clipping the near plane
// first guarantees w > 0 for all subsequent operations.

type clipVertex struct {
	worldPos Vec3
	u, v     float32
	clipPos  Vec4
	normal   Vec3
}

type clipPlane int

const (
	planeNear clipPlane = iota
	planeLeft
	planeRight
	planeBottom
	planeTop
	planeFar
)

var allPlanes = [6]clipPlane{planeNear, planeLeft, planeRight, planeBottom, planeTop, planeFar}

// clipFaceToFrustum transforms the face to clip space and clips against all six
// planes. Returns nil if fully outside.
func clipFaceToFrustum(face Face, vp Mat4) []clipVertex {
	verts := make([]clipVertex, len(face.Vertices))
	for i, v := range face.Vertices {
		clip := vp.MulVec4(Vec4{v.Pos.X, v.Pos.Y, v.Pos.Z, 1})
		verts[i] = clipVertex{worldPos: v.Pos, u: v.U, v: v.V, clipPos: clip, normal: v.Normal}
	}

	if triviallyRejected(verts) {
		return nil
	}

	for _, plane := range allPlanes {
		verts = sutherlandHodgman(verts, plane)
		if len(verts) < 3 {
			return nil
		}
	}
	return verts
}

func triviallyRejected(verts []clipVertex) bool {
	for _, plane := range allPlanes {
		allOutside := true
		for i := range verts {
			if isInside(verts[i], plane) {
				allOutside = false
				break
			}
		}
		if allOutside {
			return true
		}
	}
	return false
}

func sutherlandHodgman(verts []clipVertex, plane clipPlane) []clipVertex {
	if len(verts) < 3 {
		return nil
	}
	out := make([]clipVertex, 0, len(verts)+1)
	n := len(verts)
	for i := 0; i < n; i++ {
		current := verts[i]
		next := verts[(i+1)%n]
		curIn := isInside(current, plane)
		nextIn := isInside(next, plane)

		switch {
		case curIn && nextIn:
			out = append(out, next)
		case curIn && !nextIn:
			out = append(out, intersect(current, next, plane))
		case !curIn && nextIn:
			out = append(out, intersect(current, next, plane))
			out = append(out, next)
		}
	}
	return out
}

func isInside(v clipVertex, plane clipPlane) bool {
	return signedDistance(v.clipPos, plane) >= 0
}

func intersect(v1, v2 clipVertex, plane clipPlane) clipVertex {
	d1 := signedDistance(v1.clipPos, plane)
	d2 := signedDistance(v2.clipPos, plane)
	denom := d1 - d2
	var t float32
	if denom < 1e-10 && denom > -1e-10 {
		t = 0.5
	} else {
		t = clamp32(d1/denom, 0, 1)
	}
	return clipVertex{
		worldPos: v1.worldPos.Lerp(v2.worldPos, t),
		u:        v1.u + t*(v2.u-v1.u),
		v:        v1.v + t*(v2.v-v1.v),
		clipPos:  v1.clipPos.Lerp(v2.clipPos, t),
		normal:   v1.normal.Lerp(v2.normal, t).Normalize(),
	}
}

func signedDistance(c Vec4, plane clipPlane) float32 {
	switch plane {
	case planeLeft:
		return c.X + c.W
	case planeRight:
		return c.W - c.X
	case planeBottom:
		return c.Y + c.W
	case planeTop:
		return c.W - c.Y
	case planeNear:
		return c.Z + c.W
	case planeFar:
		return c.W - c.Z
	}
	return 0
}
