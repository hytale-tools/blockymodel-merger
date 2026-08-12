# blockyrender

A render-only CLI that turns a Hytale character straight into a PNG using a CPU
software rasterizer - no GLB, no atlas round-trip through glTF, no GPU.

It reuses the existing merge + tint + atlas pipeline (`pkg/pipeline`), then feeds
the merged geometry into a software rasterizer (`pkg/render`).

## Usage

```bash
# Character (merges accessories + tints from the registry, like blockymerge)
blockyrender -char characters/bree.json -view full-body -size 512 -o out.png

# Standalone blockymodel + texture (no registry/assets needed)
blockyrender -model some.blockymodel -texture some_Texture.png -view headshot -o out.png
```

Flags: `-view` (full-body, headshot, bust, iso-head, isometric, front-right,
front-left, back-right, back-left), `-rotation N` (rotate the character by N
degrees), `-size`/`-width`/`-height`, `-persp` (perspective variant of
bust/iso-head views), `-bilinear`, `-light`, `-no-tint`, `-no-defaults` (do
not fill empty required slots like face/eyes/underwear with the game's
defaults), `-threads N` (0=auto/NumCPU, 1=single-threaded), `-bench N` (render
N times, report avg).

`full-body` and `headshot` use a 30-degree perspective camera auto-fit to the
model's bounding box (full body, or the Head subtree for headshots) with a
1.25x margin. Geometry inside a node named `HeldItem` is excluded from the
framing box so attachments never shift the character in frame.

## Holding a block

```bash
# Attach a block to the character's hand, posed with the game's carry animation
blockyrender -char characters/bree.json -hold-block Soil_Grass -o held.png

# Blocks from an external asset pack (mod) laid out like assets.zip (Common/ + Server/)
blockyrender -char characters/bree.json -hold-block Pillow_Block_Cyan -pack path/to/pack -o held.png

# Any static pose from a .blockyanim (frame 0); works with -char and -model
blockyrender -char characters/bree.json -pose "assets/Characters/Animations/Emote/Wave.blockyanim" -o wave.png
```

`-hold-block` takes a block item ID (the JSON basename under `data/Items/**`,
e.g. `Soil_Grass`). Cube blocks are rendered as the game's 32^3 held cube with
their face textures composed per the block definition (greyscale + tint,
side-mask overlays); `DrawType: Model` blocks (fences etc.) attach their
`CustomModel`. The pose defaults to the idle of the block's own animation set
(`PlayerAnimationsId`, resolved through item Parent chains and the
`Server/Item/Animations` registry); override it with `-pose`, adjust the item
with `-hold-rotate x,y,z`, or keep the bind pose with `-no-pose` (for exports
animated at runtime). `-pack` may be repeated; packs take priority over the
base game data. Requires `assets/BlockTextures`, `assets/Blocks`, and
`data/Items` (extracted by extract-assets).

## How the renderer works

| Optimization | Effect here |
|---|---|
| **CPU z-buffer rasterizer** (`raster.go`) | Per-pixel depth test, barycentric fill. Faces render in any order - no depth sort. |
| **No atlas/GLB on the render path** | The merged geometry is rasterized directly. The glTF encode step (the bulk of `blockymerge`) is gone. |
| **Per-face UV sampling** (`texture.go`) | UV layout (offset/mirror/angle) is resolved at sample time against the pre-tinted atlas, matching the source `.blockymodel` semantics. |
| **Frustum clipping** (`clip.go`) | Sutherland–Hodgman, near plane first, so perspective close-ups don't explode. |
| **Backface + layout culling** (`render.go`) | Negative-stretch-aware winding cull; faces with no texture layout are skipped. |
| **Camera presets** (`camera.go`) | Ortho + perspective presets (headshot, bust, full-body, iso) in raw blockymodel units. |

## How it differs from `blockymerge` (the GLB path)

- **blockymerge**: load → merge → tint → **pack atlas → rewrite UVs → encode glTF**
  → writes a `.glb`. Producing an *image* then needs an external renderer
  (Blockbench / three.js / a headless GPU) - seconds of cold start plus heavy deps.
- **blockyrender**: load → merge → tint → pack atlas → **rasterize → PNG**. Done.

Geometry and texture coordinates are shared, so a render matches what the GLB
would look like in Blockbench.

### Design notes

- **Tinting is baked, not per-pixel.** The Go pipeline bakes tints into the
  atlas up front, so the rasterizer samples final colours instead of resolving
  cosmetic/skin gradients at sample time.
- **No game "joint spacing."** The game nudges character joints together for a
  game-accurate pose. The GLB path doesn't, so we don't either (parity with the
  existing tool). If renders ever look loose at the joints, that's the knob.

## Performance

The fill path uses an incremental edge-function rasterizer (barycentric weights
stepped by constants per pixel - no per-pixel divide), a per-triangle UV-layout
fast path, and a per-face gamma-shade LUT for lighting. Projection/clip runs once
single-threaded; the fill is then split across `Threads` row bands (each band owns
disjoint rows, so there's no locking).

Pure render time, bree (561 faces), 28-core machine:

| Size | 1 thread | auto (all cores) |
|---|---|---|
| 512² | ~4.3 ms (230 fps) | ~1.1 ms (890 fps) |
| 1024² | ~14.6 ms | ~2.2 ms (460 fps) |
| 2048² | ~55 ms | ~5.8 ms (170 fps) |
| 4096² | n/a | ~23 ms (44 fps) |

(The edge-function rewrite alone was ~2.3× over the naïve barycentric loop; row
banding adds another ~8–10× on top at large sizes.)

### Threading under concurrent requests

The number of simultaneous renders is driven by user requests, so don't blindly
parallelize every render across all cores - that oversubscribes the CPU when many
requests arrive at once. Pick based on load:

- **High request concurrency** → `Threads: 1` per render. Let request-level
  parallelism (one goroutine per request) fill the cores. Best total throughput.
- **Low concurrency / large images** → `Threads: 0` (NumCPU). Lowest latency for
  the single render in flight.
- **Robust middle ground**: a server-side semaphore capping *total* concurrent
  rasterization goroutines at ~NumCPU, with each render single-threaded. One big
  render or many small ones both end up using the whole machine without thrashing.

For batch work, build the character once (`pipeline.BuildMergedCharacter`) and
call `render.RenderScene` per camera angle (≈1–6 ms each at 512²–1024²). For
many characters (e.g. an API server), create one `pipeline.Builder` and reuse
it - the registry, base model, accessory models, and tinted textures are
cached across builds, dropping a warm build from ~12 ms to under 1 ms.

## Comparison vs `blockymerge` end-to-end

- `blockymerge -format glb`: ~62 ms → a `.glb` that *still* needs an external
  renderer (Blockbench/three.js/GPU) to become an image.
- `blockyrender`: ~40 ms shared pipeline (load+merge+tint+atlas) + ~1–6 ms render
  → **final PNG**, no GPU, no external renderer.
