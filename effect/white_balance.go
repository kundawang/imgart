package effect

import (
	"context"
	"image"
	"image/color"
	"math"

	"github.com/talento90/imgart/errors"
	"github.com/talento90/imgart/imgart"
)

const (
	// defaultWhiteBalanceStrength applies the full estimated correction.
	defaultWhiteBalanceStrength = 1.0
	// defaultWhiteBalanceMaxGain limits how far a single channel can be boosted/cut.
	defaultWhiteBalanceMaxGain = 2.0
	// neutralChromaTolerance is the maximum relative chroma (max-min)/max for a
	// pixel to be considered a neutral gray/white reference. The tolerance is
	// generous enough to still recognize neutral surfaces under a strong cast;
	// saturated colors stay well above it and are excluded.
	neutralChromaTolerance = 0.6
	// neutralLumaMin/Max exclude crushed blacks and clipped highlights (such as
	// specular spots) from the neutral reference set.
	neutralLumaMin = 20
	neutralLumaMax = 235
)

type whiteBalance struct {
	effect
}

// NewWhiteBalance creates an Effect that automatically corrects the white point
// of the image. Gains for the three color channels are estimated from neutral
// (gray) regions using a gray-world assumption and then applied to every opaque
// pixel. Semi-transparent and fully transparent pixels are left untouched.
func NewWhiteBalance() imgart.Effect {
	return &whiteBalance{
		effect: effect{
			id:          "white_balance",
			description: "White Balance - Automatic white balance based on neutral regions",
			parameters: imgart.Parameters{
				"strength": imgart.Parameter{
					Description: "How much of the estimated correction is applied. 0 keeps the image unchanged, 1 applies the full correction.",
					Required:    false,
					Example:     1.0,
					Type:        "float",
					Default:     defaultWhiteBalanceStrength,
				},
				"max_gain": imgart.Parameter{
					Description: "Maximum per-channel gain. Each channel gain is clamped to [1/max_gain, max_gain].",
					Required:    false,
					Example:     2.0,
					Type:        "float",
					Default:     defaultWhiteBalanceMaxGain,
				},
			},
		},
	}
}

func (w *whiteBalance) Transform(ctx context.Context, img image.Image, params map[string]interface{}) (image.Image, error) {
	strength, err := optionalFloatBinder("strength", params, defaultWhiteBalanceStrength)

	if err != nil {
		return nil, err
	}

	if strength < 0 || strength > 1 {
		return nil, errors.EValidation("Parameter strength needs to be between 0 and 1", nil)
	}

	maxGain, err := optionalFloatBinder("max_gain", params, defaultWhiteBalanceMaxGain)

	if err != nil {
		return nil, err
	}

	if maxGain < 1 {
		return nil, errors.EValidation("Parameter max_gain needs to be greater than or equal to 1", nil)
	}

	if img == nil {
		return nil, errors.EMalformed("Image is empty and cannot be processed", nil)
	}

	bounds := img.Bounds()

	if bounds.Empty() {
		return nil, errors.EMalformed("Image has an empty bounds and cannot be processed", nil)
	}

	// With no correction requested there is nothing to estimate or apply.
	if strength == 0 {
		return img, ctx.Err()
	}

	nrgbaImg := toNRGBA(img)

	gains, ok := estimateWhiteBalanceGains(nrgbaImg, maxGain)

	// No reliable neutral region: fall back to the original image instead of
	// guessing a correction.
	if !ok {
		return img, ctx.Err()
	}

	// Blend the estimated gains with the identity so strength controls how much
	// of the correction is applied (0 = unchanged, 1 = full correction).
	for i := range gains {
		gains[i] = 1 + (gains[i]-1)*strength
	}

	for i := 0; i < len(nrgbaImg.Pix); i += 4 {
		// Only opaque pixels are corrected; the color and alpha of
		// semi-transparent and fully transparent pixels are preserved.
		if nrgbaImg.Pix[i+3] != 255 {
			continue
		}

		nrgbaImg.Pix[i] = clampChannel(float64(nrgbaImg.Pix[i]) * gains[0])
		nrgbaImg.Pix[i+1] = clampChannel(float64(nrgbaImg.Pix[i+1]) * gains[1])
		nrgbaImg.Pix[i+2] = clampChannel(float64(nrgbaImg.Pix[i+2]) * gains[2])
	}

	return nrgbaImg, ctx.Err()
}

// estimateWhiteBalanceGains computes the per-channel gains from opaque neutral
// pixels. It returns ok=false when no usable neutral pixel exists or when the
// computed values are not finite.
func estimateWhiteBalanceGains(img *image.NRGBA, maxGain float64) (gains [3]float64, ok bool) {
	var sum [3]float64
	var count float64

	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i+3] != 255 {
			continue
		}

		r := float64(img.Pix[i])
		g := float64(img.Pix[i+1])
		b := float64(img.Pix[i+2])

		maxChannel := math.Max(r, math.Max(g, b))
		minChannel := math.Min(r, math.Min(g, b))
		luma := (r + g + b) / 3

		if maxChannel == 0 || luma < neutralLumaMin || luma > neutralLumaMax {
			continue
		}

		// Relative chroma tells how far the pixel is from a neutral gray.
		if (maxChannel-minChannel)/maxChannel > neutralChromaTolerance {
			continue
		}

		sum[0] += r
		sum[1] += g
		sum[2] += b
		count++
	}

	if count == 0 {
		return gains, false
	}

	means := [3]float64{sum[0] / count, sum[1] / count, sum[2] / count}

	// Gray world: the average of the neutral references should be achromatic,
	// so each channel is pulled towards the common reference level.
	reference := (means[0] + means[1] + means[2]) / 3

	minGain := 1 / maxGain

	for i := 0; i < 3; i++ {
		if means[i] == 0 {
			return gains, false
		}

		gain := reference / means[i]

		if math.IsNaN(gain) || math.IsInf(gain, 0) {
			return gains, false
		}

		gain = math.Max(minGain, math.Min(maxGain, gain))
		gains[i] = gain
	}

	return gains, true
}

// toNRGBA converts any image to *image.NRGBA so pixels can be edited directly.
// An already NRGBA image is copied byte by byte, guaranteeing that untouched
// pixels are preserved exactly.
func toNRGBA(img image.Image) *image.NRGBA {
	bounds := img.Bounds()

	if source, ok := img.(*image.NRGBA); ok {
		dst := image.NewNRGBA(bounds)
		copy(dst.Pix, source.Pix)

		return dst
	}

	dst := image.NewNRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			dst.SetNRGBA(x, y, c)
		}
	}

	return dst
}

func clampChannel(value float64) uint8 {
	rounded := math.Round(value)

	if rounded <= 0 {
		return 0
	}

	if rounded >= 255 {
		return 255
	}

	return uint8(rounded)
}
