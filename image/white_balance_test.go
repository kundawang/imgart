package image

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/talento90/imgart/effect"
	"github.com/talento90/imgart/imgart"
	"github.com/talento90/imgart/repository/memory"
)

type castImageRepository struct {
	img image.Image
}

func (r *castImageRepository) Get(ctx context.Context, path string) (image.Image, string, error) {
	return r.img, "png", nil
}

// newCastImage returns a warm color-cast image containing a neutral gray card
// region so that white balance has a reference to recover.
func newCastImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))

	// First row: warm-cast gray card. Second row: saturated red (non neutral).
	for x := 0; x < 4; x++ {
		img.SetNRGBA(x, 0, color.NRGBA{R: 166, G: 128, B: 90, A: 255})
		img.SetNRGBA(x, 1, color.NRGBA{R: 230, G: 10, B: 20, A: 255})
	}

	return img
}

func newServiceWithWhiteBalance(img image.Image) imgart.ImageService {
	imgRepo := &castImageRepository{img: img}
	effectRepo := memory.NewImageRepository(imgRepo)

	return NewService(imgRepo, effectRepo)
}

// TestProcessWhiteBalanceChain verifies the white balance effect runs inside the
// existing JSON filter chain and that filter order changes the result.
func TestProcessWhiteBalanceChain(t *testing.T) {
	svc := newServiceWithWhiteBalance(newCastImage())
	ctx := context.Background()

	filters := []imgart.Filter{
		{ID: "white_balance", Parameters: map[string]interface{}{}},
		{ID: "brightness", Parameters: map[string]interface{}{"percentage": 10.0}},
	}

	result, _, err := svc.Process(ctx, "cast.png", filters)
	if err != nil {
		t.Fatalf("Expected the filter chain to succeed, got: %v", err)
	}

	gray := result.(*image.NRGBA).NRGBAAt(0, 0)
	spread := int(gray.R) - int(gray.B)
	if spread > 5 {
		t.Errorf("Expected the gray card to be neutralized by white balance, got R=%d G=%d B=%d", gray.R, gray.G, gray.B)
	}

	// Reversing the order must produce a different image: white balance is
	// applied after brightness and estimates gains from the brightened input.
	reversed := []imgart.Filter{
		{ID: "brightness", Parameters: map[string]interface{}{"percentage": 10.0}},
		{ID: "white_balance", Parameters: map[string]interface{}{}},
	}

	reversedResult, _, err := svc.Process(ctx, "cast.png", reversed)
	if err != nil {
		t.Fatalf("Expected the reversed filter chain to succeed, got: %v", err)
	}

	a := result.(*image.NRGBA)
	b := reversedResult.(*image.NRGBA)

	same := true
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			same = false
			break
		}
	}

	if same {
		t.Error("Expected filter order to change the processed result")
	}
}

// TestProcessUnknownFilterStillErrors guards the existing chain behavior after
// adding a new effect.
func TestProcessUnknownFilterStillErrors(t *testing.T) {
	svc := newServiceWithWhiteBalance(newCastImage())

	_, _, err := svc.Process(context.Background(), "cast.png", []imgart.Filter{
		{ID: "not_a_real_filter", Parameters: map[string]interface{}{}},
	})

	if err == nil {
		t.Error("Expected an error for an unknown filter")
	}
}

func TestWhiteBalanceRegisteredAsEffect(t *testing.T) {
	svc := newServiceWithWhiteBalance(newCastImage())

	e, err := svc.Effect("white_balance")
	if err != nil {
		t.Fatalf("Expected white_balance effect to be registered, got: %v", err)
	}

	if e.ID() != "white_balance" {
		t.Errorf("Expected id white_balance, got: %s", e.ID())
	}

	if _, err = effect.NewWhiteBalance().Transform(context.Background(), newCastImage(), map[string]interface{}{}); err != nil {
		t.Errorf("Expected a successful transform, got: %v", err)
	}
}
