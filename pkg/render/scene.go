package render

import "github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"

// Flatten walks the merged model tree and returns all renderable faces in world
// space.
//
// The transform composition mirrors the GLB exporter (pkg/export/glb.go) so the
// rendered geometry occupies the same coordinate space as the Blockbench/GLB
// output, just in raw blockymodel units (no 1/16 scale). Each node's bone
// transform is T(parentShapeOffset + position) * R(orientation); a node's own
// shape offset is then applied to its mesh and passed to its children as their
// parentOffset. Game-specific joint spacing is intentionally not applied (the
// GLB path does not apply it either).
func Flatten(model *blockymodel.BlockyModel) []Face {
	var faces []Face
	for i := range model.Nodes {
		flattenNode(&model.Nodes[i], Identity(), Vec3{}, &faces)
	}
	return faces
}

func flattenNode(node *blockymodel.Node, parentTransform Mat4, parentOffset Vec3, faces *[]Face) {
	pos := Vec3{}
	if node.Position != nil {
		pos = Vec3{float32(node.Position.X), float32(node.Position.Y), float32(node.Position.Z)}
	}

	boneLocal := MatFromTranslation(parentOffset.Add(pos))
	if node.Orientation != nil {
		q := Quat{
			X: float32(node.Orientation.X),
			Y: float32(node.Orientation.Y),
			Z: float32(node.Orientation.Z),
			W: float32(node.Orientation.W),
		}
		boneLocal = boneLocal.Mul(MatFromQuat(q))
	}
	boneWorld := parentTransform.Mul(boneLocal)

	childOffset := Vec3{}
	if node.Shape != nil {
		childOffset = shapeOffset(node.Shape)
		if node.Shape.Type == "box" || node.Shape.Type == "quad" {
			*faces = append(*faces, generateGeometry(node.Shape, boneWorld)...)
		}
	}

	for i := range node.Children {
		flattenNode(&node.Children[i], boneWorld, childOffset, faces)
	}
}
