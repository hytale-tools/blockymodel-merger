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
