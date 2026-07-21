package registry

import (
	"errors"
	"testing"
)

// The testdata registry subset has Faces.json (first two entries flagged
// IsDefaultAsset), BodyCharacteristics.json (Default flagged), and
// Haircuts.json (nothing flagged). All other registries are absent.

func TestDefaultFor(t *testing.T) {
	r, err := Load("testdata")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tests := []struct {
		field  string
		wantID string
		wantOK bool
	}{
		{"face", "Face_A", true}, // two flagged: first in file order wins
		{"bodyCharacteristic", "Default", true},
		{"haircut", "", false}, // loaded, nothing flagged
		{"eyes", "", false},    // registry not loaded
		{"bogus", "", false},   // unknown field type
	}
	for _, tt := range tests {
		id, ok := r.DefaultFor(tt.field)
		if id != tt.wantID || ok != tt.wantOK {
			t.Errorf("DefaultFor(%q) = (%q, %v), want (%q, %v)",
				tt.field, id, ok, tt.wantID, tt.wantOK)
		}
	}
}

func TestSentinelErrors(t *testing.T) {
	r, err := Load("testdata")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if _, err := r.GetEntry("face", "Nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetEntry unknown id: got %v, want ErrNotFound", err)
	}
	if _, err := r.GetEntry("bogus", "X"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetEntry unknown field: got %v, want ErrNotFound", err)
	}
	if _, err := r.GetEntry("eyes", "X"); !errors.Is(err, ErrRegistryUnavailable) {
		t.Errorf("GetEntry unloaded registry: got %v, want ErrRegistryUnavailable", err)
	}
	if _, err := r.LookupWithVariant("eyes", "X", ""); !errors.Is(err, ErrRegistryUnavailable) {
		t.Errorf("LookupWithVariant unloaded registry: got %v, want ErrRegistryUnavailable", err)
	}
}
