package pipeline

import "testing"

func TestResolvePose(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{"explicit pose wins", Options{Pose: "poses/sit.blockyanim", NoPose: true}, "poses/sit.blockyanim"},
		{"no pose", Options{NoPose: true}, ""},
		{"nothing requested", Options{}, ""},
	}
	for _, tt := range tests {
		path, warn := resolvePose(tt.opts, nil)
		if path != tt.want {
			t.Errorf("%s: resolvePose = %q, want %q", tt.name, path, tt.want)
		}
		if warn != "" {
			t.Errorf("%s: unexpected warning %q", tt.name, warn)
		}
	}
}

func TestBlockCacheKey(t *testing.T) {
	keys := map[string]bool{}
	for _, k := range []string{
		blockCacheKey("Soil_Grass", nil),
		blockCacheKey("Soil_Grass", []string{"mods/pillow"}),
		blockCacheKey("Soil_Grass", []string{"mods/pillow", "mods/other"}),
		blockCacheKey("Soil_Dirt", nil),
	} {
		if keys[k] {
			t.Errorf("duplicate cache key %q", k)
		}
		keys[k] = true
	}
}
