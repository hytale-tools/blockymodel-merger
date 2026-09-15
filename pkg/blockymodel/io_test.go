package blockymodel

import (
	"encoding/json"
	"strings"
	"testing"
)

// Hytale writes "textureLayout": {} on every shape and its client will not
// load a box or quad that lacks the key. The Overtops_Scarf cosmetic ships a
// faceless PelvisHider box, and omitempty used to drop the key on export.
func TestShapeMarshalAlwaysWritesTextureLayout(t *testing.T) {
	cases := map[string]*Shape{
		"nil map":   {Type: "box", TextureLayout: nil},
		"empty map": {Type: "box", TextureLayout: map[string]TextureFace{}},
		"none":      {Type: "none"},
	}
	for name, shape := range cases {
		data, err := json.Marshal(shape)
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		if !strings.Contains(string(data), `"textureLayout":{}`) {
			t.Errorf("%s: expected an empty textureLayout object, got %s", name, data)
		}
	}
}

func TestShapeMarshalKeepsFaces(t *testing.T) {
	shape := &Shape{
		Type: "quad",
		TextureLayout: map[string]TextureFace{
			"front": {Offset: Vec2{X: 4, Y: 8}, Angle: 90},
		},
	}
	data, err := json.Marshal(shape)
	if err != nil {
		t.Fatal(err)
	}
	var back Shape
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	face, ok := back.TextureLayout["front"]
	if !ok || face.Offset.X != 4 || face.Offset.Y != 8 || face.Angle != 90 {
		t.Errorf("round trip lost the front face: %s", data)
	}
}

func TestFacelessBoxRoundTrip(t *testing.T) {
	src := `{"nodes":[{"id":"1","name":"PelvisHider","position":{"x":0,"y":0,"z":0},"orientation":{"w":1,"x":0,"y":0,"z":0},"shape":{"type":"box","textureLayout":{},"settings":{"size":{"x":26,"y":6,"z":19}}},"children":null}]}`
	var model BlockyModel
	if err := json.Unmarshal([]byte(src), &model); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(&model)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"textureLayout":{}`) {
		t.Errorf("faceless box lost its textureLayout key: %s", data)
	}
}
