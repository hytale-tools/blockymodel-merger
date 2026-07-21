package character

import (
	"testing"

	"github.com/hytale-tools/blockymodel-merger/pkg/registry"
)

// fakeGradients maps a gradient set name to its declared colors, first color
// being the set's default.
type fakeGradients map[string][]string

func (f fakeGradients) HasGradient(set, color string) bool {
	for _, c := range f[set] {
		if c == color {
			return true
		}
	}
	return false
}

func (f fakeGradients) DefaultColor(set string) string {
	if colors := f[set]; len(colors) > 0 {
		return colors[0]
	}
	return ""
}

func strptr(s string) *string { return &s }

// The testdata registry subset flags Face_Neutral (GradientSet Skin),
// Medium_Eyes (Eyes_Gradient), Suit (Colored_Cotton), Medium eyebrows, and
// the Default body. Pants.json has no flagged entry; all other registries are
// absent. The gradient sets mirror the real data: the preferred default
// colors (04, Blue, BrownDark) are not first in their sets, except BrownDark.
var testGradients = fakeGradients{
	"Skin":           {"53", "02", "04"},
	"Colored_Cotton": {"Red", "Blue"},
	"Eyes_Gradient":  {"BrownDark", "Blue"},
}

func assertSlot(t *testing.T, field string, got *string, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Errorf("%s = %q, want nil", field, *got)
		}
		return
	}
	if got == nil || *got != want {
		val := "<nil>"
		if got != nil {
			val = *got
		}
		t.Errorf("%s = %s, want %q", field, val, want)
	}
}

func TestApplyDefaults(t *testing.T) {
	reg, err := registry.Load("testdata")
	if err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	c := &CharacterData{}
	c.ApplyDefaults(reg, testGradients)

	assertSlot(t, "BodyCharacteristic", c.BodyCharacteristic, "Default.04") // preferred tone beats first color 53
	assertSlot(t, "Face", c.Face, "Face_Neutral.04")                        // skin-tinted: matches body tone
	assertSlot(t, "Eyes", c.Eyes, "Medium_Eyes.BrownDark")                  // preferred eye color
	assertSlot(t, "Underwear", c.Underwear, "Suit.Blue")                    // preferred beats first color Red
	assertSlot(t, "Eyebrows", c.Eyebrows, "")                               // explicitly skipped: Hytale allows null
	assertSlot(t, "Pants", c.Pants, "")                                     // nothing flagged
	assertSlot(t, "Haircut", c.Haircut, "")                                 // registry absent
}

func TestApplyDefaultsMatchesSetSkinTone(t *testing.T) {
	reg, err := registry.Load("testdata")
	if err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	c := &CharacterData{BodyCharacteristic: strptr("Default.02")}
	c.ApplyDefaults(reg, testGradients)

	assertSlot(t, "BodyCharacteristic", c.BodyCharacteristic, "Default.02") // set slots never overwritten
	assertSlot(t, "Face", c.Face, "Face_Neutral.02")                        // follows the chosen tone, not 04
}

func TestApplyDefaultsSetSlotUntouched(t *testing.T) {
	reg, err := registry.Load("testdata")
	if err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	c := &CharacterData{Face: strptr("Face_Custom.01")}
	c.ApplyDefaults(reg, testGradients)
	assertSlot(t, "Face", c.Face, "Face_Custom.01")
}

func TestApplyDefaultsGradientFallbacks(t *testing.T) {
	reg, err := registry.Load("testdata")
	if err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	// Preferred colors absent from the sets: fall back to the first color.
	c := &CharacterData{}
	c.ApplyDefaults(reg, fakeGradients{"Skin": {"53"}, "Colored_Cotton": {"Red"}})
	assertSlot(t, "BodyCharacteristic", c.BodyCharacteristic, "Default.53")
	assertSlot(t, "Face", c.Face, "Face_Neutral.53")
	assertSlot(t, "Underwear", c.Underwear, "Suit.Red")

	// No gradient data at all: bare IDs.
	c = &CharacterData{}
	c.ApplyDefaults(reg, nil)
	assertSlot(t, "BodyCharacteristic", c.BodyCharacteristic, "Default")
	assertSlot(t, "Face", c.Face, "Face_Neutral")
	assertSlot(t, "Eyes", c.Eyes, "Medium_Eyes")
}

func TestClone(t *testing.T) {
	orig := &CharacterData{
		Face:               strptr("Face_Neutral.01"),
		BodyCharacteristic: strptr("Default.02"),
	}

	clone := orig.Clone()
	*clone.Face = "Face_Sunken"
	*clone.BodyCharacteristic = "Muscular.01"
	clone.Eyes = strptr("Medium_Eyes")

	if *orig.Face != "Face_Neutral.01" {
		t.Errorf("original Face changed to %q", *orig.Face)
	}
	if *orig.BodyCharacteristic != "Default.02" {
		t.Errorf("original BodyCharacteristic changed to %q", *orig.BodyCharacteristic)
	}
	if orig.Eyes != nil {
		t.Errorf("original Eyes changed to %q", *orig.Eyes)
	}

	if (*CharacterData)(nil).Clone() != nil {
		t.Error("Clone of nil should be nil")
	}
}
