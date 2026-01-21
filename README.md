# BlockyModel Merger

A tool for merging blockymodel files and exporting them as GLB (glTF Binary) format. Takes a base player model and merges accessories (clothing, hair, face features, etc.) based on a character configuration file.

## Features

- Merge multiple blockymodel files into a single model
- Apply gradient tinting to textures based on color specifications
- Export to GLB format (compatible with Blockbench and 3D viewers)
- Export to blockymodel format
- Automatic texture atlas generation

## Building

```bash
go build -o blockymerge ./cmd/blockymerge
```

## Usage

```bash
./blockymerge -char <character.json> [options]
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `-char` | (required) | Path to character JSON configuration file |
| `-out` | `merged` | Output file name (without extension) |
| `-format` | `both` | Output format: `glb`, `blockymodel`, or `both` |
| `-no-tint` | `false` | Skip texture tinting (output raw greyscale) |
| `-debug` | `false` | Print debug output showing node tree |

### Examples

```bash
# Export as GLB only
./blockymerge -char my-character.json -out player -format glb

# Export both formats with debug output
./blockymerge -char my-character.json -out player -format both -debug

# Export without tinting
./blockymerge -char my-character.json -out player -no-tint
```

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

### Available Slots

| Slot | Description |
|------|-------------|
| `bodyCharacteristic` | Body type and skin tone (e.g., `Default.02`) |
| `underwear` | Base underwear layer |
| `face` | Face texture/makeup |
| `ears` | Ear shape (e.g., `Elf_Ears`, `Default`) |
| `mouth` | Mouth style |
| `haircut` | Hair style and color |
| `facialHair` | Beard/mustache |
| `eyebrows` | Eyebrow style and color |
| `eyes` | Eye style and color |
| `pants` | Pants layer |
| `overpants` | Over-pants layer (socks, skirts, etc.) |
| `undertop` | Under-top layer |
| `overtop` | Over-top layer (dresses, jackets, etc.) |
| `shoes` | Footwear |
| `headAccessory` | Head accessories (hats, ribbons, etc.) |
| `faceAccessory` | Face accessories (glasses, masks, etc.) |
| `earAccessory` | Ear accessories (earrings) |
| `skinFeature` | Skin features (freckles, etc.) |
| `gloves` | Hand/arm accessories |
| `cape` | Back accessories (capes, wings, etc.) |

## Output

Files are saved to the `output/` directory:

- `<name>.glb` - GLB model file
- `<name>.blockymodel` - BlockyModel JSON file
- `<name>_atlas.png` - Texture atlas

## Project Structure

```
blockymodel-merger/
├── cmd/
│   └── blockymerge/
│       └── main.go          # CLI entry point
├── internal/
│   ├── blockymodel/         # BlockyModel parsing
│   ├── character/           # Character data loading
│   ├── export/              # GLB export
│   ├── merger/              # Model merging logic
│   ├── registry/            # Accessory registry
│   └── texture/             # Texture processing & atlas
├── assets/                  # BlockyModel & texture assets
├── data/                    # Accessory registry JSON files
└── output/                  # Generated output files
```

## Requirements

- Go 1.21+
- Asset files in `assets/` directory
- Registry data in `data/` directory

## Dependencies

- [github.com/qmuntal/gltf](https://github.com/qmuntal/gltf) - glTF/GLB encoding

## License

GNU GPL v3 - see [LICENSE](LICENSE) file for details.
