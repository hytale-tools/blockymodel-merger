// Package pipeline factors the merge + tint + atlas orchestration so it can be
// shared between the GLB exporter (blockymerge) and the renderer (blockyrender).
package pipeline

import (
	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
	"github.com/hytale-tools/blockymodel-merger/pkg/character"
	"github.com/hytale-tools/blockymodel-merger/pkg/merger"
	"github.com/hytale-tools/blockymodel-merger/pkg/registry"
	"github.com/hytale-tools/blockymodel-merger/pkg/texture"
	"github.com/hytale-tools/blockymodel-merger/pkg/util"
)

const (
	BasePath        = "assets/Characters/Player.blockymodel"
	BaseTexturePath = "assets/Characters/Player_Textures/Player_Greyscale.png"
)

// Result holds a fully merged character ready to render or export.
type Result struct {
	Model *blockymodel.BlockyModel
	Atlas *texture.Atlas // may be nil if texturing was skipped
}

// BuildMergedCharacter loads a character config, merges all resolved accessories
// onto the base player model, tints the textures, packs an atlas, and rewrites
// the merged model's texture-layout offsets into atlas space.
//
// This mirrors the orchestration in cmd/blockymerge so a render and a GLB export
// of the same character share identical geometry and texture coordinates.
func BuildMergedCharacter(charFile string, noTint bool) (*Result, error) {
	gradientSets, err := texture.LoadGradientSets()
	if err != nil {
		util.Logger.Warn("Could not load gradient sets", "error", err)
	}

	reg, err := registry.Load()
	if err != nil {
		return nil, err
	}

	charData, err := character.Load(charFile)
	if err != nil {
		return nil, err
	}

	resolved, err := charData.ResolveAccessories(reg)
	if err != nil {
		return nil, err
	}
	for _, warn := range resolved.Warnings {
		util.Logger.Warn("Accessory warning", "message", warn)
	}

	base, err := blockymodel.Load(BasePath)
	if err != nil {
		return nil, err
	}

	m, err := merger.New(base)
	if err != nil {
		return nil, err
	}

	for _, acc := range resolved.Accessories {
		accessory, err := blockymodel.Load(acc.Path)
		if err != nil {
			return nil, err
		}
		// Use the type-qualified key as the identifier for tracking texture
		// offsets - bare IDs can collide across accessory types (e.g.
		// eyebrows "Medium" vs facialHair "Medium").
		if err := m.Merge(accessory, acc.Key()); err != nil {
			return nil, err
		}
	}

	model := m.Result()
	res := &Result{Model: model}

	if noTint {
		return res, nil
	}

	tintedTextures := buildTintedTextures(charData, resolved, gradientSets)
	if len(tintedTextures) == 0 {
		return res, nil
	}

	atlas, err := texture.PackAtlasSimple(tintedTextures, 1)
	if err != nil {
		util.Logger.Warn("Failed to pack atlas", "error", err)
		return res, nil
	}
	res.Atlas = atlas

	applyAtlasOffsets(model, m, atlas, tintedTextures)
	return res, nil
}

func buildTintedTextures(
	charData *character.CharacterData,
	resolved *character.ResolveResult,
	gradientSets *texture.GradientSets,
) []*texture.TintedTexture {
	var tinted []*texture.TintedTexture

	// Base player texture (skin-toned).
	skinTone := charData.GetSkinTone()
	if skinTone != "" {
		baseTinted, err := texture.ProcessAccessoryTexture("_base", BaseTexturePath, "Skin", skinTone, gradientSets)
		if err != nil {
			util.Logger.Warn("Could not tint base texture", "error", err)
		} else {
			tinted = append(tinted, baseTinted)
		}
	} else {
		baseImg, err := texture.LoadImage(BaseTexturePath)
		if err != nil {
			util.Logger.Warn("Could not load base texture", "error", err)
		} else {
			tinted = append(tinted, &texture.TintedTexture{Name: "_base", Image: baseImg, OriginalPath: BaseTexturePath})
		}
	}

	// Accessory textures.
	for _, acc := range resolved.Accessories {
		if acc.ResolvedTexture == nil {
			continue
		}
		switch {
		case acc.ResolvedTexture.DirectTexture != "":
			img, err := texture.LoadImage(acc.ResolvedTexture.DirectTexture)
			if err != nil {
				util.Logger.Warn("Failed to load direct texture", "id", acc.Spec.ID, "error", err)
				continue
			}
			tinted = append(tinted, &texture.TintedTexture{Name: acc.Key(), Image: img, OriginalPath: acc.ResolvedTexture.DirectTexture})
		case acc.ResolvedTexture.GreyscaleTexture != "":
			t, err := texture.ProcessAccessoryTexture(
				acc.Key(),
				acc.ResolvedTexture.GreyscaleTexture,
				acc.ResolvedTexture.GradientSet,
				acc.Spec.Color,
				gradientSets,
			)
			if err != nil {
				util.Logger.Warn("Failed to process accessory texture", "id", acc.Spec.ID, "error", err)
				continue
			}
			tinted = append(tinted, t)
		}
	}

	return tinted
}

func applyAtlasOffsets(model *blockymodel.BlockyModel, m *merger.Merger, atlas *texture.Atlas, tinted []*texture.TintedTexture) {
	for _, tex := range tinted {
		if tex.Name == "_base" {
			continue // base stays at origin
		}
		x, y, _, _, ok := atlas.GetPixelCoords(tex.Name)
		if !ok {
			continue
		}
		nodeIDs := make(map[string]bool)
		for nodeID, accessoryID := range m.NodeSources {
			if accessoryID == tex.Name {
				nodeIDs[nodeID] = true
			}
		}
		if len(nodeIDs) > 0 {
			blockymodel.UpdateTextureOffsets(model.Nodes, nodeIDs, blockymodel.AtlasOffset{X: float64(x), Y: float64(y)})
		}
	}
}
