// Package pipeline factors the merge + tint + atlas orchestration so it can be
// shared between the GLB exporter (blockymerge) and the renderer (blockyrender).
package pipeline

import (
	"fmt"
	"image"
	"log/slog"
	"strings"
	"sync"

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

// Result holds a fully merged character ready to render or export, plus the
// diagnostics produced while building it.
type Result struct {
	Model *blockymodel.BlockyModel
	Atlas *texture.Atlas // may be nil if texturing was skipped

	// Character is the effective config that was built: the caller's input
	// after defaulting (Options.ApplyDefaults) and Sanitize repairs. The
	// input CharacterData is never modified.
	Character *character.CharacterData

	// Issues are the repairs Sanitize made to invalid config values.
	Issues []character.ValidationIssue

	// Warnings are non-fatal problems: accessories that could not be
	// resolved, textures that failed to load, a missing carry pose, or a
	// failed atlas pack. The build proceeded without the affected part.
	Warnings []string
}

// Options controls optional pipeline stages beyond the plain merge.
type Options struct {
	NoTint bool

	// ApplyDefaults fills required slots that are nil or empty (face, eyes,
	// underwear, ...) with the registry's IsDefaultAsset entries before
	// sanitizing. Off by default so embedders opt in explicitly; the CLIs
	// enable it unless -no-defaults is passed.
	ApplyDefaults bool

	// HoldBlock attaches the named block (a Server/Item item ID with a
	// BlockType, e.g. "Soil_Grass") to the character's hand attachment bone.
	// Unless Pose is set, the idle of the item's own animation set is applied.
	HoldBlock string

	// HoldRotate is an optional extra rotation for the held item, in degrees
	// about X, Y, Z. Nil leaves the item in its authored orientation.
	HoldRotate *[3]float64

	// HoldOffset is an optional extra translation for the held item, in model
	// units within the hand attachment frame. Nil grafts the item at the
	// attachment origin, which is right for models authored around their own
	// origin and wrong for ones that are not (a player-head item model keeps
	// the character rig it was cut from, so its geometry sits well above the
	// model origin).
	HoldOffset *[3]float64

	// HoldScale multiplies the held item's own scale. Zero means no extra
	// scaling. Use it to size an item to the pose that carries it - the
	// carry pose's grip is authored for a one-block item.
	HoldScale float64

	// Pose applies frame 0 of a .blockyanim file as a static pose.
	// Precedence: PoseAnim > Pose > NoPose > held-block carry pose.
	Pose string

	// PoseAnim is a pre-parsed pose applied as Pose would be. It takes
	// precedence over Pose and over the default carry pose of a held block.
	// Lets a long-lived embedder parse its pose set once at startup.
	PoseAnim *anim.Animation

	// NoPose leaves the skeleton in bind pose, suppressing the default carry
	// pose of a held block. Use for exports that will be animated at runtime:
	// animation deltas compose against bind pose, so a baked pose would
	// double-apply.
	NoPose bool

	// Packs are external asset pack (mod) roots mirroring the assets.zip
	// layout (Common/ + Server/). They take priority over the base game data
	// when resolving blocks and their textures.
	Packs []string

	// Hide names merged nodes whose geometry is dropped, along with the
	// geometry of everything under them - e.g. []string{"Head"} renders the
	// character headless (head, face, hair and any head accessory), which is
	// what a "carrying your own head" render needs. The bones stay in place,
	// so nothing else moves. Names that match no node produce a warning.
	Hide []string
}

// Builder builds merged characters while caching everything immutable across
// builds: the registry, gradient sets, base model, parsed accessory models,
// and tinted texture images. A long-lived process (e.g. an API server) should
// create one Builder and reuse it - repeat builds then skip all file loading
// and tinting except for accessories and colors not seen before.
//
// Builder is safe for concurrent use.
type Builder struct {
	gradientSets *texture.GradientSets
	reg          *registry.Registry
	base         *blockymodel.BlockyModel
	logger       *slog.Logger

	mu     sync.RWMutex
	models map[string]*blockymodel.BlockyModel // accessory path -> parsed model
	images map[string]image.Image              // tint key -> tinted image
	blocks map[string]*blocks.Definition       // held-block cache, key: id + pack set
	anims  map[string]*anim.Animation          // pose cache, key: file path
}

// BuilderOption configures a Builder at construction.
type BuilderOption func(*Builder)

// WithLogger routes the Builder's log output to l instead of the global
// util.Logger. Build-scoped diagnostics are returned on Result (see
// Result.Issues and Result.Warnings), so this covers only residual logging.
func WithLogger(l *slog.Logger) BuilderOption {
	return func(b *Builder) { b.logger = l }
}

// NewBuilder loads the registry, gradient sets, and base player model.
func NewBuilder(opts ...BuilderOption) (*Builder, error) {
	b := &Builder{
		logger: util.Logger,
		models: make(map[string]*blockymodel.BlockyModel),
		images: make(map[string]image.Image),
		blocks: make(map[string]*blocks.Definition),
		anims:  make(map[string]*anim.Animation),
	}
	for _, opt := range opts {
		opt(b)
	}

	gradientSets, err := texture.LoadGradientSets()
	if err != nil {
		b.logger.Warn("Could not load gradient sets", "error", err)
	}

	reg, err := registry.Load()
	if err != nil {
		return nil, err
	}

	base, err := blockymodel.Load(BasePath)
	if err != nil {
		return nil, fmt.Errorf("base player model: %w", err)
	}

	b.gradientSets = gradientSets
	b.reg = reg
	b.base = base
	return b, nil
}

// BuildFile builds a character from a config file path.
func (b *Builder) BuildFile(charFile string, opts Options) (*Result, error) {
	charData, err := character.Load(charFile)
	if err != nil {
		return nil, err
	}
	return b.Build(charData, opts)
}

// Build sanitizes a copy of the character config, merges all resolved
// accessories onto the base player model, tints the textures, packs an atlas,
// and rewrites the merged model's texture-layout offsets into atlas space.
// The caller's CharacterData is never modified; the effective config and all
// diagnostics come back on the Result.
//
// Errors can be classified with errors.Is for status-code mapping:
// registry.ErrNotFound, blocks.ErrNotFound, and blocks.ErrNotRenderable mean
// bad caller input; registry.ErrRegistryUnavailable, blocks.ErrSourcesUnavailable,
// and fs.ErrNotExist from NewBuilder mean the deployment is missing assets.
//
// This mirrors the orchestration in cmd/blockymerge so a render and a GLB
// export of the same character share identical geometry and texture
// coordinates.
func (b *Builder) Build(charData *character.CharacterData, opts Options) (*Result, error) {
	charData = charData.Clone()
	if opts.ApplyDefaults {
		charData.ApplyDefaults(b.reg, b.gradientSets)
	}

	res := &Result{
		Character: charData,
		Issues:    charData.Sanitize(b.reg, b.gradientSets),
	}

	resolved, err := charData.ResolveAccessories(b.reg)
	if err != nil {
		return nil, err
	}
	res.Warnings = append(res.Warnings, resolved.Warnings...)

	m, err := merger.New(b.base)
	if err != nil {
		return nil, err
	}

	for _, acc := range resolved.Accessories {
		accessory, err := b.model(acc.Path)
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
		heldBlock, err = b.heldBlock(opts.HoldBlock, opts.Packs)
		if err != nil {
			return nil, err
		}
		var rotate *blockymodel.Quaternion
		if opts.HoldRotate != nil {
			q := blocks.EulerToQuaternion(opts.HoldRotate[0], opts.HoldRotate[1], opts.HoldRotate[2])
			rotate = &q
		}
		var offset *blockymodel.Vec3
		if opts.HoldOffset != nil {
			offset = &blockymodel.Vec3{X: opts.HoldOffset[0], Y: opts.HoldOffset[1], Z: opts.HoldOffset[2]}
		}
		heldModel, heldTex, err := heldBlock.BuildHeld(blocks.HeldTransform{
			Rotate: rotate, Offset: offset, Scale: opts.HoldScale,
		})
		if err != nil {
			return nil, err
		}
		heldTexture = heldTex
		if err := m.Merge(heldModel, blocks.AtlasKey); err != nil {
			return nil, err
		}
	}

	model := m.Result()
	res.Model = model

	// Hide after merging so a hidden bone takes its accessories with it (the
	// haircut and head accessories merge into the Head subtree).
	if len(opts.Hide) > 0 {
		for _, name := range blockymodel.HideSubtrees(model.Nodes, opts.Hide) {
			res.Warnings = append(res.Warnings, fmt.Sprintf("no node named %q to hide", name))
		}
	}

	poseAnim := opts.PoseAnim
	if poseAnim == nil {
		posePath, warn := resolvePose(opts, heldBlock)
		if warn != "" {
			res.Warnings = append(res.Warnings, warn)
		}
		if posePath != "" {
			if poseAnim, err = b.anim(posePath); err != nil {
				return nil, err
			}
		}
	}
	if poseAnim != nil {
		poseAnim.ApplyPose(model)
	}

	if opts.NoTint {
		return res, nil
	}

	tintedTextures, warnings := b.tintedTextures(charData, resolved)
	res.Warnings = append(res.Warnings, warnings...)
	if heldTexture != nil {
		tintedTextures = append(tintedTextures, &texture.TintedTexture{Name: blocks.AtlasKey, Image: heldTexture})
	}
	if len(tintedTextures) == 0 {
		return res, nil
	}

	atlas, err := texture.PackAtlasSimple(tintedTextures, 1)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("failed to pack atlas: %v", err))
		return res, nil
	}
	res.Atlas = atlas

	applyAtlasOffsets(model, m, atlas, tintedTextures)
	return res, nil
}

// BuildMergedCharacter is the one-shot form of Builder.Build: it loads
// everything from disk, builds the character, and discards the caches. Use a
// Builder directly when building more than one character.
func BuildMergedCharacter(charFile string, noTint bool) (*Result, error) {
	return BuildMergedCharacterWithOptions(charFile, Options{NoTint: noTint})
}

// BuildMergedCharacterWithOptions is BuildMergedCharacter with held-block and
// pose support.
func BuildMergedCharacterWithOptions(charFile string, opts Options) (*Result, error) {
	b, err := NewBuilder()
	if err != nil {
		return nil, err
	}
	return b.BuildFile(charFile, opts)
}

// model returns the parsed accessory model at path, loading it on first use.
// Merging only clones from accessory models, so the cached model is shared.
func (b *Builder) model(path string) (*blockymodel.BlockyModel, error) {
	b.mu.RLock()
	cached, ok := b.models[path]
	b.mu.RUnlock()
	if ok {
		return cached, nil
	}

	model, err := blockymodel.Load(path)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.models[path] = model
	b.mu.Unlock()
	return model, nil
}

// image returns the (possibly tinted) texture image for key, computing it via
// load on first use. Atlas packing only reads pixels, so images are shared.
func (b *Builder) image(key string, load func() (image.Image, error)) (image.Image, error) {
	b.mu.RLock()
	cached, ok := b.images[key]
	b.mu.RUnlock()
	if ok {
		return cached, nil
	}

	img, err := load()
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.images[key] = img
	b.mu.Unlock()
	return img, nil
}

// heldBlock returns the block definition for id, resolving it on first use.
// Packs are part of the cache key: the same id can resolve differently under
// different mod packs, so a bare-id key would leak a modded block into an
// unmodded build. BuildHeld does not mutate the definition, so it is shared.
func (b *Builder) heldBlock(id string, packs []string) (*blocks.Definition, error) {
	key := blockCacheKey(id, packs)

	b.mu.RLock()
	cached, ok := b.blocks[key]
	b.mu.RUnlock()
	if ok {
		return cached, nil
	}

	def, err := blocks.Find(id, blocks.Sources(packs))
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.blocks[key] = def
	b.mu.Unlock()
	return def, nil
}

// blockCacheKey joins id and packs with NUL, which cannot appear in item IDs
// or paths, so distinct (id, packs) pairs never collide.
func blockCacheKey(id string, packs []string) string {
	return id + "\x00" + strings.Join(packs, "\x00")
}

// anim returns the parsed .blockyanim at path, loading it on first use.
// ApplyPose mutates only the target model, so a cached Animation is shared.
func (b *Builder) anim(path string) (*anim.Animation, error) {
	b.mu.RLock()
	cached, ok := b.anims[path]
	b.mu.RUnlock()
	if ok {
		return cached, nil
	}

	a, err := anim.Load(path)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.anims[path] = a
	b.mu.Unlock()
	return a, nil
}

// resolvePose picks the pose file: an explicit Pose always wins; NoPose only
// suppresses the default carry pose of a held block. A non-empty warning
// means the held block's carry pose could not be found and the character
// renders unposed.
func resolvePose(opts Options, heldBlock *blocks.Definition) (string, string) {
	if opts.Pose != "" {
		return opts.Pose, ""
	}
	if opts.NoPose {
		return "", ""
	}
	if heldBlock != nil {
		pose, err := heldBlock.CarryPose()
		if err != nil {
			return "", fmt.Sprintf("carry pose for block %q (animation set %q) not found, rendering unposed: %v",
				heldBlock.ID, heldBlock.AnimationsID, err)
		}
		return pose, ""
	}
	return "", ""
}

func (b *Builder) tintedTextures(
	charData *character.CharacterData,
	resolved *character.ResolveResult,
) ([]*texture.TintedTexture, []string) {
	var tinted []*texture.TintedTexture
	var warnings []string

	// Base player texture (skin-toned).
	skinTone := charData.GetSkinTone()
	baseImg, err := b.image("base|"+skinTone, func() (image.Image, error) {
		if skinTone == "" {
			return texture.LoadImage(BaseTexturePath)
		}
		t, err := texture.ProcessAccessoryTexture("_base", BaseTexturePath, "Skin", skinTone, b.gradientSets)
		if err != nil {
			return nil, err
		}
		return t.Image, nil
	})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("could not load base texture: %v", err))
	} else {
		tinted = append(tinted, &texture.TintedTexture{Name: "_base", Image: baseImg, OriginalPath: BaseTexturePath})
	}

	// Accessory textures.
	for _, acc := range resolved.Accessories {
		if acc.ResolvedTexture == nil {
			continue
		}
		switch {
		case acc.ResolvedTexture.DirectTexture != "":
			path := acc.ResolvedTexture.DirectTexture
			img, err := b.image("direct|"+path, func() (image.Image, error) {
				return texture.LoadImage(path)
			})
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to load direct texture for %s: %v", acc.Spec.ID, err))
				continue
			}
			tinted = append(tinted, &texture.TintedTexture{Name: acc.Key(), Image: img, OriginalPath: path})
		case acc.ResolvedTexture.GreyscaleTexture != "":
			path := acc.ResolvedTexture.GreyscaleTexture
			set := acc.ResolvedTexture.GradientSet
			color := acc.Spec.Color
			img, err := b.image("grey|"+path+"|"+set+"|"+color, func() (image.Image, error) {
				t, err := texture.ProcessAccessoryTexture(acc.Key(), path, set, color, b.gradientSets)
				if err != nil {
					return nil, err
				}
				return t.Image, nil
			})
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to process accessory texture for %s: %v", acc.Spec.ID, err))
				continue
			}
			tinted = append(tinted, &texture.TintedTexture{Name: acc.Key(), Image: img, OriginalPath: path})
		}
	}

	return tinted, warnings
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
