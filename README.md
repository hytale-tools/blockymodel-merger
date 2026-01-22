# BlockyModel Merger

A tool for merging blockymodel files and exporting them as GLB (glTF Binary) format or blockymodel (Blockbench Hytale Format). Takes a base player model and merges accessories (clothing, hair, face features, etc.) based on a character configuration file.

If you find this useful, consider using code `jack` in the Hytale Store

> **Note:** This project is provided as-is. No support or assistance will be provided. Please refer to the documentation and troubleshooting section for help.

## Features

- Merge multiple blockymodel files into a single model
- Apply gradient tinting to textures based on color specifications
- Export to GLB format (compatible with Blockbench and 3D viewers)
- Export to blockymodel format
- Automatic texture atlas generation

## Setup

### Prerequisites

- Go 1.21 or later ([download](https://go.dev/dl/))
- Access to the game assets and registry data

### Getting Started

1. **Clone the repository:**
   ```bash
   git clone <repository-url>
   cd blockymodel-merger
   ```

2. **Set up the `assets/` directory:**
   
   The `assets/` directory should contain:
   - `Characters/` - Character model files (`.blockymodel`) and textures
   - `Cosmetics/` - Cosmetic/accessory model files
   - `TintGradients/` - Gradient textures for tinting
   
   **How to obtain assets:**
   
   1. Download `assets.zip` from the [Hytale Server Manual](https://support.hytale.com/hc/en-us/articles/45326769420827-Hytale-Server-Manual#server-setup)
   
      **Important:** You need the **server** assets.zip (not the client version), as it includes the `data/` directory with registry JSON files.
   
   2. Use the extraction utility:
      ```bash
      # Build the extractor
      go build -o extract-assets ./cmd/extract-assets
      
      # Extract required folders from assets.zip
      ./extract-assets /path/to/assets.zip
      ```
   
   This will extract and map:
   - `Common/Characters` → `assets/Characters/`
   - `Common/Cosmetics` → `assets/Cosmetics/`
   - `Common/TintGradients` → `assets/TintGradients/`
   - `Cosmetics/CharacterCreator` → `data/` (registry JSON files)
   
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

3. **Verify the `data/` directory:**
   
   The extraction utility will also extract the `data/` directory from the server assets.zip. This contains JSON registry files that map accessory IDs to their model files and textures:
   - `Haircuts.json` - Hair style definitions
   - `Faces.json` - Face texture definitions
   - `Eyes.json` - Eye style definitions
   - `Pants.json`, `Overtops.json`, etc. - Clothing definitions
   - `GradientSets.json` - Gradient tint definitions
   - And other registry files...
   
   **Note:** The client assets.zip does not include the `data/` directory - you must use the server assets.zip from the Hytale Server Manual.

4. **Verify your setup:**
   ```bash
   # Check that required directories exist
   ls assets/Characters/Player.blockymodel
   ls data/Haircuts.json
   ls data/GradientSets.json
   ```

5. **Build the tool:**
   ```bash
   go build -o blockymerge ./cmd/blockymerge
   ```

6. **Test with a character file:**
   ```bash
   # Create a simple test character
   echo '{"bodyCharacteristic": "Default.02", "haircut": "Scavenger_Hair.PitchBlack"}' > test-char.json
   
   # Run the merger
   ./blockymerge -char test-char.json -out test
   
   # Check output
   ls output/test.*
   ```

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
│   ├── blockymerge/
│   │   └── main.go          # CLI entry point
│   └── extract-assets/
│       └── main.go          # Assets extraction utility
├── pkg/
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
