package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"sync"
)

// Read supported image formats from bytes
func Decode(data []byte) (image.Image, string, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("Decoding image: %w", err)
	}
	return img, format, nil
}

// Resize image to max width and height while maintaining aspect ratio
// Uses bilinear interpolation
func Resize(img image.Image, maxW, maxH int) image.Image {
	srcBounds := img.Bounds()
	srcW, srcH := srcBounds.Dx(), srcBounds.Dy()

	if srcW <= maxW && srcH <= maxH {
		return img
	}

	ratio := float64(srcW) / float64(srcH)
	dstW, dstH := maxW, int(float64(maxW)/ratio)
	if dstH > maxH {
		dstH = maxH
		dstW = int(float64(maxH) * ratio)
	}
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}
	
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	xRatio := float64(srcW) / float64(dstW)
	yRatio := float64(srcH) / float64(dstH)

	for y := 0; y < dstH; y++ {
		srcY := float64(y) * yRatio
		y0 := int(srcY)
		y1 := min(y0+1, srcH-1)
		yLerp := srcY - float64(y0)

		for x := 0; x < dstW; x++ {
			srcX := float64(x) * xRatio
			x0 := int(srcX)
			x1 := min(x0+1, srcW-1)
			xLerp := srcX - float64(x0)

			c00 := img.At(srcBounds.Min.X+x0, srcBounds.Min.Y+y0)
			c10 := img.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y0)
			c01 := img.At(srcBounds.Min.X+x0, srcBounds.Min.Y+y1)
			c11 := img.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y1)

			dst.Set(x, y, bilerp(c00, c10, c01, c11, xLerp, yLerp))
		}
	}
	return dst
}

// Bilinear interpolation
func bilerp(c00, c10, c01, c11 color.Color, xLerp, yLerp float64) color.Color {
	r00, g00, b00, a00 := rgba64(c00)
	r10, g10, b10, a10 := rgba64(c10)
	r01, g01, b01, a01 := rgba64(c01)
	r11, g11, b11, a11 := rgba64(c11)

	top := func(v00, v10 float64) float64 { return v00 + (v10-v00)*xLerp }
	bot := func(v01, v11 float64) float64 { return v01 + (v11-v01)*xLerp }
	blend := func(v00, v10, v01, v11 float64) uint8 {
		t, b := top(v00, v10), bot(v01, v11)
		return uint8(t + (b-t)*yLerp)
	}

	return color.RGBA{
		R: blend(r00, r10, r01, r11),
		G: blend(g00, g10, g01, g11),
		B: blend(b00, b10, b01, b11),
		A: blend(a00, a10, a01, a11),
	}
}

func rgba64(c color.Color) (r, g, b, a float64) {
	cr, cg, cb, ca := c.RGBA()
	// scale down to 8bit
	return float64(cr >> 8), float64(cg >> 8), float64(cb >> 8), float64(ca >> 8)
}

// encode to jpeg at given quality
func EncodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("Encoding JPEG: %w", err)
	}
	return buf.Bytes(), nil
}

// Define one thumbnail size to generate
type ThumbnailSpec struct {
	Name string
	MaxW int
	MaxH int
}

// Encoded output of thumbnail generation
type ThumbnailResult struct {
	Name string
	JPEG []byte
	Err  error
}

// Generate thumbnails across goroutines
func GenerateThumbnails(img image.Image, specs []ThumbnailSpec, quality int) []ThumbnailResult {
	results := make([]ThumbnailResult, len(specs))
	var wg sync.WaitGroup

	for i, spec := range specs {
		wg.Add(1)
		go func(i int, spec ThumbnailSpec) {
			defer wg.Done()
			resized := Resize(img, spec.MaxW, spec.MaxH)
			data, err := EncodeJPEG(resized, quality)
			results[i] = ThumbnailResult{Name: spec.Name, JPEG: data, Err: err}
		}(i, spec)
	}

	wg.Wait()
	return results
}