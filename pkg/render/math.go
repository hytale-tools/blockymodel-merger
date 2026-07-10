package render

import "math"

// Minimal linear-algebra types for the software rasterizer. The matrix math uses
// right-handed, column-major conventions (same as common GPU/glTF math libraries)
// so projection and clipping behave the way the rest of the pipeline expects.

// Vec3 is a 3-component vector.
type Vec3 struct{ X, Y, Z float32 }

// Vec4 is a 4-component vector (homogeneous coordinate).
type Vec4 struct{ X, Y, Z, W float32 }

func (a Vec3) Add(b Vec3) Vec3      { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Sub(b Vec3) Vec3      { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Scale(s float32) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func (a Vec3) Dot(b Vec3) float32   { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{
		a.Y*b.Z - a.Z*b.Y,
		a.Z*b.X - a.X*b.Z,
		a.X*b.Y - a.Y*b.X,
	}
}

func (a Vec3) Length() float32 { return float32(math.Sqrt(float64(a.Dot(a)))) }

func (a Vec3) Normalize() Vec3 {
	l := a.Length()
	if l == 0 {
		return a
	}
	return a.Scale(1.0 / l)
}

// Lerp linearly interpolates between a and b by t.
func (a Vec3) Lerp(b Vec3, t float32) Vec3 { return a.Add(b.Sub(a).Scale(t)) }

func (a Vec4) Lerp(b Vec4, t float32) Vec4 {
	return Vec4{
		a.X + (b.X-a.X)*t,
		a.Y + (b.Y-a.Y)*t,
		a.Z + (b.Z-a.Z)*t,
		a.W + (b.W-a.W)*t,
	}
}

// Quat is a quaternion (x, y, z, w).
type Quat struct{ X, Y, Z, W float32 }

func (q Quat) Normalize() Quat {
	l := float32(math.Sqrt(float64(q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W)))
	if l == 0 {
		return Quat{0, 0, 0, 1}
	}
	return Quat{q.X / l, q.Y / l, q.Z / l, q.W / l}
}

// Mat4 is a 4x4 matrix stored column-major to match glam:
// element (row r, col c) lives at index c*4+r.
type Mat4 [16]float32

// Identity returns the identity matrix.
func Identity() Mat4 {
	return Mat4{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

// Mul returns a * b (column-major: result column j = a * b's column j).
func (a Mat4) Mul(b Mat4) Mat4 {
	var out Mat4
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += a[k*4+row] * b[col*4+k]
			}
			out[col*4+row] = sum
		}
	}
	return out
}

// MulVec4 returns m * v.
func (m Mat4) MulVec4(v Vec4) Vec4 {
	return Vec4{
		m[0]*v.X + m[4]*v.Y + m[8]*v.Z + m[12]*v.W,
		m[1]*v.X + m[5]*v.Y + m[9]*v.Z + m[13]*v.W,
		m[2]*v.X + m[6]*v.Y + m[10]*v.Z + m[14]*v.W,
		m[3]*v.X + m[7]*v.Y + m[11]*v.Z + m[15]*v.W,
	}
}

// TransformPoint3 applies the affine transform to a point (w = 1, no perspective divide).
func (m Mat4) TransformPoint3(p Vec3) Vec3 {
	r := m.MulVec4(Vec4{p.X, p.Y, p.Z, 1})
	return Vec3{r.X, r.Y, r.Z}
}

// TransformVector3 applies only the linear part (no translation) to a direction.
func (m Mat4) TransformVector3(v Vec3) Vec3 {
	return Vec3{
		m[0]*v.X + m[4]*v.Y + m[8]*v.Z,
		m[1]*v.X + m[5]*v.Y + m[9]*v.Z,
		m[2]*v.X + m[6]*v.Y + m[10]*v.Z,
	}
}

// MatFromTranslation builds a translation matrix.
func MatFromTranslation(t Vec3) Mat4 {
	m := Identity()
	m[12] = t.X
	m[13] = t.Y
	m[14] = t.Z
	return m
}

// MatFromScale builds a (possibly mirroring) scale matrix.
func MatFromScale(s Vec3) Mat4 {
	m := Identity()
	m[0] = s.X
	m[5] = s.Y
	m[10] = s.Z
	return m
}

// MatFromQuat builds a rotation matrix from a quaternion (matching glam::Mat4::from_quat).
func MatFromQuat(q Quat) Mat4 {
	q = q.Normalize()
	x, y, z, w := q.X, q.Y, q.Z, q.W
	x2, y2, z2 := x+x, y+y, z+z
	xx, xy, xz := x*x2, x*y2, x*z2
	yy, yz, zz := y*y2, y*z2, z*z2
	wx, wy, wz := w*x2, w*y2, w*z2

	return Mat4{
		// column 0
		1 - (yy + zz), xy + wz, xz - wy, 0,
		// column 1
		xy - wz, 1 - (xx + zz), yz + wx, 0,
		// column 2
		xz + wy, yz - wx, 1 - (xx + yy), 0,
		// column 3
		0, 0, 0, 1,
	}
}

// LookAtRH matches glam::Mat4::look_at_rh.
func LookAtRH(eye, center, up Vec3) Mat4 {
	f := center.Sub(eye).Normalize()
	s := f.Cross(up).Normalize()
	u := s.Cross(f)

	return Mat4{
		// column 0
		s.X, u.X, -f.X, 0,
		// column 1
		s.Y, u.Y, -f.Y, 0,
		// column 2
		s.Z, u.Z, -f.Z, 0,
		// column 3
		-s.Dot(eye), -u.Dot(eye), f.Dot(eye), 1,
	}
}

// OrthographicRH matches glam::Mat4::orthographic_rh (depth range [0, 1]).
func OrthographicRH(left, right, bottom, top, near, far float32) Mat4 {
	rcpWidth := 1.0 / (right - left)
	rcpHeight := 1.0 / (top - bottom)
	r := 1.0 / (near - far)

	return Mat4{
		// column 0
		2 * rcpWidth, 0, 0, 0,
		// column 1
		0, 2 * rcpHeight, 0, 0,
		// column 2
		0, 0, r, 0,
		// column 3
		-(left + right) * rcpWidth, -(top + bottom) * rcpHeight, r * near, 1,
	}
}

// PerspectiveRH matches glam::Mat4::perspective_rh (depth range [0, 1]).
func PerspectiveRH(fovYRadians, aspect, near, far float32) Mat4 {
	sinFov := float32(math.Sin(float64(0.5 * fovYRadians)))
	cosFov := float32(math.Cos(float64(0.5 * fovYRadians)))
	h := cosFov / sinFov
	w := h / aspect
	r := far / (near - far)

	return Mat4{
		// column 0
		w, 0, 0, 0,
		// column 1
		0, h, 0, 0,
		// column 2
		0, 0, r, -1,
		// column 3
		0, 0, r * near, 0,
	}
}
