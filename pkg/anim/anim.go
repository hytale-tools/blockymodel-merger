// Package anim loads Hytale .blockyanim files and applies them to a
// blockymodel as a static pose.
//
// A .blockyanim is plain JSON: nodeAnimations maps bone names to keyframe
// tracks whose values are deltas relative to the model's bind pose.
package anim

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
)

// Animation is a parsed .blockyanim file.
type Animation struct {
	Duration       float64               `json:"duration"`
	NodeAnimations map[string]NodeTracks `json:"nodeAnimations"`
}

// NodeTracks holds the keyframe tracks for a single bone.
type NodeTracks struct {
	Position    []PositionKey    `json:"position"`
	Orientation []OrientationKey `json:"orientation"`
}

// PositionKey is a positional keyframe (delta from bind pose).
type PositionKey struct {
	Time  float64          `json:"time"`
	Delta blockymodel.Vec3 `json:"delta"`
}

// OrientationKey is a rotational keyframe (delta from bind pose).
type OrientationKey struct {
	Time  float64                `json:"time"`
	Delta blockymodel.Quaternion `json:"delta"`
}

// Load reads a .blockyanim file.
func Load(path string) (*Animation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read animation %s: %w", path, err)
	}
	var a Animation
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("failed to parse animation %s: %w", path, err)
	}
	return &a, nil
}

// ApplyPose applies the animation's first keyframe of every track to the model
// as a static pose: orientation = bind ⊗ delta, position = bind + delta.
//
// Deltas are matched to nodes by name, and every node with a matching name is
// posed - merged accessories clone skeleton bone names (e.g. cape physics
// bones Cape1..Cape3), and those clones are animated in-game too.
func (a *Animation) ApplyPose(model *blockymodel.BlockyModel) {
	applyToNodes(model.Nodes, a.NodeAnimations)
}

func applyToNodes(nodes []blockymodel.Node, tracks map[string]NodeTracks) {
	for i := range nodes {
		n := &nodes[i]
		if t, ok := tracks[n.Name]; ok {
			if len(t.Orientation) > 0 {
				bind := blockymodel.Quaternion{W: 1}
				if n.Orientation != nil {
					bind = *n.Orientation
				}
				q := mul(bind, t.Orientation[0].Delta)
				n.Orientation = &q
			}
			if len(t.Position) > 0 {
				if n.Position == nil {
					n.Position = &blockymodel.Vec3{}
				}
				d := t.Position[0].Delta
				n.Position.X += d.X
				n.Position.Y += d.Y
				n.Position.Z += d.Z
			}
		}
		applyToNodes(n.Children, tracks)
	}
}

func mul(a, b blockymodel.Quaternion) blockymodel.Quaternion {
	return blockymodel.Quaternion{
		W: a.W*b.W - a.X*b.X - a.Y*b.Y - a.Z*b.Z,
		X: a.W*b.X + a.X*b.W + a.Y*b.Z - a.Z*b.Y,
		Y: a.W*b.Y - a.X*b.Z + a.Y*b.W + a.Z*b.X,
		Z: a.W*b.Z + a.X*b.Y - a.Y*b.X + a.Z*b.W,
	}
}
