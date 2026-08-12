package character

import (
	"github.com/hytale-tools/blockymodel-merger/pkg/registry"
)

// preferredDefaultColors picks the color used when defaulting a slot whose
// entry tints from the given gradient set. These mirror the game's customizer
// defaults (and the values embedders hardcoded before this existed); a
// preferred color missing from the gradient data falls back to the set's
// first color, so a game update cannot break defaulting.
var preferredDefaultColors = map[string]string{
	"Skin":           "04",
	"Colored_Cotton": "Blue",
	"Eyes_Gradient":  "BrownDark",
}

// ApplyDefaults fills required slots that are nil or empty with the entry the
// registry flags IsDefaultAsset for that slot. Slots whose registry declares
// no default are left untouched, and set slots are never overwritten, so
// defaulting is data-driven and safe to apply to any config.
//
// Colors: skin-tinted entries (face, ears, mouth) match the character's skin
// tone; other entries use their gradient set's preferred default color (see
// preferredDefaultColors), else the same fallback rule Sanitize uses (first
// gradient color, else first pre-colored texture key).
//
// Eyebrows are skipped even though the game data flags a default: Hytale
// allows an empty eyebrows slot.
func (c *CharacterData) ApplyDefaults(reg *registry.Registry, gradients GradientChecker) {
	// Body first: the skin-tinted defaults below follow its tone.
	if c.BodyCharacteristic == nil || *c.BodyCharacteristic == "" {
		if id, ok := reg.DefaultFor("bodyCharacteristic"); ok {
			value := id
			if tone := preferredColor("Skin", gradients); tone != "" {
				value = id + "." + tone
			}
			c.BodyCharacteristic = &value
		}
	}
	skinTone := c.GetSkinTone()

	for _, f := range c.fieldSlots() {
		if *f.ptr != nil && **f.ptr != "" {
			continue
		}
		if f.name == "eyebrows" {
			continue
		}
		id, ok := reg.DefaultFor(f.name)
		if !ok {
			continue
		}
		value := id
		if entry, err := reg.GetEntry(f.name, id); err == nil {
			if color := defaultEntryColor(entry, skinTone, gradients); color != "" {
				value = id + "." + color
			}
		}
		*f.ptr = &value
	}
}

// defaultEntryColor picks the color for a defaulted entry: skin-tinted
// entries match the character's skin tone, other tinted entries use their
// set's preferred default color, and anything else falls back to Sanitize's
// fallback rule.
func defaultEntryColor(entry *registry.AccessoryEntry, skinTone string, gradients GradientChecker) string {
	if entry.GradientSet == "Skin" && skinTone != "" &&
		(gradients == nil || gradients.HasGradient("Skin", skinTone)) {
		return skinTone
	}
	if entry.GradientSet != "" {
		if color := preferredColor(entry.GradientSet, gradients); color != "" {
			return color
		}
	}
	return fallbackColor(entry, nil, gradients)
}

// preferredColor returns the preferred default color of a gradient set when
// the set declares it, else the set's first color.
func preferredColor(set string, gradients GradientChecker) string {
	if gradients == nil {
		return ""
	}
	if pref, ok := preferredDefaultColors[set]; ok && gradients.HasGradient(set, pref) {
		return pref
	}
	return gradients.DefaultColor(set)
}
