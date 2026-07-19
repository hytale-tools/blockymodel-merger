package render

import "math"

// CameraProjection is implemented by both orthographic and perspective cameras.
type CameraProjection interface {
	ViewProjectionMatrix(outputWidth, outputHeight uint32) Mat4
	// CalculateDepth returns the view-space depth used for the z-buffer
	// (smaller = closer). Equals -viewSpaceZ in right-handed space.
	CalculateDepth(p Vec3) float32
}

// Camera is an orthographic camera.
type Camera struct {
	Position  Vec3
	Target    Vec3
	Up        Vec3
	OrthoSize float32
	Near      float32
	Far       float32
}

func (c Camera) ViewMatrix() Mat4 { return LookAtRH(c.Position, c.Target, c.Up) }

func (c Camera) ProjectionMatrix(outputWidth, outputHeight uint32) Mat4 {
	aspect := float32(outputWidth) / float32(outputHeight)
	halfWidth := c.OrthoSize * aspect / 2.0
	halfHeight := c.OrthoSize / 2.0
	return OrthographicRH(-halfWidth, halfWidth, -halfHeight, halfHeight, c.Near, c.Far)
}

func (c Camera) ViewProjectionMatrix(outputWidth, outputHeight uint32) Mat4 {
	return c.ProjectionMatrix(outputWidth, outputHeight).Mul(c.ViewMatrix())
}

func (c Camera) CalculateDepth(p Vec3) float32 {
	view := c.ViewMatrix().TransformPoint3(p)
	return -view.Z
}

// PerspectiveCamera is a perspective camera.
type PerspectiveCamera struct {
	Position Vec3
	Target   Vec3
	Up       Vec3
	FovYDeg  float32
	Near     float32
	Far      float32
}

func (c PerspectiveCamera) ViewMatrix() Mat4 { return LookAtRH(c.Position, c.Target, c.Up) }

func (c PerspectiveCamera) ProjectionMatrix(outputWidth, outputHeight uint32) Mat4 {
	aspect := float32(outputWidth) / float32(outputHeight)
	fovRad := c.FovYDeg * float32(math.Pi) / 180.0
	return PerspectiveRH(fovRad, aspect, c.Near, c.Far)
}

func (c PerspectiveCamera) ViewProjectionMatrix(outputWidth, outputHeight uint32) Mat4 {
	return c.ProjectionMatrix(outputWidth, outputHeight).Mul(c.ViewMatrix())
}

func (c PerspectiveCamera) CalculateDepth(p Vec3) float32 {
	view := c.ViewMatrix().TransformPoint3(p)
	return -view.Z
}

var defaultUp = Vec3{0, 1, 0}

// --- Orthographic presets (raw blockymodel units) ---

func CameraDefaultIsometric() Camera {
	return Camera{Position: Vec3{30, 30, 30}, Target: Vec3{0, 0, 0}, Up: defaultUp, OrthoSize: 60, Near: 0.1, Far: 1000}
}

func CameraFrontRight() Camera {
	return Camera{Position: Vec3{65, 75, 75}, Target: Vec3{0, 63.5, 0}, Up: defaultUp, OrthoSize: 140, Near: 0.1, Far: 1000}
}

func CameraBackRight() Camera {
	return Camera{Position: Vec3{65, 75, -75}, Target: Vec3{0, 63.5, 0}, Up: defaultUp, OrthoSize: 140, Near: 0.1, Far: 1000}
}

func CameraFrontLeft() Camera {
	return Camera{Position: Vec3{-65, 75, 75}, Target: Vec3{0, 63.5, 0}, Up: defaultUp, OrthoSize: 140, Near: 0.1, Far: 1000}
}

func CameraBackLeft() Camera {
	return Camera{Position: Vec3{-65, 75, -75}, Target: Vec3{0, 63.5, 0}, Up: defaultUp, OrthoSize: 140, Near: 0.1, Far: 1000}
}

func CameraHeadshot() Camera {
	return Camera{Position: Vec3{0, 100, 150}, Target: Vec3{0, 100, 0}, Up: defaultUp, OrthoSize: 30, Near: 0.0000001, Far: 1000}
}

func CameraIsometricHead() Camera {
	return Camera{Position: Vec3{-175, 175, 175}, Target: Vec3{0, 100, 0}, Up: defaultUp, OrthoSize: 90, Near: 0.1, Far: 1000}
}

func CameraFullBodyFront() Camera {
	return Camera{Position: Vec3{0, 63.5, 150}, Target: Vec3{0, 63.5, 0}, Up: defaultUp, OrthoSize: 130, Near: 0.1, Far: 1000}
}

func CameraPlayerBust() Camera {
	return Camera{Position: Vec3{0, 92, 85}, Target: Vec3{0, 94, 0}, Up: defaultUp, OrthoSize: 62, Near: 0.1, Far: 1000}
}

// --- Perspective presets ---

func PerspectiveHeadshot() PerspectiveCamera {
	return PerspectiveCamera{Position: Vec3{0, 107, 120}, Target: Vec3{0, 107, 0}, Up: defaultUp, FovYDeg: 21, Near: 0.1, Far: 1000}
}

func PerspectiveIsometricHead() PerspectiveCamera {
	return PerspectiveCamera{Position: Vec3{-80, 140, 80}, Target: Vec3{0, 100, 0}, Up: defaultUp, FovYDeg: 35, Near: 1, Far: 1000}
}

func PerspectivePlayerBust() PerspectiveCamera {
	return PerspectiveCamera{Position: Vec3{0, 92, 100}, Target: Vec3{0, 94, 0}, Up: defaultUp, FovYDeg: 40, Near: 1, Far: 1000}
}

// AutoFitPerspective builds a 30-degree perspective camera on the +Z axis
// aimed at the geometry's bounding-box center, at a distance that fits the
// largest dimension with a 1.25x margin.
func AutoFitPerspective(faces []Face) PerspectiveCamera {
	min := Vec3{float32(math.Inf(1)), float32(math.Inf(1)), float32(math.Inf(1))}
	max := Vec3{float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(-1))}
	for _, f := range faces {
		for _, v := range f.Vertices {
			min.X = float32(math.Min(float64(min.X), float64(v.Pos.X)))
			min.Y = float32(math.Min(float64(min.Y), float64(v.Pos.Y)))
			min.Z = float32(math.Min(float64(min.Z), float64(v.Pos.Z)))
			max.X = float32(math.Max(float64(max.X), float64(v.Pos.X)))
			max.Y = float32(math.Max(float64(max.Y), float64(v.Pos.Y)))
			max.Z = float32(math.Max(float64(max.Z), float64(v.Pos.Z)))
		}
	}
	// No vertices leaves the bounds at infinities; fall back to a finite
	// camera on a unit box at the origin (the render is empty either way).
	if !(min.X <= max.X && min.Y <= max.Y && min.Z <= max.Z) {
		min, max = Vec3{-0.5, -0.5, -0.5}, Vec3{0.5, 0.5, 0.5}
	}
	center := Vec3{(min.X + max.X) / 2, (min.Y + max.Y) / 2, (min.Z + max.Z) / 2}
	size := Vec3{max.X - min.X, max.Y - min.Y, max.Z - min.Z}
	maxDim := math.Max(float64(size.X), math.Max(float64(size.Y), float64(size.Z)))

	const fovY = 30.0
	dist := float32(maxDim / (2 * math.Tan(fovY/2*math.Pi/180)) * 1.25)
	return PerspectiveCamera{
		Position: Vec3{center.X, center.Y, center.Z + dist},
		Target:   center,
		Up:       defaultUp,
		FovYDeg:  fovY,
		Near:     dist / 100,
		Far:      dist * 10,
	}
}

// RotateFacesY rotates face vertices (and normals) around the world Y axis
// through the origin.
func RotateFacesY(faces []Face, deg float32) {
	if deg == 0 {
		return
	}
	rad := float64(deg) * math.Pi / 180
	sin, cos := float32(math.Sin(rad)), float32(math.Cos(rad))
	rot := func(v Vec3) Vec3 {
		return Vec3{v.X*cos + v.Z*sin, v.Y, -v.X*sin + v.Z*cos}
	}
	for i := range faces {
		for j := range faces[i].Vertices {
			v := &faces[i].Vertices[j]
			v.Pos = rot(v.Pos)
			v.Normal = rot(v.Normal)
		}
	}
}

// CameraForView returns a camera preset by name. ortho selects orthographic vs
// perspective where both exist.
func CameraForView(name string, perspective bool) (CameraProjection, bool) {
	switch name {
	case "isometric", "default":
		return CameraDefaultIsometric(), true
	case "front-right":
		return CameraFrontRight(), true
	case "back-right":
		return CameraBackRight(), true
	case "front-left":
		return CameraFrontLeft(), true
	case "back-left":
		return CameraBackLeft(), true
	case "full-body", "full-body-front":
		return CameraFullBodyFront(), true
	case "headshot":
		if perspective {
			return PerspectiveHeadshot(), true
		}
		return CameraHeadshot(), true
	case "iso-head", "isometric-head":
		if perspective {
			return PerspectiveIsometricHead(), true
		}
		return CameraIsometricHead(), true
	case "bust", "player-bust":
		if perspective {
			return PerspectivePlayerBust(), true
		}
		return CameraPlayerBust(), true
	}
	return nil, false
}
