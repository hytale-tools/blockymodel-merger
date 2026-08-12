package character

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hytale-tools/blockymodel-merger/pkg/registry"
)

// GradientChecker exposes the gradient-set knowledge needed to validate and
// repair config colors. *texture.GradientSets satisfies this.
type GradientChecker interface {
	// HasGradient reports whether a named color exists in a gradient set.
	HasGradient(setName, colorName string) bool
	// DefaultColor returns the first color declared in a gradient set.
	DefaultColor(setName string) string
}

// ValidationIssue describes one invalid value in a character config.
type ValidationIssue struct {
	Field string // config field, e.g. "haircut"
	Value string // the raw config value
	Err   string // what is wrong with it (and, for Sanitize, the fallback used)
}

func (v ValidationIssue) String() string {
	return fmt.Sprintf("%s %q: %s", v.Field, v.Value, v.Err)
}

// Validate checks every set field against the registry and gradient sets:
// the accessory ID must exist, a variant must be defined on the entry, and a
// color must be either a pre-colored texture key or a color in the entry's
// gradient set. Rendering does not fail on these - it degrades to untinted or
// mis-mapped textures - so API callers should either reject configs with
// issues or call Sanitize to repair them.
func (c *CharacterData) Validate(reg *registry.Registry, gradients GradientChecker) []ValidationIssue {
	return c.check(reg, gradients, false)
}

// Sanitize repairs invalid values in place and returns an issue per repair:
// unknown accessories are removed, unknown variants stripped, unknown colors
// replaced with the accessory's default (first gradient color, or the
// greyscale/first pre-colored texture), and an unknown skin tone replaced
// with the Skin set's default.
func (c *CharacterData) Sanitize(reg *registry.Registry, gradients GradientChecker) []ValidationIssue {
	return c.check(reg, gradients, true)
}

func (c *CharacterData) check(reg *registry.Registry, gradients GradientChecker, repair bool) []ValidationIssue {
	var issues []ValidationIssue

	for _, f := range c.fieldSlots() {
		if *f.ptr == nil || **f.ptr == "" {
			continue
		}
		value := *f.ptr
		spec := ParseAccessorySpec(*value)
		changed := false

		entry, err := reg.GetEntry(f.name, spec.ID)
		if errors.Is(err, registry.ErrRegistryUnavailable) {
			// A missing registry says nothing about the value - leave the slot
			// alone rather than repairing it away.
			issues = append(issues, ValidationIssue{f.name, *value, "registry unavailable, left unchecked"})
			continue
		}
		if err != nil || entry == nil {
			issue := ValidationIssue{f.name, *value, fmt.Sprintf("unknown accessory %q", spec.ID)}
			if repair {
				issue.Err += ", removed"
				*value = ""
			}
			issues = append(issues, issue)
			continue
		}

		var variantEntry *registry.VariantEntry
		if spec.Variant != "" {
			ve, ok := entry.Variants[spec.Variant]
			if !ok {
				issue := ValidationIssue{f.name, *value, fmt.Sprintf("unknown variant %q", spec.Variant)}
				if repair {
					issue.Err += ", stripped"
					spec.Variant = ""
					changed = true
				}
				issues = append(issues, issue)
			} else {
				variantEntry = &ve
			}
		}

		if spec.Color != "" {
			if reason := checkColor(entry, variantEntry, spec.Color, gradients); reason != "" {
				issue := ValidationIssue{f.name, *value, reason}
				if repair {
					fallback := fallbackColor(entry, variantEntry, gradients)
					if fallback != "" {
						issue.Err += fmt.Sprintf(", using %q", fallback)
					} else {
						issue.Err += ", color removed"
					}
					spec.Color = fallback
					changed = true
				}
				issues = append(issues, issue)
			}
		}

		if repair && changed {
			*value = formatSpec(spec)
		}
	}

	// bodyCharacteristic carries the skin tone ("Default.02"); the tone must
	// exist in the Skin gradient set or the base body renders untinted.
	if tone := c.GetSkinTone(); tone != "" && gradients != nil && !gradients.HasGradient("Skin", tone) {
		issue := ValidationIssue{"bodyCharacteristic", *c.BodyCharacteristic,
			fmt.Sprintf("unknown skin tone %q", tone)}
		if repair {
			body := strings.SplitN(*c.BodyCharacteristic, ".", 2)[0]
			if def := gradients.DefaultColor("Skin"); def != "" {
				issue.Err += fmt.Sprintf(", using %q", def)
				*c.BodyCharacteristic = body + "." + def
			} else {
				issue.Err += ", tone removed"
				*c.BodyCharacteristic = body
			}
		}
		issues = append(issues, issue)
	}

	return issues
}

// formatSpec rebuilds the "ID.Color.Variant" config string.
func formatSpec(spec AccessorySpec) string {
	s := spec.ID
	if spec.Color != "" || spec.Variant != "" {
		s += "." + spec.Color
	}
	if spec.Variant != "" {
		s += "." + spec.Variant
	}
	return s
}

// checkColor returns a description of why color is invalid for the entry, or
// "" if it is acceptable: a pre-colored texture key (variant-level first) or a
// color in the entry's gradient set.
func checkColor(entry *registry.AccessoryEntry, variant *registry.VariantEntry, color string, gradients GradientChecker) string {
	if variant != nil {
		if _, ok := variant.Textures[color]; ok {
			return ""
		}
	}
	if _, ok := entry.Textures[color]; ok {
		return ""
	}
	if entry.GradientSet != "" {
		if gradients == nil || gradients.HasGradient(entry.GradientSet, color) {
			return ""
		}
		return fmt.Sprintf("unknown color %q (not in gradient set %q)", color, entry.GradientSet)
	}
	if len(entry.Textures) > 0 || (variant != nil && len(variant.Textures) > 0) {
		return fmt.Sprintf("unknown color %q (no such pre-colored texture)", color)
	}
	return fmt.Sprintf("unknown color %q (accessory takes no color)", color)
}

// fallbackColor picks a replacement for an invalid color: the gradient set's
// default, else the first pre-colored texture key, else no color (greyscale).
func fallbackColor(entry *registry.AccessoryEntry, variant *registry.VariantEntry, gradients GradientChecker) string {
	if entry.GradientSet != "" && gradients != nil {
		if def := gradients.DefaultColor(entry.GradientSet); def != "" {
			return def
		}
	}
	if variant != nil && len(variant.Textures) > 0 && variant.GreyscaleTexture == "" {
		return firstKey(variant.Textures)
	}
	if len(entry.Textures) > 0 && entry.GreyscaleTexture == "" {
		return firstKey(entry.Textures)
	}
	return ""
}

func firstKey(m map[string]registry.TextureEntry) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys[0]
}
