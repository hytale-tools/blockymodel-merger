package merger

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
)

func parseModel(t *testing.T, src string) *blockymodel.BlockyModel {
	t.Helper()
	var m blockymodel.BlockyModel
	if err := json.Unmarshal([]byte(src), &m); err != nil {
		t.Fatalf("parse model: %v", err)
	}
	return &m
}

// Player declares empty eyelid bones; the face model supplies their quads
// under the same names, hung off a Head reference.
const eyelidBase = `{"nodes": [{"id": "1", "name": "Head", "shape": {"type": "none"}, "children": [
	{"id": "2", "name": "L-Eyelid-Top", "position": {"x": -8, "y": 1, "z": 13.9}, "shape": {"type": "none"}},
	{"id": "3", "name": "R-Eyelid-Top", "position": {"x": 8, "y": 1, "z": 13.9}, "shape": {"type": "none"}}
]}]}`

const eyelidFace = `{"nodes": [{"id": "1", "name": "Head", "shape": {"type": "none", "settings": {"isPiece": true}}, "children": [
	{"id": "2", "name": "Face", "shape": {"type": "box"}},
	{"id": "3", "name": "L-Eyelid-Top", "position": {"x": -8, "y": 1, "z": 13.9}, "shape": {"type": "quad"}},
	{"id": "4", "name": "R-Eyelid-Top", "position": {"x": 0, "y": 0, "z": 0}, "shape": {"type": "quad"}}
]}]}`

func TestMergeFillsEmptyBone(t *testing.T) {
	m, err := New(parseModel(t, eyelidBase))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Merge(parseModel(t, eyelidFace), "face"); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	count := map[string]int{}
	var walk func(nodes []blockymodel.Node)
	walk = func(nodes []blockymodel.Node) {
		for i := range nodes {
			count[nodes[i].Name]++
			walk(nodes[i].Children)
		}
	}
	result := m.Result()
	walk(result.Nodes)

	if count["L-Eyelid-Top"] != 1 {
		t.Errorf("L-Eyelid-Top appears %d times, want 1 (bone filled in place)", count["L-Eyelid-Top"])
	}
	bone := findNodeByName(result.Nodes, "L-Eyelid-Top")
	if bone.ID != "2" || bone.Shape == nil || bone.Shape.Type != "quad" {
		t.Errorf("L-Eyelid-Top = id %s shape %+v, want base bone 2 carrying the quad", bone.ID, bone.Shape)
	}
	if m.NodeSources["2"] != "face" {
		t.Errorf("NodeSources[2] = %q, want face (for the atlas offset)", m.NodeSources["2"])
	}

	// A same-named node elsewhere can't fill the bone; it hangs off it as an
	// overlay that keeps its rest position.
	if count["R-Eyelid-Top"] != 1 {
		t.Errorf("R-Eyelid-Top appears %d times, want 1", count["R-Eyelid-Top"])
	}
	r := findNodeByName(result.Nodes, "R-Eyelid-Top")
	if r.Shape.Type != "none" || len(r.Children) != 1 || r.Children[0].Name != "R-Eyelid-Top-face" {
		t.Fatalf("R-Eyelid-Top = shape %q children %+v, want empty bone with overlay R-Eyelid-Top-face", r.Shape.Type, r.Children)
	}
	assertPos(t, "R-Eyelid-Top-face", r.Children[0].Position, -8, -1, -13.9)
	if count["Face"] != 1 {
		t.Errorf("Face appears %d times, want 1", count["Face"])
	}
}

func assertPos(t *testing.T, name string, p *blockymodel.Vec3, x, y, z float64) {
	t.Helper()
	if p == nil || math.Abs(p.X-x) > 1e-6 || math.Abs(p.Y-y) > 1e-6 || math.Abs(p.Z-z) > 1e-6 {
		t.Errorf("%s position = %+v, want (%g, %g, %g)", name, p, x, y, z)
	}
}

// Stockings models the legs as boxes named after Player's, under an Origin
// piece, with its Pelvis offset half a pixel lower than Player's (so its
// thighs sit half a pixel higher to compensate).
const legBase = `{"nodes": [{"id": "1", "name": "Origin", "shape": {"type": "none"}, "children": [
	{"id": "2", "name": "Pelvis", "position": {"x": 0, "y": 51, "z": 0}, "shape": {"type": "box", "offset": {"x": 0, "y": 0, "z": 0}}, "children": [
		{"id": "3", "name": "R-Thigh", "position": {"x": -7.5, "y": -1, "z": 1}, "shape": {"type": "box", "offset": {"x": 0, "y": -11, "z": 0}}}
	]}
]}]}`

const legStockings = `{"nodes": [{"id": "1", "name": "Origin", "shape": {"type": "none", "settings": {"isPiece": true}}, "children": [
	{"id": "2", "name": "Pelvis", "position": {"x": 0, "y": 51, "z": 0}, "shape": {"type": "box", "offset": {"x": 0, "y": -0.5, "z": 0}}, "children": [
		{"id": "3", "name": "R-Thigh", "position": {"x": -7.5, "y": -0.5, "z": 1}, "shape": {"type": "box", "offset": {"x": 0, "y": -11, "z": 0}}, "children": [
			{"id": "4", "name": "Garter", "position": {"x": 0, "y": -2, "z": 0}, "shape": {"type": "box"}}
		]}
	]}
]}]}`

func TestMergeBindsNamedGeometry(t *testing.T) {
	m, err := New(parseModel(t, legBase))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Merge(parseModel(t, legStockings), "overpants:Stockings"); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	result := m.Result()

	origin := &result.Nodes[0]
	if len(origin.Children) != 1 {
		t.Fatalf("Origin has %d children, want 1 (no second Pelvis)", len(origin.Children))
	}
	pelvis := &origin.Children[0]
	if pelvis.ID != "2" {
		t.Fatalf("Pelvis id = %s, want base 2", pelvis.ID)
	}

	var pelvisOverlay, thigh *blockymodel.Node
	for i := range pelvis.Children {
		switch c := &pelvis.Children[i]; c.Name {
		case "Pelvis-Stockings":
			pelvisOverlay = c
		case "R-Thigh":
			thigh = c
		}
	}
	if pelvisOverlay == nil || thigh == nil || len(pelvis.Children) != 2 {
		t.Fatalf("Pelvis children = %+v, want R-Thigh and Pelvis-Stockings", pelvis.Children)
	}
	assertPos(t, "Pelvis-Stockings", pelvisOverlay.Position, 0, 0, 0)
	if m.NodeSources[pelvisOverlay.ID] != "overpants:Stockings" {
		t.Errorf("Pelvis-Stockings source = %q", m.NodeSources[pelvisOverlay.ID])
	}

	// The stocking thigh lands where it was authored: at the base thigh's
	// origin, which is the base thigh's child frame minus its offset.
	if thigh.ID != "3" || len(thigh.Children) != 1 || thigh.Children[0].Name != "R-Thigh-Stockings" {
		t.Fatalf("R-Thigh = id %s children %+v, want base 3 with R-Thigh-Stockings", thigh.ID, thigh.Children)
	}
	thighOverlay := &thigh.Children[0]
	assertPos(t, "R-Thigh-Stockings", thighOverlay.Position, 0, 11, 0)

	// Unmatched children ride along under the overlay, untouched.
	if len(thighOverlay.Children) != 1 || thighOverlay.Children[0].Name != "Garter" {
		t.Fatalf("R-Thigh-Stockings children = %+v, want Garter", thighOverlay.Children)
	}
	assertPos(t, "Garter", thighOverlay.Children[0].Position, 0, -2, 0)
}

// VarsityJacket's collar is a box named Neck under Chest; Player's Neck is
// under Head. It must not bind, and must not keep the bone's name either.
const collarBase = `{"nodes": [{"id": "1", "name": "Chest", "shape": {"type": "none"}, "children": [
	{"id": "2", "name": "Head", "shape": {"type": "none"}, "children": [
		{"id": "3", "name": "Neck", "shape": {"type": "box"}}
	]}
]}]}`

const collarJacket = `{"nodes": [{"id": "1", "name": "Chest", "shape": {"type": "none", "settings": {"isPiece": true}}, "children": [
	{"id": "2", "name": "Neck", "shape": {"type": "box"}}
]}]}`

func TestMergeRenamesUnboundNameClash(t *testing.T) {
	m, err := New(parseModel(t, collarBase))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Merge(parseModel(t, collarJacket), "overtop:Varsity_Jacket"); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	chest := &m.Result().Nodes[0]
	if len(chest.Children) != 2 || chest.Children[1].Name != "Neck-Varsity_Jacket" {
		t.Fatalf("Chest children = %+v, want Head and Neck-Varsity_Jacket", chest.Children)
	}
	if neck := findNodeByName(m.Result().Nodes, "Neck"); neck.ID != "3" {
		t.Errorf("Neck resolves to id %s, want base 3", neck.ID)
	}
}
