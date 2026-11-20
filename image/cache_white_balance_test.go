package image

import (
	"context"
	"image"
	"testing"

	"github.com/talento90/imgart/cache"
	"github.com/talento90/imgart/imgart"
	"github.com/talento90/imgart/mock"
)

// TestCacheServiceWithoutWhiteBalanceUnchanged proves that requests not using
// the new effect keep the original caching behavior: the same filters hit the
// same key, including a cached second lookup.
func TestCacheServiceWithoutWhiteBalanceUnchanged(t *testing.T) {
	imgCache := cache.NewImage(mock.NewCache())
	svc := NewCacheService(imgCache, newServiceWithWhiteBalance(newCastImage()))

	filters := []imgart.Filter{
		{ID: "brightness", Parameters: map[string]interface{}{"percentage": 5.0}},
	}

	first, format, err := svc.Process(context.Background(), "cast.png", filters)
	if err != nil {
		t.Fatalf("Expected first processing to succeed, got: %v", err)
	}

	if format != "png" {
		t.Errorf("Expected png format, got: %s", format)
	}

	second, _, err := svc.Process(context.Background(), "cast.png", filters)
	if err != nil {
		t.Fatalf("Expected cached processing to succeed, got: %v", err)
	}

	if first.Bounds() != second.Bounds() {
		t.Error("Expected the cached image to have the same bounds")
	}
}

// TestCacheServiceWhiteBalanceHasOwnCacheEntry proves that enabling white
// balance produces a distinct cache entry from the same image without the
// effect, and that repeated lookups are served from cache deterministically.
func TestCacheServiceWhiteBalanceHasOwnCacheEntry(t *testing.T) {
	rawCache := mock.NewCache()
	imgCache := cache.NewImage(rawCache)
	svc := NewCacheService(imgCache, newServiceWithWhiteBalance(newCastImage()))

	plain := []imgart.Filter{
		{ID: "brightness", Parameters: map[string]interface{}{"percentage": 5.0}},
	}
	withBalance := []imgart.Filter{
		{ID: "brightness", Parameters: map[string]interface{}{"percentage": 5.0}},
		{ID: "white_balance", Parameters: map[string]interface{}{}},
	}

	plainImg, _, err := svc.Process(context.Background(), "cast.png", plain)
	if err != nil {
		t.Fatalf("Expected plain processing to succeed, got: %v", err)
	}

	balanced, _, err := svc.Process(context.Background(), "cast.png", withBalance)
	if err != nil {
		t.Fatalf("Expected white balance processing to succeed, got: %v", err)
	}

	if pixelsEqual(plainImg, balanced) {
		t.Error("Expected the white balanced result to differ from the plain result")
	}

	// Second call must be served from cache and return the same image.
	cached, _, err := svc.Process(context.Background(), "cast.png", withBalance)
	if err != nil {
		t.Fatalf("Expected cached white balance processing to succeed, got: %v", err)
	}

	if !pixelsEqual(balanced, cached) {
		t.Error("Expected deterministic cached result for white balance")
	}
}

func pixelsEqual(a, b image.Image) bool {
	bounds := a.Bounds()

	if bounds != b.Bounds() {
		return false
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()

			if ar != br || ag != bg || ab != bb || aa != ba {
				return false
			}
		}
	}

	return true
}
