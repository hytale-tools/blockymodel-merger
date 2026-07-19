// Package pipeline factors the merge + tint + atlas orchestration so it can be
// shared between the GLB exporter (blockymerge) and the renderer (blockyrender).
package pipeline

import (
	"image"

	"github.com/hytale-tools/blockymodel-merger/pkg/anim"
	"github.com/hytale-tools/blockymodel-merger/pkg/blocks"
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

// Options controls optional pipeline stages beyond the plain merge.
type Options struct {
	NoTint bool

	// HoldBlock attaches the named block (a Server/Item item ID with a
	// BlockType, e.g. "Soil_Grass") to the character's hand attachment bone.
	// Unless Pose is set, the idle of the item's own animation set is applied.
	HoldBlock string

	// HoldRotate is an optional extra rotation for the held item, in degrees
	// about X, Y, Z. Nil leaves the item in its authored orientation.
	HoldRotate *[3]float64

	// Pose applies frame 0 of a .blockyanim file as a static pose.
	Pose string

	// NoPose leaves the skeleton in bind pose, suppressing the default carry
	// pose of a held block. Use for exports that will be animated at runtime:
	// animation deltas compose against bind pose, so a baked pose would
	// double-apply.
	NoPose bool

	// Packs are external asset pack (mod) roots mirroring the assets.zip
	// layout (Common/ + Server/). They take priority over the base game data
	// when resolving blocks and their textures.
	Packs []string
}

// BuildMergedCharacter loads a character config, merges all resolved accessories
// onto the base player model, tints the textures, packs an atlas, and rewrites
// the merged model's texture-layout offsets into atlas space.
//
// This mirrors the orchestration in cmd/blockymerge so a render and a GLB export
// of the same character share identical geometry and texture coordinates.
func BuildMergedCharacter(charFile string, noTint bool) (*Result, error) {
	return BuildMergedCharacterWithOptions(charFile, Options{NoTint: noTint})
}

// BuildMergedCharacterWithOptions is BuildMergedCharacter with held-block and
// pose support.
func BuildMergedCharacterWithOptions(charFile string, opts Options) (*Result, error) {
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

	for _, issue := range charData.Sanitize(reg, gradientSets) {
		util.Logger.Warn("Invalid character value", "issue", issue.String())
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

	var heldBlock *blocks.Definition
	var heldTexture image.Image
	if opts.HoldBlock != "" {
		heldBlock, err = blocks.Find(opts.HoldBlock, blocks.Sources(opts.Packs))
		if err != nil {
			return nil, err
		}
		var rotate *blockymodel.Quaternion
		if opts.HoldRotate != nil {
			q := blocks.EulerToQuaternion(opts.HoldRotate[0], opts.HoldRotate[1], opts.HoldRotate[2])
			rotate = &q
		}
		heldModel, heldTex, err := heldBlock.BuildHeld(rotate)
		if err != nil {
			return nil, err
		}
		heldTexture = heldTex
		if err := m.Merge(heldModel, blocks.AtlasKey); err != nil {
			return nil, err
		}
	}

	model := m.Result()

	if pose := resolvePose(opts, heldBlock); pose != "" {
		poseAnim, err := anim.Load(pose)
		if err != nil {
			return nil, err
		}
		poseAnim.ApplyPose(model)
	}

	res := &Result{Model: model}

	if opts.NoTint {
		return res, nil
	}

	tintedTextures := buildTintedTextures(charData, resolved, gradientSets)
	if heldTexture != nil {
		tintedTextures = append(tintedTextures, &texture.TintedTexture{Name: blocks.AtlasKey, Image: heldTexture})
	}
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

// resolvePose picks the pose file: an explicit Pose wins; a held block
// defaults to the idle of its own animation set (PlayerAnimationsId).
func resolvePose(opts Options, heldBlock *blocks.Definition) string {
	if opts.NoPose {
		return ""
	}
	if opts.Pose != "" {
		return opts.Pose
	}
	if heldBlock != nil {
		pose, err := heldBlock.CarryPose()
		if err != nil {
			util.Logger.Warn("Carry pose not found; rendering unposed",
				"block", heldBlock.ID, "animationSet", heldBlock.AnimationsID, "error", err)
			return ""
		}
		return pose
	}
	return ""
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
