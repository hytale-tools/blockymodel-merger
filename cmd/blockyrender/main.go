// Command blockyrender renders a Hytale character (or a standalone blockymodel)
// directly to a PNG using a CPU software rasterizer.
//
// Unlike blockymerge (which exports GLB/blockymodel for an external renderer to
// turn into an image), blockyrender produces the final image itself: no atlas
// round-trip through glTF, no GPU. It reuses the same merge+tint+atlas pipeline
// so a render matches what the GLB would look like in Blockbench.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hytale-tools/blockymodel-merger/pkg/blockymodel"
	"github.com/hytale-tools/blockymodel-merger/pkg/pipeline"
	"github.com/hytale-tools/blockymodel-merger/pkg/render"
	"github.com/hytale-tools/blockymodel-merger/pkg/util"
)

func main() {
	charFile := flag.String("char", "", "Path to character JSON file")
	modelFile := flag.String("model", "", "Path to a standalone .blockymodel (instead of -char)")
	texFile := flag.String("texture", "", "Texture PNG for -model mode (default: white)")
	view := flag.String("view", "full-body", "Camera preset: full-body, headshot, bust, iso-head, isometric, front-right, front-left, back-right, back-left")
	rotation := flag.Float64("rotation", 0, "Rotate the character by this many degrees")
	size := flag.Int("size", 512, "Output image size (square)")
	width := flag.Int("width", 0, "Output width (overrides -size)")
	height := flag.Int("height", 0, "Output height (overrides -size)")
	out := flag.String("o", "output/render.png", "Output PNG path")
	noTint := flag.Bool("no-tint", false, "Skip tinting (renders a flat white texture)")
	persp := flag.Bool("persp", false, "Use perspective camera variant where available")
	bilinear := flag.Bool("bilinear", false, "Bilinear texture filtering (slower, smoother)")
	light := flag.Bool("light", false, "Apply directional lighting")
	threads := flag.Int("threads", 0, "Rasterizer threads (0=auto/NumCPU, 1=single-threaded)")
	verbose := flag.Bool("verbose", false, "Verbose logging")
	bench := flag.Int("bench", 0, "Render N times and report average timing")
	flag.Parse()

	verboseEnabled := *verbose
	util.SetVerbose(&verboseEnabled)

	w, h := *size, *size
	if *width > 0 {
		w = *width
	}
	if *height > 0 {
		h = *height
	}

	// full-body and headshot use a perspective camera auto-fit to the geometry
	// (full body or head), built after the geometry is loaded. Other presets
	// keep their fixed cameras.
	autoFit := *view == "full-body" || *view == "full-body-front" || *view == "headshot"
	var camera render.CameraProjection
	if !autoFit {
		var ok bool
		camera, ok = render.CameraForView(*view, *persp)
		if !ok {
			fmt.Fprintf(os.Stderr, "Unknown view %q\n", *view)
			os.Exit(1)
		}
	}

	cfg := render.RenderConfig{Bilinear: *bilinear, Threads: *threads}
	if *light {
		cfg.Light = render.DefaultLight()
	}

	var faces []render.Face
	var tex *render.Texture
	var srcModel *blockymodel.BlockyModel

	loadStart := time.Now()
	switch {
	case *modelFile != "":
		model, err := blockymodel.Load(*modelFile)
		if err != nil {
			fatal("loading model", err)
		}
		srcModel = model
		faces = render.Flatten(model)
		if *texFile != "" {
			img, err := loadImage(*texFile)
			if err != nil {
				fatal("loading texture", err)
			}
			tex = render.NewTexture(img)
		} else {
			tex = render.NewTexture(whiteImage(16))
		}
	case *charFile != "":
		res, err := pipeline.BuildMergedCharacter(*charFile, *noTint)
		if err != nil {
			fatal("building character", err)
		}
		srcModel = res.Model
		faces = render.Flatten(res.Model)
		if res.Atlas != nil {
			tex = render.NewTexture(res.Atlas.Image)
		} else {
			tex = render.NewTexture(whiteImage(16))
		}
	default:
		fmt.Fprintln(os.Stderr, "Provide -char <character.json> or -model <file.blockymodel>")
		flag.Usage()
		os.Exit(1)
	}
	if autoFit {
		// Frame before rotating (bbox of the unrotated mesh). Held items are
		// excluded from the framing box so the character stays centered
		// exactly as in an item-less render.
		var bboxFaces []render.Face
		if *view == "headshot" {
			bboxFaces = render.FlattenSubtree(srcModel, "Head")
		} else {
			bboxFaces = render.FlattenExcluding(srcModel, render.HeldItemNodeName)
		}
		if len(bboxFaces) == 0 {
			bboxFaces = faces
		}
		camera = render.AutoFitPerspective(bboxFaces)
	}
	render.RotateFacesY(faces, float32(-*rotation))
	loadDur := time.Since(loadStart)

	util.Logger.Info("Scene built", "faces", len(faces), "loadMs", loadDur.Milliseconds())

	// Benchmark mode: render N times and report.
	if *bench > 0 {
		var total time.Duration
		var img *image.RGBA
		for i := 0; i < *bench; i++ {
			start := time.Now()
			img = render.RenderScene(faces, tex, camera, w, h, cfg)
			total += time.Since(start)
		}
		avg := total / time.Duration(*bench)
		fmt.Printf("Rendered %dx%d, %d faces, %d iterations: avg %v/frame (%.1f fps)\n",
			w, h, len(faces), *bench, avg, float64(time.Second)/float64(avg))
		if err := writePNG(img, *out); err != nil {
			fatal("writing PNG", err)
		}
		fmt.Printf("Saved %s\n", *out)
		return
	}

	renderStart := time.Now()
	img := render.RenderScene(faces, tex, camera, w, h, cfg)
	renderDur := time.Since(renderStart)

	if err := writePNG(img, *out); err != nil {
		fatal("writing PNG", err)
	}

	fmt.Printf("Rendered %s (%dx%d, %d faces) in %v (load %v)\n",
		*out, w, h, len(faces), renderDur, loadDur)
}

func fatal(ctx string, err error) {
	fmt.Fprintf(os.Stderr, "Error %s: %v\n", ctx, err)
	os.Exit(1)
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func whiteImage(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	return img
}

func writePNG(img image.Image, path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	if !strings.HasSuffix(strings.ToLower(path), ".png") {
		path += ".png"
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
