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
