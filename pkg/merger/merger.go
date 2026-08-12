package merger

import (
	"fmt"
	"strconv"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
	"github.com/hytale-tools/blockymodel-merger/pkg/util"
)

// Merger handles merging accessories into a base model
type Merger struct {
	base   *blockymodel.BlockyModel
	nextID int
	// Track which accessory each merged node ID came from
	NodeSources map[string]string // node ID -> accessory ID
	// Names of attachment-point-eligible nodes (shape.type=="none") in the
	// ORIGINAL base, captured before any merging. Used to prevent accessory
	// nodes from matching against geometry containers that were added by a
	// previously-merged accessory.
	baseBoneNames map[string]bool
}

// New creates a new Merger with the given base model (deep copied)
func New(base *blockymodel.BlockyModel) (*Merger, error) {
	cloned, err := blockymodel.Clone(base)
	if err != nil {
		return nil, fmt.Errorf("failed to clone base model: %w", err)
	}

	// Find the highest existing ID to use for new nodes
	maxID := findMaxID(cloned.Nodes)

	boneNames := make(map[string]bool)
	collectBoneNames(cloned.Nodes, boneNames)

	return &Merger{
		base:          cloned,
		nextID:        maxID + 1,
		NodeSources:   make(map[string]string),
		baseBoneNames: boneNames,
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
			if child.Name == blockymodel.HeldItemNodeName {
				if err := m.attachVerbatim(baseNode, child, accessoryID); err != nil {
					return err
				}
			} else if !child.IsSkeletonReference() && !m.isAttachmentPoint(child) {
				// Clone and re-ID the child, then append to base
				cloned, err := blockymodel.CloneNode(child)
				if err != nil {
					return fmt.Errorf("failed to clone node %s: %w", child.Name, err)
				}
				// Filter out skeleton ref nodes from cloned children - they should only be attachment points
				cloned.Children = m.filterSkeletonRefs(cloned.Children, accessoryID)
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

// attachVerbatim clones an entire subtree onto baseNode without any bone
// matching. It is how the held-item group is merged: an item model brings its
// own authoring rig, whose bone names collide with the character's (a
// severed-head item model has Head, Chest and eye bones of its own), so
// matching inside it would graft the item's geometry onto the character.
func (m *Merger) attachVerbatim(baseNode, node *blockymodel.Node, accessoryID string) error {
	cloned, err := blockymodel.CloneNode(node)
	if err != nil {
		return fmt.Errorf("failed to clone node %s: %w", node.Name, err)
	}
	m.reIDNode(cloned, accessoryID)
	baseNode.Children = append(baseNode.Children, *cloned)
	return nil
}

// filterSkeletonRefs removes skeleton reference nodes from children and processes their children instead
func (m *Merger) filterSkeletonRefs(children []blockymodel.Node, accessoryID string) []blockymodel.Node {
	var filtered []blockymodel.Node
	for i := range children {
		child := &children[i]
		if child.Name == blockymodel.HeldItemNodeName {
			filtered = append(filtered, *child) // held items keep their own rig
		} else if child.IsSkeletonReference() || m.isAttachmentPoint(child) {
			// This is a skeleton ref - process its children but don't add the ref itself
			baseNode := findNodeByName(m.base.Nodes, child.Name)
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
	// Also check if it matches an original base bone and has no geometry.
	// Only the ORIGINAL base bones count - m.base mutates as accessories are
	// merged in, and a type="none" container node from an earlier accessory
	// (e.g. Hair-Bun under Hairband_R in Magical_Pigtails) would otherwise
	// masquerade as an attachment point for a later accessory.
	if node.Shape != nil && node.Shape.Type == "none" {
		if m.baseBoneNames[node.Name] {
			return true
		}
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

// collectBoneNames records the names of every node with shape.type=="none"
// into out. Used to snapshot the original base skeleton.
func collectBoneNames(nodes []blockymodel.Node, out map[string]bool) {
	for i := range nodes {
		n := &nodes[i]
		if n.Shape != nil && n.Shape.Type == "none" {
			out[n.Name] = true
		}
		collectBoneNames(n.Children, out)
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
