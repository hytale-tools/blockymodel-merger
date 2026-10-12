package character

import (
	"testing"

	"github.com/hytale-tools/blockymodel-merger/pkg/registry"
)

// Pants.json in testdata retires Old_Jeans into Jeans, and Ripped_Jeans into
// Jeans' Ripped variant.
func TestSanitizeMigratesRetiredIDs(t *testing.T) {
	reg, err := registry.Load("testdata")
	if err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	tests := []struct {
		value, want string
	}{
		{"Old_Jeans.Blue", "Jeans.Blue"},
		{"Ripped_Jeans.Blue", "Jeans.Blue.Ripped"},
		{"Ripped_Jeans", "Jeans..Ripped"},
		{"Jeans.Blue", "Jeans.Blue"},
	}
	for _, tt := range tests {
		c := &CharacterData{Pants: strptr(tt.value)}
		if issues := c.Validate(reg, testGradients); len(issues) != 0 {
			t.Errorf("Validate(%q) = %v, want no issues", tt.value, issues)
		}
		if issues := c.Sanitize(reg, testGradients); len(issues) != 0 {
			t.Errorf("Sanitize(%q) = %v, want no issues", tt.value, issues)
		}
		assertSlot(t, "Pants", c.Pants, tt.want)
	}
}

func TestSanitizeChecksMigratedColor(t *testing.T) {
	reg, err := registry.Load("testdata")
	if err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	c := &CharacterData{Pants: strptr("Old_Jeans.Charcoal")}
	if issues := c.Sanitize(reg, testGradients); len(issues) != 1 {
		t.Errorf("Sanitize issues = %v, want one bad color", issues)
	}
	assertSlot(t, "Pants", c.Pants, "Jeans.Red")
}
