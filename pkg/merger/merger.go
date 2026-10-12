package merger

import (
	"fmt"
	"strconv"
	"strings"

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
	// IDs of every node in the ORIGINAL base: the nodes accessory geometry
	// may bind to by name (see attachGeometry).
	baseIDs map[string]bool
	// Names of every node in the ORIGINAL base (see reIDNode).
	baseNames map[string]bool
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
	baseIDs := make(map[string]bool)
	baseNames := make(map[string]bool)
	collectIDs(cloned.Nodes, baseIDs, baseNames)

	return &Merger{
		base:          cloned,
		nextID:        maxID + 1,
		NodeSources:   make(map[string]string),
		baseBoneNames: boneNames,
		baseIDs:       baseIDs,
		baseNames:     baseNames,
	}, nil
}

// Merge integrates an accessory into the base model
// accessoryID is used to track which accessory each merged node came from (for texture offset updates)
func (m *Merger) Merge(accessory *blockymodel.BlockyModel, accessoryID string) error {
	for i := range accessory.Nodes {
		if err := m.mergeNode(&accessory.Nodes[i], accessoryID, nil); err != nil {
			return err
		}
	}
	return nil
}

// Result returns the merged model
func (m *Merger) Result() *blockymodel.BlockyModel {
	return m.base
}

// mergeNode processes a single accessory node. parent is the base-model node
// the accessory subtree is being attached under (nil at the top level); an
// unmatched attachment reference is added there rather than dropped.
func (m *Merger) mergeNode(accessoryNode *blockymodel.Node, accessoryID string, parent *blockymodel.Node) error {
	// Check if this node matches a bone in the base model (either skeleton ref or by name)
	baseNode := findNodeByName(m.base.Nodes, accessoryNode.Name)

	if accessoryNode.IsSkeletonReference() || (baseNode != nil && accessoryNode.Shape != nil && accessoryNode.Shape.Type == "none") {
		// This is an attachment point - attach children to base model
		if baseNode == nil {
			// The game adds unmatched attachment nodes to the model
			// instead of dropping them. Routine, not an error: a cosmetic can
			// provide sockets for later cosmetics (Default ears carries
			// R/L-Ear-EarAccessory for earrings), and an accessory can bring
			// its own socket (DoubleEarrings' Tilt socket on Default ears).
			util.Logger.Debug("No matching attachment point in base model, adding node",
				"node", accessoryNode.Name, "accessory", accessoryID)
			return m.addUnmatchedNode(accessoryNode, parent, accessoryID)
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
				if err := m.attachGeometry(baseNode, child, identityFrame, accessoryID); err != nil {
					return err
				}
			} else {
				// Recurse into skeleton reference children
				if err := m.mergeNode(child, accessoryID, baseNode); err != nil {
					return err
				}
			}
		}
	} else {
		// Non-skeleton reference nodes at top level: recursively process children
		for i := range accessoryNode.Children {
			if err := m.mergeNode(&accessoryNode.Children[i], accessoryID, parent); err != nil {
				return err
			}
		}
	}
	return nil
}

// attachGeometry adds an accessory geometry node under baseNode, binding it to
// the base node of the same name when baseNode has one. Cosmetics model the
// body parts they cover as boxes named after the Player nodes they follow:
// Stockings is a Pelvis box with R-Thigh, R-Calf and R-Foot boxes under it,
// and Player's eyelid bones are empty until the face supplies their quads.
// Copying such a subtree in as-is would leave a second, unanimated skeleton
// beside the real one, so each named node is bound to its base counterpart:
//
//   - an empty base bone at the same place takes the node's shape directly
//     (the eyelids), keeping one node per name;
//   - otherwise the node's shape is added under the base node as an overlay
//     (named "<node>-<accessory>"), placed so it keeps its rest position.
//
// Children are matched the same way one level down, against the base node's
// children; whatever does not match rides along under the shape it hung off.
// delta is the accessory parent's child frame expressed in baseNode's child
// frame: identity at an attachment point, and whatever small mismatch the
// authored parent had (Stockings' Pelvis is offset 0.5 lower than Player's)
// further down.
func (m *Merger) attachGeometry(baseNode, node *blockymodel.Node, delta frame, accessoryID string) error {
	bone := m.originalChild(baseNode, node.Name)
	if bone == nil {
		cloned, err := blockymodel.CloneNode(node)
		if err != nil {
			return fmt.Errorf("failed to clone node %s: %w", node.Name, err)
		}
		// Filter out skeleton ref nodes from cloned children - they should only be attachment points
		cloned.Children, err = m.filterSkeletonRefs(cloned.Children, accessoryID)
		if err != nil {
			return err
		}
		delta.mul(localFrame(cloned)).setOn(cloned)
		m.reIDNode(cloned, accessoryID)
		baseNode.Children = append(baseNode.Children, *cloned)
		return nil
	}

	nodeFrame := delta.mul(localFrame(node))
	var shape *blockymodel.Shape
	if node.Shape != nil {
		shape = node.Shape.Clone()
	}

	// The eyelid case: fill the empty bone in place.
	fill := shape != nil && shape.Type != "none" &&
		(bone.Shape == nil || bone.Shape.Type == "none") &&
		childFrame(bone).inverse().mul(nodeFrame).isIdentity()
	var overlay *blockymodel.Node
	if fill {
		bone.Shape = shape
		// The shape keeps the accessory's UVs, so the bone's ID must map to
		// the accessory for its atlas offset.
		m.NodeSources[bone.ID] = accessoryID
	} else {
		overlay = &blockymodel.Node{Name: node.Name + "-" + overlaySuffix(accessoryID), Shape: shape}
		childFrame(bone).inverse().mul(nodeFrame).setOn(overlay)
	}

	// The node's child frame, in the frame of whichever node now hosts its
	// children: the bone (filled with the node's shape) or the overlay.
	childDelta := childFrame(bone).inverse().mul(nodeFrame).mul(offsetFrame(node))
	var rest []blockymodel.Node
	for i := range node.Children {
		child := &node.Children[i]
		if child.Name != blockymodel.HeldItemNodeName && !child.IsSkeletonReference() &&
			!m.isAttachmentPoint(child) && m.originalChild(bone, child.Name) != nil {
			if err := m.attachGeometry(bone, child, childDelta, accessoryID); err != nil {
				return err
			}
			continue
		}
		rest = append(rest, *child)
	}

	rest, err := m.filterSkeletonRefs(rest, accessoryID)
	if err != nil {
		return err
	}
	for i := range rest {
		cloned, err := blockymodel.CloneNode(&rest[i])
		if err != nil {
			return fmt.Errorf("failed to clone node %s: %w", rest[i].Name, err)
		}
		m.reIDNode(cloned, accessoryID)
		rest[i] = *cloned
	}
	if fill {
		bone.Children = append(bone.Children, rest...)
		return nil
	}
	overlay.Children = rest
	overlay.ID = strconv.Itoa(m.nextID)
	m.NodeSources[overlay.ID] = accessoryID
	m.nextID++
	bone.Children = append(bone.Children, *overlay)
	return nil
}

// originalChild returns the direct child of baseNode named name if it came
// with the base model. Nodes added by earlier accessories never bind.
func (m *Merger) originalChild(baseNode *blockymodel.Node, name string) *blockymodel.Node {
	for i := range baseNode.Children {
		child := &baseNode.Children[i]
		if child.Name == name && m.baseIDs[child.ID] {
			return child
		}
	}
	return nil
}

// overlaySuffix is the accessory part of an overlay node's name: the ID out
// of a "type:ID" tracking key.
func overlaySuffix(accessoryID string) string {
	if i := strings.LastIndex(accessoryID, ":"); i >= 0 {
		return accessoryID[i+1:]
	}
	return accessoryID
}

// addUnmatchedNode clones an attachment-reference node that has no counterpart
// in the base model and adds it under parent (or as a new root when parent is
// nil), so later accessories can attach to it by name.
func (m *Merger) addUnmatchedNode(node *blockymodel.Node, parent *blockymodel.Node, accessoryID string) error {
	cloned, err := blockymodel.CloneNode(node)
	if err != nil {
		return fmt.Errorf("failed to clone node %s: %w", node.Name, err)
	}
	cloned.Children, err = m.filterSkeletonRefs(cloned.Children, accessoryID)
	if err != nil {
		return err
	}
	m.reIDNode(cloned, accessoryID)
	if parent != nil {
		parent.Children = append(parent.Children, *cloned)
	} else {
		m.base.Nodes = append(m.base.Nodes, *cloned)
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
func (m *Merger) filterSkeletonRefs(children []blockymodel.Node, accessoryID string) ([]blockymodel.Node, error) {
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
						if err := m.attachGeometry(baseNode, grandchild, identityFrame, accessoryID); err != nil {
							return nil, err
						}
					} else {
						// Recurse into nested skeleton refs
						if err := m.mergeNode(grandchild, accessoryID, baseNode); err != nil {
							return nil, err
						}
					}
				}
			} else {
				// Unmatched ref - keep it in place so later accessories can
				// attach to it by name (see addUnmatchedNode).
				cloned := *child
				var err error
				cloned.Children, err = m.filterSkeletonRefs(child.Children, accessoryID)
				if err != nil {
					return nil, err
				}
				filtered = append(filtered, cloned)
			}
		} else {
			// Not a skeleton ref - add it but filter its children too
			cloned := *child
			var err error
			cloned.Children, err = m.filterSkeletonRefs(child.Children, accessoryID)
			if err != nil {
				return nil, err
			}
			filtered = append(filtered, cloned)
		}
	}
	return filtered, nil
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

// reIDNode assigns new unique IDs to a node and all its children, tracking
// accessory source. A node that reuses a base node's name without being bound
// to it (VarsityJacket's Neck collar hangs off Chest, not Head like Player's
// Neck) gets the accessory suffix, so a name only ever means the base node
// to anything that drives nodes by name.
func (m *Merger) reIDNode(node *blockymodel.Node, accessoryID string) {
	if m.baseNames[node.Name] {
		node.Name += "-" + overlaySuffix(accessoryID)
	}
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

// collectIDs records the ID and name of every node into ids and names.
func collectIDs(nodes []blockymodel.Node, ids, names map[string]bool) {
	for i := range nodes {
		ids[nodes[i].ID] = true
		names[nodes[i].Name] = true
		collectIDs(nodes[i].Children, ids, names)
	}
}
