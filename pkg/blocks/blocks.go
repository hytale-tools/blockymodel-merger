// Package blocks loads Hytale block item definitions and builds a held-block
// accessory that the merger can attach to a character's hand.
//
// Block definitions live in the server assets under Server/Item/Items/** (one
// JSON per item; blocks are the ones with a BlockType). Face textures are
// referenced relative to Common/ (e.g. "BlockTextures/Soil_Dirt.png") and may
// be greyscale images that the game tints at runtime (TintUp) plus an optional
// tinted overlay mask for the side faces (TextureSideMask).
package blocks

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
	"github.com/hytale-tools/blockymodel-merger/pkg/render"
	"github.com/hytale-tools/blockymodel-merger/pkg/texture"
	"github.com/hytale-tools/blockymodel-merger/pkg/util"
)

const (
	// AtlasKey identifies the held block in the merger's node sources and the
	// texture atlas. Prefixed like AccessoryPath.Key() to avoid collisions.
	AtlasKey = "heldItem:block"

	// HeldItemNodeName is the group node every held item is wrapped in,
	// directly under the R-Attachment bone. Viewers can toggle this one node
	// to show/hide the held item regardless of what it is, and the renderer
	// excludes it from auto-fit framing.
	HeldItemNodeName = render.HeldItemNodeName

	// DefaultAnimationsID is the animation set used when an item definition
	// has no PlayerAnimationsId (the generic main-hand item animations).
	DefaultAnimationsID = "Item"

	// CubeSize matches assets/Characters/Empty_Cube.blockymodel, the model the
	// game attaches when a block is held: a 32^3 box, so each face samples a
	// native 32x32 block texture.
	CubeSize = 32
)

// Sentinel errors for embedders to classify failures with errors.Is: whether
// a lookup failed because of the caller's input or the deployment.
var (
	// ErrNotFound reports an unknown item ID: caller error.
	ErrNotFound = errors.New("item not found")

	// ErrNotRenderable reports a known item without renderable block data:
	// caller error.
	ErrNotRenderable = errors.New("not a renderable block")

	// ErrSourcesUnavailable reports that no item source directory exists at
	// all - the assets were never extracted: server misconfiguration.
	ErrSourcesUnavailable = errors.New("item sources unavailable")
)

// Source is one place block definitions and their textures can come from.
type Source struct {
	ItemsDir      string // directory holding item JSONs (searched recursively)
	CommonDir     string // directory texture paths like "BlockTextures/x.png" resolve against
	AnimationsDir string // directory holding animation set JSONs (Server/Item/Animations)
}

// DefaultSource is the base game data as laid out by extract-assets.
var DefaultSource = Source{
	ItemsDir:      filepath.Join("data", "Items"),
	CommonDir:     "assets",
	AnimationsDir: filepath.Join("data", "Animations"),
}

// PackSource returns the Source for an external asset pack (mod) directory
// that mirrors the assets.zip layout (Common/ + Server/ at its root).
func PackSource(root string) Source {
	return Source{
		ItemsDir:      filepath.Join(root, "Server", "Item", "Items"),
		CommonDir:     filepath.Join(root, "Common"),
		AnimationsDir: filepath.Join(root, "Server", "Item", "Animations"),
	}
}

// Sources builds the search order for the given pack roots: packs first (so
// mods win), then the base game data.
func Sources(packRoots []string) []Source {
	var s []Source
	for _, root := range packRoots {
		s = append(s, PackSource(root))
	}
	return append(s, DefaultSource)
}

// faceTextures mirrors one entry of BlockType.Textures in an item definition.
type faceTextures struct {
	All   string `json:"All"`
	Down  string `json:"Down"`
	Sides string `json:"Sides"`
	Up    string `json:"Up"`
}

type blockType struct {
	DrawType        string         `json:"DrawType"`
	Textures        []faceTextures `json:"Textures"`
	TintUp          []string       `json:"TintUp"`
	TextureSideMask string         `json:"TextureSideMask"`
	CustomModel     string         `json:"CustomModel"`
	CustomModelTexture []struct {
		Texture string `json:"Texture"`
	} `json:"CustomModelTexture"`
}

type itemDef struct {
	Parent             string     `json:"Parent"`
	BlockType          *blockType `json:"BlockType"`
	PlayerAnimationsId string     `json:"PlayerAnimationsId"`
	Scale              float64    `json:"Scale"` // held-item scale multiplier (0 = unset)
}

// Definition is a resolved block definition ready for compositing.
type Definition struct {
	ID string

	// AnimationsID is the item's PlayerAnimationsId: the name of the
	// character animation set used while holding it. The set's handedness is
	// determined by which folder under Characters/Animations/Items contains
	// it (e.g. "Block" -> Dual_Handed/Block, "Item" -> Main_Handed/Item).
	AnimationsID string

	// Scale is the item's held-item scale multiplier (1 when unset), e.g. the
	// pillow-mod dakis inherit Scale 2.0 from their template.
	Scale float64

	source Source   // where the definition was found
	all    []Source // full search order, for resolving textures
	block  *blockType
}

// animSet mirrors a Server/Item/Animations/<Id>.json animation set: named
// animation states plus a Parent set that missing states are inherited from
// (e.g. Mace has only attack states and inherits Idle from Battleaxe).
type animSet struct {
	Parent     string `json:"Parent"`
	Animations map[string]struct {
		ThirdPerson string `json:"ThirdPerson"`
	} `json:"Animations"`
}

// handedness folders under Characters/Animations/Items, in search order. Used
// only as a fallback when the animation set registry isn't extracted.
var handedDirs = []string{"Dual_Handed", "Main_Handed", "Off_Handed"}

// CarryPose resolves the Idle pose of this item's animation set from the
// Server/Item/Animations registry, following each set's Parent chain the way
// the game does (e.g. daki -> Mace -> Battleaxe's two-handed idle). If the
// registry isn't available it falls back to scanning the handedness folders
// for <AnimationsID>/Idle.blockyanim.
func (d *Definition) CarryPose() (string, error) {
	seen := map[string]bool{}
	for id := d.AnimationsID; id != "" && !seen[id]; {
		seen[id] = true
		set, err := d.loadAnimSet(id)
		if err != nil {
			break // registry not extracted - fall back to folder scan
		}
		if idle, ok := set.Animations["Idle"]; ok && idle.ThirdPerson != "" {
			return d.resolveFile(idle.ThirdPerson)
		}
		id = set.Parent
	}

	for _, id := range []string{d.AnimationsID, DefaultAnimationsID} {
		for _, src := range d.all {
			for _, handed := range handedDirs {
				p := filepath.Join(src.CommonDir, "Characters", "Animations", "Items",
					handed, filepath.FromSlash(id), "Idle.blockyanim")
				if _, err := os.Stat(p); err == nil {
					return p, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no idle animation found for set %q (or fallback %q) in any source",
		d.AnimationsID, DefaultAnimationsID)
}

func (d *Definition) loadAnimSet(id string) (*animSet, error) {
	for _, src := range append([]Source{d.source}, d.all...) {
		p := filepath.Join(src.AnimationsDir, id+".json")
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var set animSet
		if err := json.Unmarshal(data, &set); err != nil {
			return nil, fmt.Errorf("failed to parse animation set %s: %w", p, err)
		}
		return &set, nil
	}
	return nil, fmt.Errorf("animation set %q not found in any source", id)
}

// Find locates a block item definition by ID (the JSON's basename, e.g.
// "Soil_Grass") across the given sources, first match wins. Fields missing
// from the definition (PlayerAnimationsId, BlockType) are inherited by
// walking its Parent chain - which may cross between packs and the base game
// (e.g. a mod's Template_Daki inherits the base Template_Weapon_Mace).
func Find(id string, sources []Source) (*Definition, error) {
	def, src, path, err := loadItemDef(id, sources)
	if err != nil {
		return nil, err
	}

	// Inherit missing fields from the Parent chain.
	seen := map[string]bool{id: true}
	for parent := def.Parent; parent != "" && (def.PlayerAnimationsId == "" || def.BlockType == nil || def.Scale == 0); {
		if seen[parent] {
			util.Logger.Warn("Cycle in item Parent chain", "block", id, "parent", parent)
			break
		}
		seen[parent] = true
		p, _, _, err := loadItemDef(parent, sources)
		if err != nil {
			util.Logger.Warn("Parent item not found; ignoring rest of chain",
				"block", id, "parent", parent, "error", err)
			break
		}
		if def.PlayerAnimationsId == "" {
			def.PlayerAnimationsId = p.PlayerAnimationsId
		}
		if def.BlockType == nil {
			def.BlockType = p.BlockType
		}
		if def.Scale == 0 {
			def.Scale = p.Scale
		}
		parent = p.Parent
	}

	if def.BlockType == nil || (len(def.BlockType.Textures) == 0 && def.BlockType.CustomModel == "") {
		return nil, fmt.Errorf("item %q (%s): %w", id, path, ErrNotRenderable)
	}
	if def.BlockType.DrawType != "Cube" && def.BlockType.CustomModel == "" {
		util.Logger.Warn("Block is not a plain cube; rendering it as one",
			"block", id, "drawType", def.BlockType.DrawType)
	}
	animID := def.PlayerAnimationsId
	if animID == "" {
		animID = DefaultAnimationsID
	}
	scale := def.Scale
	if scale == 0 {
		scale = 1
	}
	return &Definition{
		ID:           id,
		AnimationsID: animID,
		Scale:        scale,
		source:       src,
		all:          sources,
		block:        def.BlockType,
	}, nil
}

// loadItemDef reads the item JSON named <id>.json from the first source
// containing it.
func loadItemDef(id string, sources []Source) (*itemDef, Source, string, error) {
	target := id + ".json"
	for _, src := range sources {
		var found string
		err := filepath.WalkDir(src.ItemsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // unreadable subtree - keep searching
			}
			if !d.IsDir() && d.Name() == target {
				found = path
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil || found == "" {
			continue
		}

		data, err := os.ReadFile(found)
		if err != nil {
			return nil, Source{}, "", fmt.Errorf("failed to read item definition %s: %w", found, err)
		}
		var def itemDef
		if err := json.Unmarshal(data, &def); err != nil {
			return nil, Source{}, "", fmt.Errorf("failed to parse item definition %s: %w", found, err)
		}
		return &def, src, found, nil
	}
	// Distinguish an unknown ID from missing assets: if no items directory
	// exists at all, the assets were never extracted.
	if !anyItemsDirExists(sources) {
		return nil, Source{}, "", fmt.Errorf("no item source directory exists (looked under %s): %w",
			itemsDirs(sources), ErrSourcesUnavailable)
	}
	return nil, Source{}, "", fmt.Errorf("item %q not found in any source (looked for %s under %s): %w",
		id, target, itemsDirs(sources), ErrNotFound)
}

func anyItemsDirExists(sources []Source) bool {
	for _, s := range sources {
		if info, err := os.Stat(s.ItemsDir); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func itemsDirs(sources []Source) string {
	var dirs []string
	for _, s := range sources {
		dirs = append(dirs, s.ItemsDir)
	}
	return strings.Join(dirs, ", ")
}

// resolveFile resolves a Common-relative path ("BlockTextures/x.png",
// "Blocks/.../x.blockymodel") to a file, preferring the definition's own
// source, then the remaining sources - mod blocks may reference base-game
// assets and vice versa.
func (d *Definition) resolveFile(rel string) (string, error) {
	tryOrder := append([]Source{d.source}, d.all...)
	for _, src := range tryOrder {
		p := filepath.Join(src.CommonDir, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("asset %q not found in any source", rel)
}

// BuildHeld returns the held-item accessory to merge onto the character and
// the texture image to pack into the atlas (keyed by AtlasKey).
//
// Cube blocks become an Empty_Cube-style 32^3 box textured with a composed
// face strip; DrawType=Model blocks attach their CustomModel with its own
// texture, in the model's authored orientation. rotate, if non-nil, is an
// explicit extra rotation applied to the HeldItem wrapper node.
func (d *Definition) BuildHeld(rotate *blockymodel.Quaternion) (*blockymodel.BlockyModel, image.Image, error) {
	model, img, err := d.buildHeld()
	if err != nil {
		return nil, nil, err
	}
	if rotate != nil {
		if held := findNode(model.Nodes, HeldItemNodeName); held != nil {
			held.Orientation = rotate
		}
	}
	return model, img, nil
}

func (d *Definition) buildHeld() (*blockymodel.BlockyModel, image.Image, error) {
	if d.block.CustomModel != "" {
		return d.buildCustomModel()
	}
	strip, err := d.ComposeStrip()
	if err != nil {
		return nil, nil, err
	}
	return HeldAccessory(), strip, nil
}

// EulerToQuaternion converts degrees about X, Y, Z (applied in that order)
// into a quaternion, for user-supplied held-item rotations.
func EulerToQuaternion(xDeg, yDeg, zDeg float64) blockymodel.Quaternion {
	q := axisAngle(zDeg, 0, 0, 1)
	q = qMul(q, axisAngle(yDeg, 0, 1, 0))
	q = qMul(q, axisAngle(xDeg, 1, 0, 0))
	return q
}

func axisAngle(deg, x, y, z float64) blockymodel.Quaternion {
	r := deg * math.Pi / 360
	s := math.Sin(r)
	return blockymodel.Quaternion{W: math.Cos(r), X: x * s, Y: y * s, Z: z * s}
}

func qMul(a, b blockymodel.Quaternion) blockymodel.Quaternion {
	return blockymodel.Quaternion{
		W: a.W*b.W - a.X*b.X - a.Y*b.Y - a.Z*b.Z,
		X: a.W*b.X + a.X*b.W + a.Y*b.Z - a.Z*b.Y,
		Y: a.W*b.Y - a.X*b.Z + a.Y*b.W + a.Z*b.X,
		Z: a.W*b.Z + a.X*b.Y - a.Y*b.X + a.Z*b.W,
	}
}

func findNode(nodes []blockymodel.Node, name string) *blockymodel.Node {
	for i := range nodes {
		if nodes[i].Name == name {
			return &nodes[i]
		}
		if n := findNode(nodes[i].Children, name); n != nil {
			return n
		}
	}
	return nil
}

func (d *Definition) buildCustomModel() (*blockymodel.BlockyModel, image.Image, error) {
	modelPath, err := d.resolveFile(d.block.CustomModel)
	if err != nil {
		return nil, nil, fmt.Errorf("block %q: %w", d.ID, err)
	}
	model, err := blockymodel.Load(modelPath)
	if err != nil {
		return nil, nil, fmt.Errorf("block %q custom model: %w", d.ID, err)
	}

	if len(d.block.CustomModelTexture) == 0 {
		return nil, nil, fmt.Errorf("block %q has a custom model but no CustomModelTexture", d.ID)
	}
	texPath, err := d.resolveFile(d.block.CustomModelTexture[0].Texture)
	if err != nil {
		return nil, nil, fmt.Errorf("block %q: %w", d.ID, err)
	}
	img, err := texture.LoadImage(texPath, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("block %q custom model texture: %w", d.ID, err)
	}
	// The game ignores the custom model's root node transform when attaching
	// it as a held item - root transforms are placement/editor artifacts (the
	// pillow-mod daki's root carries a rotation+offset that would otherwise
	// sink it into the character). Verified against in-game screenshots.
	for i := range model.Nodes {
		model.Nodes[i].Position = &blockymodel.Vec3{}
		model.Nodes[i].Orientation = &blockymodel.Quaternion{W: 1}
	}
	if d.Scale != 1 {
		scaleNodes(model.Nodes, d.Scale)
	}
	return wrapHeldItem(model.Nodes), img, nil
}

// scaleNodes scales a node subtree uniformly: positions and shape offsets move
// outward, shape stretch grows the geometry (UVs are unaffected - texture
// layouts use pre-stretch sizes).
func scaleNodes(nodes []blockymodel.Node, s float64) {
	for i := range nodes {
		n := &nodes[i]
		if n.Position != nil {
			n.Position.X *= s
			n.Position.Y *= s
			n.Position.Z *= s
		}
		if n.Shape != nil {
			if n.Shape.Offset != nil {
				n.Shape.Offset.X *= s
				n.Shape.Offset.Y *= s
				n.Shape.Offset.Z *= s
			}
			if n.Shape.Stretch != nil {
				n.Shape.Stretch.X *= s
				n.Shape.Stretch.Y *= s
				n.Shape.Stretch.Z *= s
			} else if n.Shape.Type != "none" {
				n.Shape.Stretch = &blockymodel.Vec3{X: s, Y: s, Z: s}
			}
		}
		scaleNodes(n.Children, s)
	}
}

// wrapHeldItem parents the given geometry under R-Attachment inside a single
// HeldItemNodeName group node.
func wrapHeldItem(nodes []blockymodel.Node) *blockymodel.BlockyModel {
	return &blockymodel.BlockyModel{
		Nodes: []blockymodel.Node{{
			ID:   "0",
			Name: "R-Attachment",
			Shape: &blockymodel.Shape{
				Type:     "none",
				Settings: map[string]interface{}{"isPiece": true},
			},
			Children: []blockymodel.Node{{
				ID:          "1",
				Name:        HeldItemNodeName,
				Position:    &blockymodel.Vec3{},
				Orientation: &blockymodel.Quaternion{W: 1},
				Shape:       &blockymodel.Shape{Type: "none"},
				Children:    nodes,
			}},
		}},
	}
}

// ComposeStrip renders the block's faces into a horizontal (3*CubeSize) x
// CubeSize strip: [top][sides][bottom], applying the game's tint rules.
func (d *Definition) ComposeStrip() (*image.RGBA, error) {
	faces := d.block.Textures[0]

	upPath, sidesPath, downPath := faces.Up, faces.Sides, faces.Down
	if faces.All != "" {
		upPath, sidesPath, downPath = faces.All, faces.All, faces.All
	}
	if upPath == "" || sidesPath == "" || downPath == "" {
		return nil, fmt.Errorf("block %q has incomplete face textures", d.ID)
	}

	top, err := d.loadFace(upPath)
	if err != nil {
		return nil, err
	}
	sides, err := d.loadFace(sidesPath)
	if err != nil {
		return nil, err
	}
	bottom, err := d.loadFace(downPath)
	if err != nil {
		return nil, err
	}

	// Greyscale top textures are tinted with the block's TintUp colour
	// (in-game this is the biome tint; TintUp is the block's base colour).
	if len(d.block.TintUp) > 0 {
		r, g, b, err := parseHexColor(d.block.TintUp[0])
		if err != nil {
			return nil, fmt.Errorf("block %q TintUp: %w", d.ID, err)
		}
		tintGreyscale(top, r, g, b)

		// The side mask is an alpha overlay (e.g. the grass fringe) drawn
		// over the side texture with the same tint.
		if d.block.TextureSideMask != "" {
			mask, err := d.loadFace(d.block.TextureSideMask)
			if err != nil {
				return nil, err
			}
			overlayTinted(sides, mask, r, g, b)
		}
	}

	strip := image.NewRGBA(image.Rect(0, 0, 3*CubeSize, CubeSize))
	blit(strip, top, 0)
	blit(strip, sides, CubeSize)
	blit(strip, bottom, 2*CubeSize)
	return strip, nil
}

// HeldAccessory builds an accessory model that the merger attaches to the
// R-Attachment bone (the in-game held-item grip): a CubeSize^3 cube whose
// faces point into a ComposeStrip laid out at texture offset (0,0). The
// pipeline's atlas-offset pass shifts the layout to the strip's final
// position, keyed by AtlasKey.
func HeldAccessory() *blockymodel.BlockyModel {
	visible := true
	face := func(x float64) blockymodel.TextureFace {
		return blockymodel.TextureFace{Offset: blockymodel.Vec2{X: x}}
	}
	cube := blockymodel.Node{
		ID:          "2",
		Name:        "HeldBlock",
		Position:    &blockymodel.Vec3{},
		Orientation: &blockymodel.Quaternion{W: 1},
		Shape: &blockymodel.Shape{
			Type:    "box",
			Offset:  &blockymodel.Vec3{},
			Stretch: &blockymodel.Vec3{X: 1, Y: 1, Z: 1},
			Settings: map[string]interface{}{
				"size": map[string]interface{}{
					"x": float64(CubeSize), "y": float64(CubeSize), "z": float64(CubeSize),
				},
			},
			UnwrapMode: "custom",
			Visible:    &visible,
			TextureLayout: map[string]blockymodel.TextureFace{
				"top":    face(0),
				"front":  face(CubeSize),
				"back":   face(CubeSize),
				"left":   face(CubeSize),
				"right":  face(CubeSize),
				"bottom": face(2 * CubeSize),
			},
		},
	}
	return wrapHeldItem([]blockymodel.Node{cube})
}

// loadFace loads a face texture and normalises it to CubeSize x CubeSize RGBA.
func (d *Definition) loadFace(rel string) (*image.RGBA, error) {
	path, err := d.resolveFile(rel)
	if err != nil {
		return nil, err
	}
	img, err := texture.LoadImage(path, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to load block texture %s: %w", path, err)
	}
	return resample(img, CubeSize), nil
}

// resample converts img to a size x size RGBA, box-averaging on downscale and
// nearest-neighbour otherwise.
func resample(img image.Image, size int) *image.RGBA {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	k := b.Dx() / size
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, bl, a uint32
			if k > 1 && b.Dx()%size == 0 && b.Dy() == b.Dx() {
				for dy := 0; dy < k; dy++ {
					for dx := 0; dx < k; dx++ {
						sr, sg, sb, sa := img.At(b.Min.X+x*k+dx, b.Min.Y+y*k+dy).RGBA()
						r += sr >> 8
						g += sg >> 8
						bl += sb >> 8
						a += sa >> 8
					}
				}
				n := uint32(k * k)
				r, g, bl, a = r/n, g/n, bl/n, a/n
			} else {
				sr, sg, sb, sa := img.At(b.Min.X+x*b.Dx()/size, b.Min.Y+y*b.Dy()/size).RGBA()
				r, g, bl, a = sr>>8, sg>>8, sb>>8, sa>>8
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = uint8(r)
			dst.Pix[i+1] = uint8(g)
			dst.Pix[i+2] = uint8(bl)
			dst.Pix[i+3] = uint8(a)
		}
	}
	return dst
}

// tintGreyscale multiplies a greyscale image by a flat tint colour in place.
func tintGreyscale(img *image.RGBA, r, g, b int) {
	for i := 0; i < len(img.Pix); i += 4 {
		grey := int(img.Pix[i])
		img.Pix[i+0] = uint8(grey * r / 255)
		img.Pix[i+1] = uint8(grey * g / 255)
		img.Pix[i+2] = uint8(grey * b / 255)
	}
}

// overlayTinted alpha-blends a greyscale mask (tinted) over dst in place.
func overlayTinted(dst, mask *image.RGBA, r, g, b int) {
	for i := 0; i < len(dst.Pix); i += 4 {
		a := int(mask.Pix[i+3])
		if a == 0 {
			continue
		}
		grey := int(mask.Pix[i])
		or, og, ob := grey*r/255, grey*g/255, grey*b/255
		dst.Pix[i+0] = uint8((or*a + int(dst.Pix[i+0])*(255-a)) / 255)
		dst.Pix[i+1] = uint8((og*a + int(dst.Pix[i+1])*(255-a)) / 255)
		dst.Pix[i+2] = uint8((ob*a + int(dst.Pix[i+2])*(255-a)) / 255)
	}
}

func blit(dst *image.RGBA, src *image.RGBA, xOff int) {
	for y := 0; y < CubeSize; y++ {
		copy(dst.Pix[dst.PixOffset(xOff, y):dst.PixOffset(xOff+CubeSize, y)],
			src.Pix[src.PixOffset(0, y):src.PixOffset(CubeSize, y)])
	}
}

func parseHexColor(s string) (r, g, b int, err error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return 0, 0, 0, fmt.Errorf("invalid hex colour %q", s)
	}
	var v int64
	if _, err := fmt.Sscanf(s, "%x", &v); err != nil {
		return 0, 0, 0, fmt.Errorf("invalid hex colour %q: %w", s, err)
	}
	return int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff), nil
}
