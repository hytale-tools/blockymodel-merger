# BlockyModel Merger

A tool for working with Hytale's blockymodel files and exporting them as GLB (glTF Binary) or blockymodel (Hytale's Blockbench Format). Ships with three CLIs:

- `blockymerge` - builds a player model by merging accessories (clothing, hair, face features, etc.) onto the base player model, based off a character config.
- `blockyrender` - renders a character config straight to a PNG using a built-in CPU rasterizer. No GPU or external renderer needed.
- `item-to-glb` - converts a standalone item blockymodel (weapons, armor, tools, etc.) to GLB, using its matching `_Texture.png`.

If you find this useful, consider using code `jack` in the Hytale Store

Try it online or use the API: [https://hytl.skin/](https://hytl.skin/)

> **Note:** This project is provided as-is. No support or assistance will be provided. Please refer to the documentation and troubleshooting section for help.

## Features

- Merge multiple blockymodel files into a single model
- Apply gradient tinting to textures based on color specifications
- Export to GLB format (compatible with Blockbench and 3D viewers)
- Export to blockymodel format
- Automatic texture atlas generation
- Render characters directly to PNG via `blockyrender` (CPU-only, multiple camera views)
- Convert individual item blockymodels (weapons, armor, tools, etc.) to GLB via `item-to-glb`

## Downloads

Pre-built binaries are available for Windows, Linux, and macOS (Intel & Apple Silicon):

- **[Latest Release](https://github.com/hytale-tools/blockymodel-merger/releases/latest)** - Download the latest version
- **[All Releases](https://github.com/hytale-tools/blockymodel-merger/releases)** - Browse all releases

Each release includes:

- `blockymerge` / `blockymerge.exe` - Main merger tool
- `blockyrender` / `blockyrender.exe` - Character → PNG renderer
- `extract-assets` / `extract-assets.exe` - Assets extraction utility
- `item-to-glb` / `item-to-glb.exe` - Single-item blockymodel → GLB converter

**Quick Start:**

1. Download the archive for your platform from the [latest release](https://github.com/hytale-tools/blockymodel-merger/releases/latest)
2. Extract the archive
3. Follow the [Setup](#setup) instructions below to configure assets

## Setup

### Prerequisites

- Go 1.21 or later ([download](https://go.dev/dl/))
- Access to the game assets and registry data

### Getting Started

#### 1. Download the tools

You can either download pre-built binaries from the [releases page](https://github.com/hytale-tools/blockymodel-merger/releases/latest) or build from source.

**Option A: Use pre-built binaries (recommended)**

- Download the archive for your platform from the [latest release](https://github.com/hytale-tools/blockymodel-merger/releases/latest)
- Extract the archive and make the binaries executable (on Linux/macOS: `chmod +x blockymerge blockyrender extract-assets item-to-glb`)

**Option B: Build from source**

```bash
git clone https://github.com/hytale-tools/blockymodel-merger.git
cd blockymodel-merger
go build -o blockymerge ./cmd/blockymerge
go build -o blockyrender ./cmd/blockyrender
go build -o extract-assets ./cmd/extract-assets
go build -o item-to-glb ./cmd/item-to-glb
```

#### 2. Set up the `assets/` directory

The `assets/` directory should contain:

- `Characters/` - character model files (`.blockymodel`) and textures
- `Cosmetics/` - cosmetic/accessory model files
- `TintGradients/` - gradient textures for tinting

**How to obtain assets:**

1. Download `assets.zip` from the [Hytale Server Manual](https://support.hytale.com/hc/en-us/articles/45326769420827-Hytale-Server-Manual#server-setup).

   **Important:** You need the **server** `assets.zip` (not the client version), as it includes the `data/` directory with registry JSON files.

2. Use the extraction utility:

   ```bash
   # If you downloaded pre-built binaries, extract-assets should already be available
   # If you built from source, you may need to build it first:
   # go build -o extract-assets ./cmd/extract-assets

   # Extract required folders from assets.zip
   ./extract-assets /path/to/assets.zip
   ```

This will extract and map:

- `Common/Characters` → `assets/Characters/`
- `Common/Cosmetics` → `assets/Cosmetics/`
- `Common/TintGradients` → `assets/TintGradients/`
- `Common/BlockTextures` → `assets/BlockTextures/` (block face textures, for `-hold-block`)
- `Common/Blocks` → `assets/Blocks/` (custom block models, for `-hold-block`)
- `Cosmetics/CharacterCreator` → `data/` (registry JSON files)
- `Server/Item/Items` → `data/Items/` (item/block definitions, for `-hold-block`)

The directory structure should match:

```
assets/
├── Characters/
│   ├── Player.blockymodel
│   ├── Player_Textures/
│   ├── Haircuts/
│   ├── Body_Attachments/
│   └── ...
├── Cosmetics/
└── TintGradients/
```

#### 3. Verify the `data/` directory

The extraction utility will also extract the `data/` directory from the server `assets.zip`. This contains JSON registry files that map accessory IDs to their model files and textures:

- `Haircuts.json` - hair style definitions
- `Faces.json` - face texture definitions
- `Eyes.json` - eye style definitions
- `Pants.json`, `Overtops.json`, etc. - clothing definitions
- `GradientSets.json` - gradient tint definitions
- And other registry files...

**Note:** The client `assets.zip` does not include the `data/` directory - you must use the server `assets.zip` from the Hytale Server Manual.

#### 4. Verify your setup

```bash
# Check that required directories exist
ls assets/Characters/Player.blockymodel
ls data/Haircuts.json
ls data/GradientSets.json
```

#### 5. Verify the tools are available

```bash
# If you downloaded pre-built binaries, they should be in your current directory
# If you built from source, they should be in the project root
./blockymerge -h    # Should show help text
./extract-assets -h # Should show help text (if available)
./item-to-glb -h    # Should show flag list
```

#### 6. Test with a character file

```bash
# Create a simple test character
echo '{"bodyCharacteristic": "Default.02", "haircut": "Scavenger_Hair.PitchBlack"}' > test-char.json

# Run the merger
./blockymerge -char test-char.json -out test

# Check output
ls output/test.*
```

### (Optional) Items for `item-to-glb`

`item-to-glb` operates on standalone item blockymodels that are **not** extracted by
`extract-assets`. If you want to use it, extract the `Common/Items/` folder from the
server `assets.zip` manually into `assets/Items/`.

Each item folder contains a `.blockymodel` alongside its matching `_Texture.png` (for example `Bronze.blockymodel` + `Bronze_Texture.png`). Some variants reuse another model's texture (e.g. `Adamantite_Triple.blockymodel` uses `Adamantite_Texture.png`) - pass `-texture` explicitly in those cases.

Expected structure after extraction:

```
assets/Items/
├── Armors/          (Bronze/, Iron/, Cobalt/, Mithril/, ... each with Chest/Head/Hands/Legs)
├── Weapons/         (Dagger/, Bow/, Sword/, Axe/, Spear/, ...)
├── Tools/           (Pickaxe/, Hammer/, Hatchet/, Fishing_Rod/, ...)
├── Consumables/
├── Trinkets/
├── Deployables/
├── Vehicles/
├── Projectiles/
├── Instruments/
├── Ingredients/
├── Torch/
└── ...
```

Quick verification:

```bash
./item-to-glb assets/Items/Weapons/Dagger/Bronze.blockymodel
ls output/Bronze.glb
```

## Building

```bash
go build -o blockymerge ./cmd/blockymerge
go build -o blockyrender ./cmd/blockyrender
go build -o extract-assets ./cmd/extract-assets
go build -o item-to-glb ./cmd/item-to-glb
```

## Usage

```bash
./blockymerge -char <character.json> [options]
```

### Options


| Flag          | Default    | Description                                    |
| ------------- | ---------- | ---------------------------------------------- |
| `-char`       | (required) | Path to character JSON configuration file      |
| `-out`        | `merged`   | Output file name (without extension)           |
| `-format`     | `both`     | Output format: `glb`, `blockymodel`, or `both` |
| `-no-tint`    | `false`    | Skip texture tinting (output raw greyscale)    |
| `-no-defaults` | `false`   | Do not fill empty required slots (face, eyes, underwear, ...) with the game's defaults |
| `-debug`      | `false`    | Print debug output showing node tree           |
| `-hold-block` | (none)     | Block item ID to place in the character's hand (e.g. `Soil_Grass`); applies the game's carry pose |
| `-pose`       | (none)     | Apply frame 0 of a `.blockyanim` as a static pose |
| `-pack`       | (none)     | External asset pack (mod) root directory; repeatable, takes priority over base assets |


### Examples

```bash
# Export as GLB only
./blockymerge -char my-character.json -out player -format glb

# Export both formats with debug output
./blockymerge -char my-character.json -out player -format both -debug

# Export without tinting
./blockymerge -char my-character.json -out player -no-tint

# Export holding a grass block (posed with the game's block carry idle)
./blockymerge -char my-character.json -out player -hold-block Soil_Grass

# Hold a block from a mod / external asset pack (layout mirrors assets.zip)
./blockymerge -char my-character.json -hold-block Pillow_Block_Cyan -pack path/to/pack
```

### Holding blocks & poses

`-hold-block <Id>` puts a block in the character's hand the way the game does -
textures, model, scale, and carry pose all come from the block's own
definition. `-pose <file.blockyanim>` applies any animation's first frame as a
static pose, and `-pack <dir>` adds mod asset packs (assets.zip layout,
searched before base assets). Shared by `blockymerge` and `blockyrender`;
requires the block data extracted by extract-assets.

## Converting individual items to GLB

```bash
./item-to-glb assets/Items/Weapons/Dagger/Bronze.blockymodel
./item-to-glb -model <path.blockymodel> -texture <path.png> -out <path.glb>
```

### Options


| Flag       | Default                                       | Description                     |
| ---------- | --------------------------------------------- | ------------------------------- |
| `-model`   | (required or pass as positional arg)          | Path to the item `.blockymodel` |
| `-texture` | `<model-basename>_Texture.png` (sibling file) | Path to the texture PNG         |
| `-out`     | `output/<basename>.glb`                       | Output GLB path                 |
| `-verbose` | `false`                                       | Print info messages             |


Override `-texture` for variants that reuse another model's texture, e.g.
`Adamantite_Triple.blockymodel` uses `Adamantite_Texture.png`:

```bash
./item-to-glb \
  -model assets/Items/Weapons/Bow/Adamantite_Triple.blockymodel \
  -texture assets/Items/Weapons/Bow/Adamantite_Texture.png
```

## Rendering characters to PNG

`blockyrender` skips the GLB step entirely and rasterizes the merged character
straight to an image:

```bash
# Character config (uses the same registry/assets as blockymerge)
./blockyrender -char my-character.json -view full-body -size 512 -o out.png

# Standalone blockymodel + texture (no registry/assets needed)
./blockyrender -model some.blockymodel -texture some_Texture.png -view headshot -o out.png
```

Supports multiple camera views (full-body, headshot, bust, isometric, and more).
See [cmd/blockyrender/README.md](cmd/blockyrender/README.md) for the full flag
list and performance notes.

## Character Configuration

Character JSON files define the appearance of a character. Each field is optional except for accessories you want to include.

Format: `"AccessoryId.Color.Variant"` where Color and Variant are optional.

```json
{
  "bodyCharacteristic": "Default.02",
  "underwear": "Boxer.Turquoise",
  "face": "Face_MakeUp",
  "ears": "Elf_Ears",
  "mouth": "Mouth_Makeup",
  "haircut": "Scavenger_Hair.PitchBlack",
  "facialHair": null,
  "eyebrows": "Plucked.PitchBlack",
  "eyes": "Large_Eyes.Pink",
  "pants": null,
  "overpants": "LongSocks_Striped.Black",
  "undertop": "Mercenary_Top.Black",
  "overtop": "OnePiece_ApronDress.Black",
  "shoes": null,
  "headAccessory": "Ribbon.White",
  "faceAccessory": null,
  "earAccessory": "AcornEarrings.Acorn.Both",
  "skinFeature": null,
  "gloves": "LongGloves_Savanna.Black",
  "cape": "Cape_Wasteland_Marauder.BlueDark.NoNeck"
}
```

**Accessing cached character skin data:**

If you have the game installed, you can access character skins you've created from the game's installation directory:

1. Open the Hytale launcher
2. Click the settings cog (⚙️)
3. Click "Open Directory"
4. Navigate to the `UserData` folder
5. Open the `CachedPlayerSkins` folder

You can copy character JSON files from this folder and use them directly with the tool:

```bash
./blockymerge -char /path/to/CachedPlayerSkins/your-character.json -out output-name
```

### Available Slots


| Slot                 | Description                                  |
| -------------------- | -------------------------------------------- |
| `bodyCharacteristic` | Body type and skin tone (e.g., `Default.02`) |
| `underwear`          | Base underwear layer                         |
| `face`               | Face texture/makeup                          |
| `ears`               | Ear shape (e.g., `Elf_Ears`, `Default`)      |
| `mouth`              | Mouth style                                  |
| `haircut`            | Hair style and color                         |
| `facialHair`         | Beard/mustache                               |
| `eyebrows`           | Eyebrow style and color                      |
| `eyes`               | Eye style and color                          |
| `pants`              | Pants layer                                  |
| `overpants`          | Over-pants layer (socks, skirts, etc.)       |
| `undertop`           | Under-top layer                              |
| `overtop`            | Over-top layer (dresses, jackets, etc.)      |
| `shoes`              | Footwear                                     |
| `headAccessory`      | Head accessories (hats, ribbons, etc.)       |
| `faceAccessory`      | Face accessories (glasses, masks, etc.)      |
| `earAccessory`       | Ear accessories (earrings)                   |
| `skinFeature`        | Skin features (freckles, etc.)               |
| `gloves`             | Hand/arm accessories                         |
| `cape`               | Back accessories (capes, wings, etc.)        |


## Output

Files are saved to the `output/` directory:

- `<name>.glb` - GLB model file
- `<name>.blockymodel` - BlockyModel JSON file
- `<name>_atlas.png` - Texture atlas

## Project Structure

```
blockymodel-merger/
├── cmd/
│   ├── blockymerge/
│   │   └── main.go          # CLI entry point
│   ├── blockyrender/
│   │   └── main.go          # Character → PNG renderer
│   ├── extract-assets/
│   │   └── main.go          # Assets extraction utility
│   └── item-to-glb/
│       └── main.go          # Single-item blockymodel → GLB converter
├── pkg/
│   ├── anim/                # .blockyanim loading & static pose application
│   ├── blocks/              # Block definitions & held-block accessory building
│   ├── blockymodel/         # BlockyModel parsing
│   ├── character/           # Character data loading
│   ├── export/              # GLB export
│   ├── merger/              # Model merging logic
│   ├── pipeline/            # Shared load + merge + tint + atlas pipeline
│   ├── registry/            # Accessory registry
│   ├── render/              # CPU software rasterizer
│   └── texture/             # Texture processing & atlas
├── assets/                  # BlockyModel & texture assets
├── data/                    # Accessory registry JSON files
└── output/                  # Generated output files
```

## Requirements

- Go 1.21+
- Asset files in `assets/` directory (see Setup section above)
- Registry data in `data/` directory (see Setup section above)

## Troubleshooting

### "Failed to load registry" error

Make sure the `data/` directory exists and contains the required JSON registry files. The tool looks for files like `Haircuts.json`, `Faces.json`, `Eyes.json`, etc.

### "Failed to load base model" error

Ensure `assets/Characters/Player.blockymodel` exists. This is the base player model that all characters are built from.

### "Could not load texture" warnings

These are usually non-fatal - the tool will continue but the accessory may appear without textures. Check that:

- Texture paths in the registry JSON files are correct
- The texture files exist in the `assets/` directory
- Path separators match your OS (use forward slashes `/` in JSON files)

## Dependencies

- [github.com/qmuntal/gltf](https://github.com/qmuntal/gltf) - glTF/GLB encoding

## License

GNU GPL v3 - see [LICENSE](LICENSE) file for details.