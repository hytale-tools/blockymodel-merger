package texture

import (
	"image"
	"testing"
)

func solid(name string, w, h int) *TintedTexture {
	return &TintedTexture{Name: name, Image: image.NewRGBA(image.Rect(0, 0, w, h))}
}

func TestAlignSize(t *testing.T) {
	cases := map[int]int{0: 32, 1: 32, 32: 32, 33: 64, 288: 288, 517: 544, 550: 576}
	for in, want := range cases {
		if got := alignSize(in); got != want {
			t.Errorf("alignSize(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestPackersProduce32AlignedAtlas(t *testing.T) {
	textures := []*TintedTexture{solid("base", 288, 480), solid("hat", 100, 37)}
	packers := map[string]func([]*TintedTexture, int) (*Atlas, error){
		"PackAtlas":         PackAtlas,
		"PackAtlasSimple":   PackAtlasSimple,
		"PackAtlasWithBase": PackAtlasWithBase,
	}
	for name, pack := range packers {
		atlas, err := pack(textures, 1)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b := atlas.Image.Bounds()
		if b.Dx() != atlas.Width || b.Dy() != atlas.Height {
			t.Errorf("%s: image %dx%d does not match atlas %dx%d", name, b.Dx(), b.Dy(), atlas.Width, atlas.Height)
		}
		if atlas.Width%32 != 0 || atlas.Height%32 != 0 || atlas.Width < 32 || atlas.Height < 32 {
			t.Errorf("%s: atlas %dx%d is not a multiple of 32", name, atlas.Width, atlas.Height)
		}
		// Base must stay at the origin so its texture layout is untouched.
		x, y, _, _, ok := atlas.GetPixelCoords("base")
		if !ok || x != 0 || y != 0 {
			t.Errorf("%s: base at (%d,%d), want (0,0)", name, x, y)
		}
	}
}
