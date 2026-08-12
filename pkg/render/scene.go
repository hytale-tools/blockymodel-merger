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

// FlattenSubtree flattens only the geometry inside the named node's subtree,
// keeping world transforms from the full hierarchy. Useful for framing a
// camera on one body part (e.g. "Head") without rendering it in isolation.
func FlattenSubtree(model *blockymodel.BlockyModel, rootName string) []Face {
	var faces []Face
	for i := range model.Nodes {
		flattenNodeFiltered(&model.Nodes[i], Identity(), Vec3{}, &faces, rootName, false, false)
	}
	return faces
}

// HeldItemNodeName is the node name the auto-fit framing excludes: anything
// attached to the character as a held item must be wrapped in a node with
// this name so full-body framing stays identical with and without it.
const HeldItemNodeName = blockymodel.HeldItemNodeName

// FlattenExcluding flattens all geometry except the named node's subtree.
// Useful for framing a camera on the character while ignoring attachments
// (e.g. a held item).
func FlattenExcluding(model *blockymodel.BlockyModel, excludeName string) []Face {
	var faces []Face
	for i := range model.Nodes {
		flattenNodeFiltered(&model.Nodes[i], Identity(), Vec3{}, &faces, excludeName, false, true)
	}
	return faces
}

func flattenNodeFiltered(node *blockymodel.Node, parentTransform Mat4, parentOffset Vec3, faces *[]Face, rootName string, inside, exclude bool) {
	inside = inside || node.Name == rootName
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
		if inside != exclude && (node.Shape.Type == "box" || node.Shape.Type == "quad") {
			*faces = append(*faces, generateGeometry(node.Shape, boneWorld)...)
		}
	}

	for i := range node.Children {
		flattenNodeFiltered(&node.Children[i], boneWorld, childOffset, faces, rootName, inside, exclude)
	}
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
