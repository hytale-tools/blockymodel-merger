package merger

import (
	"math"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
)

// frame is a rigid transform (rotation q as x,y,z,w, then translation t) in
// model pixel units.
type frame struct {
	t [3]float64
	q [4]float64
}

var identityFrame = frame{q: [4]float64{0, 0, 0, 1}}

// frameEpsilon is how close two frames must be to count as the same place.
const frameEpsilon = 1e-4

// localFrame is a node's transform relative to its parent's child frame.
func localFrame(n *blockymodel.Node) frame {
	f := identityFrame
	if n.Position != nil {
		f.t = [3]float64{n.Position.X, n.Position.Y, n.Position.Z}
	}
	if n.Orientation != nil {
		f.q = normalize([4]float64{n.Orientation.X, n.Orientation.Y, n.Orientation.Z, n.Orientation.W})
	}
	return f
}

// offsetFrame is the translation by a node's shape offset: child positions
// are relative to the parent's shape offset, not its origin.
func offsetFrame(n *blockymodel.Node) frame {
	f := identityFrame
	if n.Shape != nil && n.Shape.Offset != nil {
		f.t = [3]float64{n.Shape.Offset.X, n.Shape.Offset.Y, n.Shape.Offset.Z}
	}
	return f
}

// childFrame is the frame a node's children are positioned in, relative to
// its parent's child frame.
func childFrame(n *blockymodel.Node) frame {
	return localFrame(n).mul(offsetFrame(n))
}

// setOn writes f as n's position and orientation.
func (f frame) setOn(n *blockymodel.Node) {
	n.Position = &blockymodel.Vec3{X: f.t[0], Y: f.t[1], Z: f.t[2]}
	n.Orientation = &blockymodel.Quaternion{X: f.q[0], Y: f.q[1], Z: f.q[2], W: f.q[3]}
}

// mul composes a then b: b is expressed in a's frame.
func (a frame) mul(b frame) frame {
	r := rotate(a.q, b.t)
	return frame{
		t: [3]float64{a.t[0] + r[0], a.t[1] + r[1], a.t[2] + r[2]},
		q: normalize(quatMul(a.q, b.q)),
	}
}

func (a frame) inverse() frame {
	inv := [4]float64{-a.q[0], -a.q[1], -a.q[2], a.q[3]}
	t := rotate(inv, a.t)
	return frame{t: [3]float64{-t[0], -t[1], -t[2]}, q: inv}
}

func (a frame) isIdentity() bool {
	// q and -q are the same rotation.
	return math.Abs(a.t[0]) < frameEpsilon && math.Abs(a.t[1]) < frameEpsilon &&
		math.Abs(a.t[2]) < frameEpsilon && math.Abs(math.Abs(a.q[3])-1) < frameEpsilon
}

func quatMul(a, b [4]float64) [4]float64 {
	return [4]float64{
		a[3]*b[0] + a[0]*b[3] + a[1]*b[2] - a[2]*b[1],
		a[3]*b[1] - a[0]*b[2] + a[1]*b[3] + a[2]*b[0],
		a[3]*b[2] + a[0]*b[1] - a[1]*b[0] + a[2]*b[3],
		a[3]*b[3] - a[0]*b[0] - a[1]*b[1] - a[2]*b[2],
	}
}

func rotate(q [4]float64, v [3]float64) [3]float64 {
	p := quatMul(quatMul(q, [4]float64{v[0], v[1], v[2], 0}), [4]float64{-q[0], -q[1], -q[2], q[3]})
	return [3]float64{p[0], p[1], p[2]}
}

func normalize(q [4]float64) [4]float64 {
	n := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
	if n == 0 {
		return identityFrame.q
	}
	return [4]float64{q[0] / n, q[1] / n, q[2] / n, q[3] / n}
}
