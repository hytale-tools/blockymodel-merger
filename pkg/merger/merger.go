package merger

import (
	"fmt"
	"strconv"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
)

// Merger handles merging accessories into a base model
type Merger struct {
	base   *blockymodel.BlockyModel
	nextID int
	// Track which accessory each merged node ID came from
	NodeSources map[string]string // node ID -> accessory ID
}

// New creates a new Merger with the given base model (deep copied)
func New(base *blockymodel.BlockyModel) (*Merger, error) {
	cloned, err := blockymodel.Clone(base)
	if err != nil {
		return nil, fmt.Errorf("failed to clone base model: %w", err)
	}

	// Find the highest existing ID to use for new nodes
	maxID := findMaxID(cloned.Nodes)

	return &Merger{
		base:        cloned,
		nextID:      maxID + 1,
		NodeSources: make(map[string]string),
	}, nil
}

// Merge integrates an accessory into the base model
// accessoryID is used to track which accessory each merged node came from (for texture offset updates)
func (m *Merger) Merge(accessory *blockymodel.BlockyModel, accessoryID string) error {
	for i := range accessory.Nodes {
		if err := m.mergeNode(&accessory.Nodes[i], accessoryID); err != nil {
			return err
		}
	}
	return nil
}

// Result returns the merged model
func (m *Merger) Result() *blockymodel.BlockyModel {
	return m.base
}

// mergeNode processes a single accessory node
func (m *Merger) mergeNode(accessoryNode *blockymodel.Node, accessoryID string) error {
	// Check if this node matches a bone in the base model (either skeleton ref or by name)
	baseNode := findNodeByName(m.base.Nodes, accessoryNode.Name)

	if accessoryNode.IsSkeletonReference() || (baseNode != nil && accessoryNode.Shape != nil && accessoryNode.Shape.Type == "none") {
		// This is an attachment point - attach children to base model
		if baseNode == nil {
			fmt.Printf("Warning: no matching attachment point '%s' found in base model\n", accessoryNode.Name)
			return nil
		}

		// Copy non-skeleton children (geometry) to the base node
		for i := range accessoryNode.Children {
			child := &accessoryNode.Children[i]
			if !child.IsSkeletonReference() && !m.isAttachmentPoint(child) {
				// Clone and re-ID the child, then append to base
				cloned, err := blockymodel.CloneNode(child)
				if err != nil {
					return fmt.Errorf("failed to clone node %s: %w", child.Name, err)
				}
				m.reIDNode(cloned, accessoryID)
				baseNode.Children = append(baseNode.Children, *cloned)
			} else {
				// Recurse into skeleton reference children
				if err := m.mergeNode(child, accessoryID); err != nil {
					return err
				}
			}
		}
	} else {
		// Non-skeleton reference nodes at top level: recursively process children
		for i := range accessoryNode.Children {
			if err := m.mergeNode(&accessoryNode.Children[i], accessoryID); err != nil {
				return err
			}
		}
	}
	return nil
}

// isAttachmentPoint checks if a node is an attachment point (bone reference)
func (m *Merger) isAttachmentPoint(node *blockymodel.Node) bool {
	if node.IsSkeletonReference() {
		return true
	}
	// Also check if it matches a bone name and has no geometry
	if node.Shape != nil && node.Shape.Type == "none" {
		baseNode := findNodeByName(m.base.Nodes, node.Name)
		return baseNode != nil
	}
	return false
}

// reIDNode assigns new unique IDs to a node and all its children, tracking accessory source
func (m *Merger) reIDNode(node *blockymodel.Node, accessoryID string) {
	node.ID = strconv.Itoa(m.nextID)
	m.NodeSources[node.ID] = accessoryID
	m.nextID++
	for i := range node.Children {
		m.reIDNode(&node.Children[i], accessoryID)
	}
}

// findNodeByName recursively searches for a node with the given name
func findNodeByName(nodes []blockymodel.Node, name string) *blockymodel.Node {
	for i := range nodes {
		if nodes[i].Name == name {
			return &nodes[i]
		}
		if found := findNodeByName(nodes[i].Children, name); found != nil {
			return found
		}
	}
	return nil
}

// findMaxID recursively finds the maximum numeric ID in the node tree
func findMaxID(nodes []blockymodel.Node) int {
	maxID := 0
	for i := range nodes {
		if id, err := strconv.Atoi(nodes[i].ID); err == nil {
			if id > maxID {
				maxID = id
			}
		}
		if childMax := findMaxID(nodes[i].Children); childMax > maxID {
			maxID = childMax
		}
	}
	return maxID
}
