package blockymodel

// BlockyModel represents the root structure of a .blockymodel file
type BlockyModel struct {
	LOD   string `json:"lod,omitempty"`
	Nodes []Node `json:"nodes"`
}

// Node represents a node in the model hierarchy
type Node struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Position    *Vec3       `json:"position,omitempty"`
	Orientation *Quaternion `json:"orientation,omitempty"`
	Shape       *Shape      `json:"shape,omitempty"`
	Children    []Node      `json:"children"`
}

// Vec3 represents a 3D vector
type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// Quaternion represents a rotation
type Quaternion struct {
	W float64 `json:"w"`
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// Shape defines the geometry of a node
type Shape struct {
	Type          string                   `json:"type,omitempty"`
	Offset        *Vec3                    `json:"offset,omitempty"`
	Stretch       *Vec3                    `json:"stretch,omitempty"`
	Settings      map[string]interface{}   `json:"settings,omitempty"`
	TextureLayout map[string]TextureFace   `json:"textureLayout,omitempty"`
	UnwrapMode    string                   `json:"unwrapMode,omitempty"`
	Visible       *bool                    `json:"visible,omitempty"`
	DoubleSided   *bool                    `json:"doubleSided,omitempty"`
	ShadingMode   string                   `json:"shadingMode,omitempty"`
}

// TextureFace defines texture mapping for a single face
type TextureFace struct {
	Offset Vec2     `json:"offset"`
	Mirror Vec2Bool `json:"mirror"`
	Angle  float64  `json:"angle"`
}

// Vec2 represents a 2D vector
type Vec2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Vec2Bool represents a 2D boolean vector for mirroring
type Vec2Bool struct {
	X bool `json:"x"`
	Y bool `json:"y"`
}

// IsSkeletonReference returns true if this node is a skeleton reference point
// (used by accessories to attach to the base model)
func (n *Node) IsSkeletonReference() bool {
	if n.Shape == nil {
		return false
	}
	if n.Shape.Type != "none" {
		return false
	}
	if n.Shape.Settings == nil {
		return false
	}
	isPiece, ok := n.Shape.Settings["isPiece"]
	if !ok {
		return false
	}
	if b, ok := isPiece.(bool); ok {
		return b
	}
	return false
}

// HasGeometry returns true if this node has visible geometry
func (n *Node) HasGeometry() bool {
	if n.Shape == nil {
		return false
	}
	return n.Shape.Type == "box" || n.Shape.Type == "sphere" || n.Shape.Type == "cylinder" || n.Shape.Type == "quad"
}

// AtlasOffset represents an offset to apply to texture coordinates
type AtlasOffset struct {
	X float64
	Y float64
}

// UpdateTextureOffsets updates all textureLayout offsets for nodes matching the given IDs
// by adding the atlas offset to their existing offsets
func UpdateTextureOffsets(nodes []Node, nodeIDs map[string]bool, offset AtlasOffset) {
	for i := range nodes {
		node := &nodes[i]
		if nodeIDs[node.ID] && node.Shape != nil && node.Shape.TextureLayout != nil {
			for faceName, face := range node.Shape.TextureLayout {
				face.Offset.X += offset.X
				face.Offset.Y += offset.Y
				node.Shape.TextureLayout[faceName] = face
			}
		}
		// Recurse into children
		UpdateTextureOffsets(node.Children, nodeIDs, offset)
	}
}

// HeldItemNodeName is the group node every held item is wrapped in. It marks
// the boundary between the character and what it is carrying.
const HeldItemNodeName = "HeldItem"

// HeldItemNodePrefix namespaces every node inside a held item. Item models
// bring their own authoring rig, and those bone names collide with the
// character's - a severed-head item model has its own Head, Chest, Belly and
// arm bones, i.e. most of what a carry animation drives. Consumers that bind
// animation tracks to nodes by name (the GLB exporter's output ends up in
// exactly such players) would otherwise drive the item's internals as if they
// were the character's skeleton. The prefix makes that impossible without the
// consumer having to know about the HeldItem boundary at all.
const HeldItemNodePrefix = "HeldItem."

// HideSubtrees strips the geometry from every node named in names and from
// everything below it, and reports the names that matched nothing.
//
// The nodes themselves are kept, with their positions, orientations and shape
// offsets intact: hiding a bone must not move what hangs off it (hiding "Head"
// still leaves any head-parented attachment bone where it was). Only the shape
// type is cleared, which is what the renderer and the GLB exporter key on to
// emit a mesh.
//
// The held-item subtree is never touched: an item model carries its own
// authoring rig, whose bone names collide with the character's (a severed-head
// item model is rooted at a node called "Head"), so hiding a body part must
// not reach into the thing the character is holding.
func HideSubtrees(nodes []Node, names []string) []string {
	matched := make(map[string]bool, len(names))
	wanted := make(map[string]bool, len(names))
	for _, n := range names {
		wanted[n] = true
	}
	hideNodes(nodes, wanted, matched, false)

	var unmatched []string
	for _, n := range names {
		if !matched[n] {
			unmatched = append(unmatched, n)
		}
	}
	return unmatched
}

func hideNodes(nodes []Node, wanted, matched map[string]bool, inside bool) {
	for i := range nodes {
		node := &nodes[i]
		if node.Name == HeldItemNodeName {
			continue
		}
		hide := inside
		if wanted[node.Name] {
			matched[node.Name] = true
			hide = true
		}
		if hide && node.Shape != nil {
			node.Shape.Type = "none"
			node.Shape.TextureLayout = nil
		}
		hideNodes(node.Children, wanted, matched, hide)
	}
}
