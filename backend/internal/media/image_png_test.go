package media

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pngOf builds a solid-color PNG of the given size, exercising the PNG decode
// branch of resizeJPEGOrCopy (the APP may upload PNG originals).
func pngOf(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// TestResize_PNGDownscales: a large PNG original decodes and downscales, and the
// derived tier is normalized to JPEG (the encode target for siblings).
func TestResize_PNGDownscales(t *testing.T) {
	src := pngOf(t, 1200, 600, color.RGBA{R: 30, G: 90, B: 150, A: 255})
	out, ct := resizeJPEGOrCopy(src, 320, "image/png")
	assert.Equal(t, "image/jpeg", ct, "derived sibling is re-encoded as JPEG")

	img, format, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, "jpeg", format)
	b := img.Bounds()
	assert.Equal(t, 320, b.Dx(), "longest edge scaled to maxEdge")
	assert.Equal(t, 160, b.Dy(), "aspect ratio preserved")
}

// TestResize_PNGWithinTarget_ReencodesToJPEG: a PNG already within the target is
// not resized but is still re-encoded as JPEG (the "nw==w && nh==h" branch).
func TestResize_PNGWithinTarget_ReencodesToJPEG(t *testing.T) {
	src := pngOf(t, 200, 150, color.RGBA{R: 200, G: 50, B: 50, A: 255})
	out, ct := resizeJPEGOrCopy(src, 1920, "image/png")
	assert.Equal(t, "image/jpeg", ct, "within-target source is normalized to JPEG")

	img, format, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, "jpeg", format)
	assert.Equal(t, 200, img.Bounds().Dx())
	assert.Equal(t, 150, img.Bounds().Dy())
}

// TestResize_TallImage_LongEdgeIsHeight: when height is the longest edge the
// clamp is applied to height, not width.
func TestResize_TallImage_LongEdgeIsHeight(t *testing.T) {
	src := jpegOf(t, 500, 2000, color.RGBA{R: 10, G: 200, B: 10, A: 255})
	out, ct := resizeJPEGOrCopy(src, 400, "image/jpeg")
	assert.Equal(t, "image/jpeg", ct)
	img, _, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, 400, img.Bounds().Dy(), "longest edge (height) clamped to maxEdge")
	assert.Equal(t, 100, img.Bounds().Dx(), "aspect ratio preserved")
}

// TestScaledDims_TinyEdgeClamp: a downscale ratio that would round a dimension to
// 0 is clamped to at least 1 px on each axis.
func TestScaledDims_TinyEdgeClamp(t *testing.T) {
	// A 1000x1 source clamped to maxEdge=1 would make height round toward 0.
	nw, nh := scaledDims(1000, 1, 1)
	assert.GreaterOrEqual(t, nw, 1)
	assert.GreaterOrEqual(t, nh, 1, "height never collapses below 1px")

	// maxEdge<=0 is a no-op (defensive guard).
	w, h := scaledDims(800, 600, 0)
	assert.Equal(t, 800, w)
	assert.Equal(t, 600, h)
}

// TestResize_TinyImageDownscale_Encodes: a 2x1 image downscaled to a 1px longest
// edge still produces a decodable JPEG (boxDownscale + averageRegion edge case).
func TestResize_TinyImageDownscale_Encodes(t *testing.T) {
	src := jpegOf(t, 2, 1, color.RGBA{R: 123, G: 45, B: 67, A: 255})
	out, ct := resizeJPEGOrCopy(src, 1, "image/jpeg")
	assert.Equal(t, "image/jpeg", ct)
	img, _, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, img.Bounds().Dx(), 1)
	assert.GreaterOrEqual(t, img.Bounds().Dy(), 1)
}

// TestFallbackContentType: empty defaults to image/jpeg; non-empty passes through.
func TestFallbackContentType(t *testing.T) {
	assert.Equal(t, "image/jpeg", fallbackContentType(""))
	assert.Equal(t, "application/octet-stream", fallbackContentType("application/octet-stream"))
	assert.Equal(t, "image/png", fallbackContentType("image/png"))
}
