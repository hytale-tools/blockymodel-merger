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

// Clone creates a deep copy of a BlockyModel via JSON marshal/unmarshal
func Clone(model *BlockyModel) (*BlockyModel, error) {
	data, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal model for cloning: %w", err)
	}

	var cloned BlockyModel
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, fmt.Errorf("failed to unmarshal model for cloning: %w", err)
	}

	return &cloned, nil
}

// CloneNode creates a deep copy of a single Node via JSON marshal/unmarshal
func CloneNode(node *Node) (*Node, error) {
	data, err := json.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal node for cloning: %w", err)
	}

	var cloned Node
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, fmt.Errorf("failed to unmarshal node for cloning: %w", err)
	}

	return &cloned, nil
}
