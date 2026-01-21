package character

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/hytale-tools/blockymodel-merger/pkg/registry"
)

// Load reads a character data JSON file
func Load(path string) (*CharacterData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read character file %s: %w", path, err)
	}

	var char CharacterData
	if err := json.Unmarshal(data, &char); err != nil {
		return nil, fmt.Errorf("failed to parse character JSON: %w", err)
	}

	return &char, nil
}

// ParseAccessorySpec parses an accessory string like "Scavenger_Hair.PitchBlack" or "AcornEarrings.Acorn.Both"
func ParseAccessorySpec(spec string) AccessorySpec {
	parts := strings.Split(spec, ".")
	result := AccessorySpec{ID: parts[0]}
	if len(parts) > 1 {
		result.Color = parts[1]
	}
	if len(parts) > 2 {
		result.Variant = parts[2]
	}
	return result
}

// AccessoryPath represents a resolved accessory with its file path
type AccessoryPath struct {
	Type            string                    // e.g., "haircut", "ears"
	Spec            AccessorySpec
	Path            string                    // resolved file path
	Entry           *registry.AccessoryEntry  // registry entry (for texture info)
	ResolvedTexture *registry.ResolvedTexture // resolved texture info
}

// ResolveResult contains resolved accessories and any warnings
type ResolveResult struct {
	Accessories []AccessoryPath
	Warnings    []string
}

// ResolveAccessories returns all accessory paths for a character using the registry
func (c *CharacterData) ResolveAccessories(reg *registry.Registry) (*ResolveResult, error) {
	result := &ResolveResult{}
	skinTone := c.GetSkinTone()

	// Helper to add an accessory if it's set
	addAccessory := func(accessoryType string, value *string) {
		if value == nil || *value == "" {
			return
		}

		spec := ParseAccessorySpec(*value)

		// Look up in registry with variant support
		path, err := reg.LookupWithVariant(accessoryType, spec.ID, spec.Variant)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", accessoryType, err))
			return
		}

		// Check if file exists
		if _, err := os.Stat(path); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: file not found: %s", accessoryType, path))
			return
		}

		// Get full entry for texture info
		entry, _ := reg.GetEntry(accessoryType, spec.ID)

		// For skin-related accessories, use skin tone as color if no color specified
		colorForTexture := spec.Color
		if colorForTexture == "" && entry != nil && entry.GradientSet == "Skin" && skinTone != "" {
			colorForTexture = skinTone
		}

		// Resolve texture (greyscale or direct)
		resolvedTex, texErr := reg.ResolveTexture(accessoryType, spec.ID, colorForTexture, spec.Variant)
		if texErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: texture: %v", accessoryType, texErr))
		}

		// Store the color we used (for tinting later)
		spec.Color = colorForTexture

		result.Accessories = append(result.Accessories, AccessoryPath{
			Type:            accessoryType,
			Spec:            spec,
			Path:            path,
			Entry:           entry,
			ResolvedTexture: resolvedTex,
		})
	}

	// Add all accessories in a sensible order (body parts first, then clothing)
	// Body parts
	addAccessory("face", c.Face)
	addAccessory("ears", c.Ears)
	addAccessory("eyes", c.Eyes)
	addAccessory("eyebrows", c.Eyebrows)
	addAccessory("mouth", c.Mouth)
	addAccessory("facialHair", c.FacialHair)
	addAccessory("haircut", c.Haircut)

	// Clothing (bottom to top layering)
	addAccessory("underwear", c.Underwear)
	addAccessory("pants", c.Pants)
	addAccessory("overpants", c.Overpants)
	addAccessory("undertop", c.Undertop)
	addAccessory("overtop", c.Overtop)
	addAccessory("shoes", c.Shoes)
	addAccessory("gloves", c.Gloves)
	addAccessory("cape", c.Cape)

	// Accessories
	addAccessory("headAccessory", c.HeadAccessory)
	addAccessory("faceAccessory", c.FaceAccessory)
	addAccessory("earAccessory", c.EarAccessory)

	return result, nil
}
