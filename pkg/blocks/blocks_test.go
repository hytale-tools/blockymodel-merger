package blocks

import (
	"errors"
	"testing"
)

func TestFindSentinelErrors(t *testing.T) {
	existing := []Source{{ItemsDir: "testdata/items"}}

	if _, err := Find("nope", existing); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: got %v, want ErrNotFound", err)
	}
	if _, err := Find("NotABlock", existing); !errors.Is(err, ErrNotRenderable) {
		t.Errorf("item without block data: got %v, want ErrNotRenderable", err)
	}

	missing := []Source{{ItemsDir: "testdata/does-not-exist"}}
	if _, err := Find("nope", missing); !errors.Is(err, ErrSourcesUnavailable) {
		t.Errorf("missing sources: got %v, want ErrSourcesUnavailable", err)
	}
}
