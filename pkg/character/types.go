package character

import "strings"

// CharacterData represents the full outfit/appearance of a character
// Format for values: "AccessoryId.Color.Variant" where Color and Variant are optional
type CharacterData struct {
	BodyCharacteristic *string `json:"bodyCharacteristic"`
	Underwear          *string `json:"underwear"`
	Face               *string `json:"face"`
	Ears               *string `json:"ears"`
	Mouth              *string `json:"mouth"`
	Haircut            *string `json:"haircut"`
	FacialHair         *string `json:"facialHair"`
	Eyebrows           *string `json:"eyebrows"`
	Eyes               *string `json:"eyes"`
	Pants              *string `json:"pants"`
	Overpants          *string `json:"overpants"`
	Undertop           *string `json:"undertop"`
	Overtop            *string `json:"overtop"`
	Shoes              *string `json:"shoes"`
	HeadAccessory      *string `json:"headAccessory"`
	FaceAccessory      *string `json:"faceAccessory"`
	EarAccessory       *string `json:"earAccessory"`
	SkinFeature        *string `json:"skinFeature"`
	Gloves             *string `json:"gloves"`
	Cape               *string `json:"cape"`
}

// fieldSlot pairs a config field's registry name with a pointer to the struct
// field itself, so callers can both inspect and fill the slot.
type fieldSlot struct {
	name string
	ptr  **string
}

// fieldSlots lists every accessory slot except bodyCharacteristic, which
// carries skin-tone semantics and is handled separately by each caller.
func (c *CharacterData) fieldSlots() []fieldSlot {
	return []fieldSlot{
		{"face", &c.Face},
		{"ears", &c.Ears},
		{"eyes", &c.Eyes},
		{"eyebrows", &c.Eyebrows},
		{"mouth", &c.Mouth},
		{"facialHair", &c.FacialHair},
		{"haircut", &c.Haircut},
		{"underwear", &c.Underwear},
		{"pants", &c.Pants},
		{"overpants", &c.Overpants},
		{"undertop", &c.Undertop},
		{"overtop", &c.Overtop},
		{"shoes", &c.Shoes},
		{"gloves", &c.Gloves},
		{"cape", &c.Cape},
		{"headAccessory", &c.HeadAccessory},
		{"faceAccessory", &c.FaceAccessory},
		{"earAccessory", &c.EarAccessory},
		{"skinFeature", &c.SkinFeature},
	}
}

// Clone returns a deep copy of c: Sanitize or ApplyDefaults on the copy
// leaves the original untouched.
func (c *CharacterData) Clone() *CharacterData {
	if c == nil {
		return nil
	}
	out := *c
	for _, f := range out.fieldSlots() {
		if *f.ptr != nil {
			v := **f.ptr
			*f.ptr = &v
		}
	}
	// fieldSlots omits bodyCharacteristic; copy it explicitly.
	if out.BodyCharacteristic != nil {
		v := *out.BodyCharacteristic
		out.BodyCharacteristic = &v
	}
	return &out
}

// AccessorySpec holds the parsed parts of an accessory string
type AccessorySpec struct {
	ID      string // e.g., "Scavenger_Hair"
	Color   string // e.g., "PitchBlack" (optional)
	Variant string // e.g., "Both" or "NoNeck" (optional)
}

// GetSkinTone extracts the skin tone number from bodyCharacteristic
// e.g., "Default.02" returns "02"
func (c *CharacterData) GetSkinTone() string {
	if c.BodyCharacteristic == nil {
		return ""
	}
	parts := strings.Split(*c.BodyCharacteristic, ".")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}
