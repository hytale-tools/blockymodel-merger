package blockymodel

import (
	"encoding/json"
	"fmt"
	"os"
)

// Load reads a blockymodel file from disk
func Load(path string) (*BlockyModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	var model BlockyModel
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %s: %w", path, err)
	}

	return &model, nil
}

// Save writes a blockymodel to disk with pretty formatting
func Save(model *BlockyModel, path string) error {
	data, err := json.MarshalIndent(model, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal model: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	return nil
}

// Clone creates a deep copy of a BlockyModel
func Clone(model *BlockyModel) (*BlockyModel, error) {
	cloned := &BlockyModel{
		LOD: model.LOD,
	}
	if len(model.Nodes) > 0 {
		cloned.Nodes = make([]Node, len(model.Nodes))
		for i := range model.Nodes {
			cloned.Nodes[i] = cloneNode(model.Nodes[i])
		}
	}
	return cloned, nil
}

// CloneNode creates a deep copy of a single Node
func CloneNode(node *Node) (*Node, error) {
	cloned := cloneNode(*node)
	return &cloned, nil
}

func cloneNode(node Node) Node {
	cloned := Node{
		ID:   node.ID,
		Name: node.Name,
	}
	if node.Position != nil {
		p := *node.Position
		cloned.Position = &p
	}
	if node.Orientation != nil {
		o := *node.Orientation
		cloned.Orientation = &o
	}
	if node.Shape != nil {
		cloned.Shape = cloneShape(node.Shape)
	}
	if len(node.Children) > 0 {
		cloned.Children = make([]Node, len(node.Children))
		for i := range node.Children {
			cloned.Children[i] = cloneNode(node.Children[i])
		}
	}
	return cloned
}

func cloneShape(shape *Shape) *Shape {
	cloned := &Shape{
		Type:        shape.Type,
		UnwrapMode:  shape.UnwrapMode,
		Visible:     shape.Visible,
		DoubleSided: shape.DoubleSided,
		ShadingMode: shape.ShadingMode,
	}
	if shape.Offset != nil {
		o := *shape.Offset
		cloned.Offset = &o
	}
	if shape.Stretch != nil {
		s := *shape.Stretch
		cloned.Stretch = &s
	}
	if shape.Visible != nil {
		v := *shape.Visible
		cloned.Visible = &v
	}
	if shape.DoubleSided != nil {
		d := *shape.DoubleSided
		cloned.DoubleSided = &d
	}
	if shape.Settings != nil {
		cloned.Settings = cloneMap(shape.Settings)
	}
	if shape.TextureLayout != nil {
		cloned.TextureLayout = make(map[string]TextureFace, len(shape.TextureLayout))
		for k, v := range shape.TextureLayout {
			cloned.TextureLayout[k] = v
		}
	}
	return cloned
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			cloned[k] = cloneMap(val)
		case []interface{}:
			cloned[k] = cloneSlice(val)
		default:
			cloned[k] = v
		}
	}
	return cloned
}

func cloneSlice(s []interface{}) []interface{} {
	cloned := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]interface{}:
			cloned[i] = cloneMap(val)
		case []interface{}:
			cloned[i] = cloneSlice(val)
		default:
			cloned[i] = v
		}
	}
	return cloned
}
