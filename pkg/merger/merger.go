package merger

import (
	"fmt"
	"strconv"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
	"github.com/hytale-tools/blockymodel-merger/pkg/util"
)

// Merger handles merging accessories into a base model
type Merger struct {
	base      *blockymodel.BlockyModel
	nextID    int
	nodeIndex map[string]*blockymodel.Node // name -> node for O(1) lookups
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

	m := &Merger{
		base:        cloned,
		nextID:      maxID + 1,
		nodeIndex:   make(map[string]*blockymodel.Node),
		NodeSources: make(map[string]string),
	}
	m.buildNodeIndex(cloned.Nodes)
	return m, nil
}

// buildNodeIndex walks the node tree and indexes nodes by name
func (m *Merger) buildNodeIndex(nodes []blockymodel.Node) {
	for i := range nodes {
		m.nodeIndex[nodes[i].Name] = &nodes[i]
		m.buildNodeIndex(nodes[i].Children)
	}
}

// reindexChildren refreshes index pointers for all children of a node.
// Must be called after appending to node.Children, since append may
// reallocate the slice and invalidate pointers to existing siblings.
func (m *Merger) reindexChildren(node *blockymodel.Node) {
	for i := range node.Children {
		child := &node.Children[i]
		m.nodeIndex[child.Name] = child
		m.reindexChildren(child)
	}
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
	baseNode := m.nodeIndex[accessoryNode.Name]

	if accessoryNode.IsSkeletonReference() || (baseNode != nil && accessoryNode.Shape != nil && accessoryNode.Shape.Type == "none") {
		// This is an attachment point - attach children to base model
		if baseNode == nil {
			util.Logger.Warn("No matching attachment point found in base model",
				"node", accessoryNode.Name)
			return nil
		}

		// Copy non-skeleton children (geometry) to the base node
		// When IsPiece is true, recursively attach children to the matching base node.
		// The IsPiece node itself is NOT added - only its children are attached.
		// Children are cloned as-is with their positions unchanged.
		for i := range accessoryNode.Children {
			child := &accessoryNode.Children[i]
			if !child.IsSkeletonReference() && !m.isAttachmentPoint(child) {
				// Clone and re-ID the child, then append to base
				cloned, err := blockymodel.CloneNode(child)
				if err != nil {
					return fmt.Errorf("failed to clone node %s: %w", child.Name, err)
				}
				// Filter out skeleton ref nodes from cloned children - they should only be attachment points
				cloned.Children = m.filterSkeletonRefs(cloned.Children, accessoryID)
				m.reIDNode(cloned, accessoryID)
				baseNode.Children = append(baseNode.Children, *cloned)
				// Re-index all children - append may have reallocated the slice,
				// invalidating pointers to existing siblings
				m.reindexChildren(baseNode)
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

// filterSkeletonRefs removes skeleton reference nodes from children and processes their children instead
func (m *Merger) filterSkeletonRefs(children []blockymodel.Node, accessoryID string) []blockymodel.Node {
	var filtered []blockymodel.Node
	for i := range children {
		child := &children[i]
		if child.IsSkeletonReference() || m.isAttachmentPoint(child) {
			// This is a skeleton ref - process its children but don't add the ref itself
			baseNode := m.nodeIndex[child.Name]
			if baseNode != nil {
				// Recursively process skeleton ref's children and attach them to base
				for j := range child.Children {
					grandchild := &child.Children[j]
					if !grandchild.IsSkeletonReference() && !m.isAttachmentPoint(grandchild) {
						cloned, err := blockymodel.CloneNode(grandchild)
						if err == nil {
							cloned.Children = m.filterSkeletonRefs(cloned.Children, accessoryID)
							m.reIDNode(cloned, accessoryID)
							baseNode.Children = append(baseNode.Children, *cloned)
							m.reindexChildren(baseNode)
						}
					} else {
						// Recurse into nested skeleton refs
						m.mergeNode(grandchild, accessoryID)
					}
				}
			}
			// Don't add the skeleton ref node itself
		} else {
			// Not a skeleton ref - add it but filter its children too
			cloned := *child
			cloned.Children = m.filterSkeletonRefs(child.Children, accessoryID)
			filtered = append(filtered, cloned)
		}
	}
	return filtered
}

// isAttachmentPoint checks if a node is an attachment point (bone reference)
func (m *Merger) isAttachmentPoint(node *blockymodel.Node) bool {
	if node.IsSkeletonReference() {
		return true
	}
	// Also check if it matches a bone name and has no geometry
	if node.Shape != nil && node.Shape.Type == "none" {
		_, exists := m.nodeIndex[node.Name]
		return exists
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
