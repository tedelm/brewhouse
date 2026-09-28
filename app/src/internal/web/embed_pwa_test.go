package web_test

import (
	"testing"

	"brewhouse/internal/web"
)

func TestEmbeddedPWAAssets(t *testing.T) {
	h, err := web.New("test")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"manifest.webmanifest", "js/sw.js", "images/icon-192.png"} {
		b, err := h.ReadStatic(p)
		if err != nil {
			t.Errorf("ReadStatic(%q): %v", p, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("ReadStatic(%q): empty", p)
		}
	}
}
