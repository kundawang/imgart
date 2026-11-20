package effect

import (
	"context"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/talento90/imgart/errors"
)

// newSolidNRGBA builds an image where every pixel has the given NRGBA color.
func newSolidNRGBA(width, height int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))

	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = c.R
		img.Pix[i+1] = c.G
		img.Pix[i+2] = c.B
		img.Pix[i+3] = c.A
	}

	return img
}

// castGrayCard builds a fixed color-cast gray card: a neutral gray (128,128,128)
// multiplied by a warm color cast (R*1.30, G*1.00, B*0.70), fully opaque.
func castGrayCard(width, height int) *image.NRGBA {
	return newSolidNRGBA(width, height, color.NRGBA{
		R: clampChannel(128 * 1.30),
		G: 128,
		B: clampChannel(128 * 0.70),
		A: 255,
	})
}

func TestWhiteBalanceNeutralRecoveryOnCastGrayCard(t *testing.T) {
	img := castGrayCard(10, 10)
	wb := NewWhiteBalance()

	result, err := wb.Transform(context.Background(), img, map[string]interface{}{})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	nrgba, ok := result.(*image.NRGBA)

	if !ok {
		t.Fatalf("Expected *image.NRGBA, got: %T", result)
	}

	// Gray world must bring the three channels of the gray card back together.
	c := nrgba.NRGBAAt(5, 5)
	spread := int(math.Max(float64(c.R), math.Max(float64(c.G), float64(c.B)))) -
		int(math.Min(float64(c.R), math.Min(float64(c.G), float64(c.B))))

	if spread > 2 {
		t.Errorf("Expected neutral channels after white balance, got R=%d G=%d B=%d (spread=%d)", c.R, c.G, c.B, spread)
	}

	// The recovered gray should stay close to the original luma, not drift to white.
	luma := (int(c.R) + int(c.G) + int(c.B)) / 3
	if math.Abs(float64(luma-128)) > 15 {
		t.Errorf("Expected luma around 128, got %d (R=%d G=%d B=%d)", luma, c.R, c.G, c.B)
	}
}

func TestWhiteBalanceIsDeterministic(t *testing.T) {
	wb := NewWhiteBalance()
	params := map[string]interface{}{"strength": 0.8, "max_gain": 1.5}

	first, err := wb.Transform(context.Background(), castGrayCard(8, 8), params)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	second, err := wb.Transform(context.Background(), castGrayCard(8, 8), params)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	a := first.(*image.NRGBA)
	b := second.(*image.NRGBA)

	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("White balance is not deterministic at pixel byte %d: %d != %d", i, a.Pix[i], b.Pix[i])
		}
	}
}

func TestWhiteBalancePreservesTransparentAndSemiTransparent(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 0})        // fully transparent
	img.SetNRGBA(1, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 128}) // semi-transparent
	img.SetNRGBA(2, 0, color.NRGBA{R: 166, G: 128, B: 90, A: 255}) // opaque neutral

	wb := NewWhiteBalance()

	result, err := wb.Transform(context.Background(), img, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	got := result.(*image.NRGBA)

	transparent := got.NRGBAAt(0, 0)
	if transparent != (color.NRGBA{R: 0, G: 0, B: 0, A: 0}) {
		t.Errorf("Fully transparent pixel changed: %+v", transparent)
	}

	semi := got.NRGBAAt(1, 0)
	if semi != (color.NRGBA{R: 200, G: 100, B: 50, A: 128}) {
		t.Errorf("Semi-transparent pixel color/alpha changed: %+v", semi)
	}
}

func TestWhiteBalanceAllTransparentFallsBackToOriginal(t *testing.T) {
	img := newSolidNRGBA(6, 6, color.NRGBA{R: 255, G: 255, B: 255, A: 0})
	wb := NewWhiteBalance()

	result, err := wb.Transform(context.Background(), img, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result != image.Image(img) {
		t.Error("Expected the original image to be returned when there is no neutral region")
	}
}

func TestWhiteBalanceNoNeutralRegionFallsBackToOriginal(t *testing.T) {
	// Saturated primary colors: none of them is close to neutral.
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 230, G: 10, B: 20, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{R: 20, G: 220, B: 30, A: 255})
	img.SetNRGBA(2, 0, color.NRGBA{R: 15, G: 25, B: 210, A: 255})

	wb := NewWhiteBalance()

	result, err := wb.Transform(context.Background(), img, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result != image.Image(img) {
		t.Error("Expected the original image to be returned when there is no neutral region")
	}
}

func TestWhiteBalanceAllGrayIsIdentity(t *testing.T) {
	img := newSolidNRGBA(6, 6, color.NRGBA{R: 120, G: 120, B: 120, A: 255})
	wb := NewWhiteBalance()

	result, err := wb.Transform(context.Background(), img, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	got := result.(*image.NRGBA).NRGBAAt(3, 3)
	if got != (color.NRGBA{R: 120, G: 120, B: 120, A: 255}) {
		t.Errorf("Expected gray pixels to remain unchanged, got %+v", got)
	}
}

func TestWhiteBalanceExtremeBrightPixelsAreIgnored(t *testing.T) {
	// A clipped highlight is not a reliable neutral reference.
	img := newSolidNRGBA(4, 4, color.NRGBA{R: 255, G: 250, B: 240, A: 255})
	wb := NewWhiteBalance()

	result, err := wb.Transform(context.Background(), img, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result != image.Image(img) {
		t.Error("Expected fallback when only extreme bright pixels are available")
	}
}

func TestWhiteBalanceMaxGainIsClamped(t *testing.T) {
	img := castGrayCard(8, 8)
	wb := NewWhiteBalance()

	result, err := wb.Transform(context.Background(), img, map[string]interface{}{"max_gain": 1.05})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	got := result.(*image.NRGBA).NRGBAAt(4, 4)
	// With gains clamped to ~1.05 the cast cannot be fully removed.
	spread := int(math.Max(float64(got.R), math.Max(float64(got.G), float64(got.B)))) -
		int(math.Min(float64(got.R), math.Min(float64(got.G), float64(got.B))))

	if spread <= 2 {
		t.Errorf("Expected cast to remain when max_gain clamps the correction, got R=%d G=%d B=%d", got.R, got.G, got.B)
	}
}

func TestWhiteBalanceStrengthBlendsCorrection(t *testing.T) {
	img := castGrayCard(8, 8)
	wb := NewWhiteBalance()

	full, err := wb.Transform(context.Background(), img, map[string]interface{}{"strength": 1.0})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	half, err := wb.Transform(context.Background(), img, map[string]interface{}{"strength": 0.5})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	fullSpread := channelSpread(full.(*image.NRGBA).NRGBAAt(0, 0))
	halfSpread := channelSpread(half.(*image.NRGBA).NRGBAAt(0, 0))

	if halfSpread <= fullSpread {
		t.Errorf("Expected partial strength to leave a larger cast, full spread=%d half spread=%d", fullSpread, halfSpread)
	}
}

func TestWhiteBalanceZeroStrengthReturnsOriginal(t *testing.T) {
	img := castGrayCard(8, 8)
	wb := NewWhiteBalance()

	result, err := wb.Transform(context.Background(), img, map[string]interface{}{"strength": 0.0})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result != image.Image(img) {
		t.Error("Expected the original image when strength is zero")
	}
}

func TestWhiteBalanceInvalidParameters(t *testing.T) {
	tt := []struct {
		name   string
		params map[string]interface{}
		err    errors.Type
	}{
		{name: "strength wrong type", params: map[string]interface{}{"strength": "strong"}, err: errors.Validation},
		{name: "strength NaN", params: map[string]interface{}{"strength": math.NaN()}, err: errors.Validation},
		{name: "strength positive infinity", params: map[string]interface{}{"strength": math.Inf(1)}, err: errors.Validation},
		{name: "strength negative infinity", params: map[string]interface{}{"strength": math.Inf(-1)}, err: errors.Validation},
		{name: "strength out of range", params: map[string]interface{}{"strength": 1.5}, err: errors.Validation},
		{name: "max_gain wrong type", params: map[string]interface{}{"max_gain": "big"}, err: errors.Validation},
		{name: "max_gain NaN", params: map[string]interface{}{"max_gain": math.NaN()}, err: errors.Validation},
		{name: "max_gain below one", params: map[string]interface{}{"max_gain": 0.5}, err: errors.Validation},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			wb := NewWhiteBalance()

			_, err := wb.Transform(context.Background(), castGrayCard(4, 4), tc.params)

			if err == nil || !errors.Is(tc.err, err) {
				t.Errorf("Expected validation error, got: %v", err)
			}
		})
	}
}

func TestWhiteBalanceMalformedImage(t *testing.T) {
	wb := NewWhiteBalance()

	if _, err := wb.Transform(context.Background(), nil, map[string]interface{}{}); err == nil || !errors.Is(errors.Malformed, err) {
		t.Errorf("Expected malformed error for nil image, got: %v", err)
	}

	empty := image.NewNRGBA(image.Rectangle{})
	if _, err := wb.Transform(context.Background(), empty, map[string]interface{}{}); err == nil || !errors.Is(errors.Malformed, err) {
		t.Errorf("Expected malformed error for empty image, got: %v", err)
	}
}

func TestWhiteBalanceColorImageFidelity(t *testing.T) {
	// Half the image is a warm-cast gray card (the reference), the other half a
	// saturated blue patch whose color must keep its identity.
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for x := 0; x < 4; x++ {
		img.SetNRGBA(x, 0, color.NRGBA{R: 166, G: 128, B: 90, A: 255})
		img.SetNRGBA(x, 1, color.NRGBA{R: 20, G: 40, B: 220, A: 255})
	}

	wb := NewWhiteBalance()

	result, err := wb.Transform(context.Background(), img, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	got := result.(*image.NRGBA)

	// The saturated blue patch stays blue (blue remains the dominant channel).
	blue := got.NRGBAAt(0, 1)
	if !(blue.B > blue.R && blue.B > blue.G) {
		t.Errorf("Expected the colored patch to remain blue, got R=%d G=%d B=%d", blue.R, blue.G, blue.B)
	}

	// It must be transformed by exactly the same per-channel gains as every
	// other opaque pixel: out = clamp(round(in * gain)).
	gains, ok := estimateWhiteBalanceGains(img, defaultWhiteBalanceMaxGain)
	if !ok {
		t.Fatal("Expected gains to be estimated")
	}

	expectedR := clampChannel(20 * gains[0])
	expectedG := clampChannel(40 * gains[1])
	expectedB := clampChannel(220 * gains[2])

	if blue.R != expectedR || blue.G != expectedG || blue.B != expectedB {
		t.Errorf("Expected colored pixel R=%d G=%d B=%d, got R=%d G=%d B=%d",
			expectedR, expectedG, expectedB, blue.R, blue.G, blue.B)
	}

	// And the gray card region still converges to neutral.
	gray := got.NRGBAAt(0, 0)
	if channelSpread(gray) > 2 {
		t.Errorf("Expected neutral gray card, got R=%d G=%d B=%d", gray.R, gray.G, gray.B)
	}
}

func TestWhiteBalanceAcceptsRGBAInput(t *testing.T) {
	// Images decoded from PNG/JPEG may arrive as *image.RGBA rather than NRGBA.
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i] = 166
		src.Pix[i+1] = 128
		src.Pix[i+2] = 90
		src.Pix[i+3] = 255
	}

	wb := NewWhiteBalance()

	result, err := wb.Transform(context.Background(), src, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	nrgba, ok := result.(*image.NRGBA)
	if !ok {
		t.Fatalf("Expected *image.NRGBA output, got: %T", result)
	}

	gray := nrgba.NRGBAAt(2, 2)
	if channelSpread(gray) > 2 {
		t.Errorf("Expected neutral output for RGBA input, got R=%d G=%d B=%d", gray.R, gray.G, gray.B)
	}
}

func TestEstimateWhiteBalanceGains(t *testing.T) {
	img := castGrayCard(6, 6)

	gains, ok := estimateWhiteBalanceGains(img, defaultWhiteBalanceMaxGain)
	if !ok {
		t.Fatal("Expected gains to be estimated from the gray card")
	}

	// The warm cast inflates red and suppresses blue: blue needs the largest gain.
	if !(gains[2] > gains[1] && gains[1] > gains[0]) {
		t.Errorf("Expected B > G > R gains for a warm cast, got R=%f G=%f B=%f", gains[0], gains[1], gains[2])
	}

	// Gains are always clamped to the configured maximum.
	for i, gain := range gains {
		if gain < 1/defaultWhiteBalanceMaxGain || gain > defaultWhiteBalanceMaxGain {
			t.Errorf("Gain %d outside of allowed range: %f", i, gain)
		}
	}
}

func TestToNRGBAPreservesPixels(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	source.SetNRGBA(0, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 4})
	source.SetNRGBA(1, 1, color.NRGBA{R: 250, G: 200, B: 100, A: 128})

	converted := toNRGBA(source)

	for i := range source.Pix {
		if converted.Pix[i] != source.Pix[i] {
			t.Fatalf("Expected byte-exact conversion, index %d: %d != %d", i, converted.Pix[i], source.Pix[i])
		}
	}

	if &converted.Pix[0] == &source.Pix[0] {
		t.Error("Expected a copy, not the same backing array")
	}
}

func TestOptionalFloatBinder(t *testing.T) {
	defaultVal := 2.5

	val, err := optionalFloatBinder("missing", map[string]interface{}{}, defaultVal)
	if err != nil || val != defaultVal {
		t.Errorf("Expected default %f, got %f (err=%v)", defaultVal, val, err)
	}

	val, err = optionalFloatBinder("key", map[string]interface{}{"key": 1.25}, defaultVal)
	if err != nil || val != 1.25 {
		t.Errorf("Expected 1.25, got %f (err=%v)", val, err)
	}

	if _, err = optionalFloatBinder("key", map[string]interface{}{"key": "nope"}, defaultVal); !errors.Is(errors.Validation, err) {
		t.Errorf("Expected validation error for wrong type, got: %v", err)
	}

	if _, err = optionalFloatBinder("key", map[string]interface{}{"key": math.NaN()}, defaultVal); !errors.Is(errors.Validation, err) {
		t.Errorf("Expected validation error for NaN, got: %v", err)
	}

	if _, err = optionalFloatBinder("key", map[string]interface{}{"key": math.Inf(1)}, defaultVal); !errors.Is(errors.Validation, err) {
		t.Errorf("Expected validation error for infinity, got: %v", err)
	}
}

func channelSpread(c color.NRGBA) int {
	max := int(math.Max(float64(c.R), math.Max(float64(c.G), float64(c.B))))
	min := int(math.Min(float64(c.R), math.Min(float64(c.G), float64(c.B))))

	return max - min
}
