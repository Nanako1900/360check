package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// TestBuildOriginalKey_Deterministic: the original key is stable for the same
// inputs and lives under a client_uuid-scoped prefix.
func TestBuildOriginalKey_Deterministic(t *testing.T) {
	cu := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	k1 := buildOriginalKey(oapi.MediaOwnerTypeProblem, 42, cu)
	k2 := buildOriginalKey(oapi.MediaOwnerTypeProblem, 42, cu)
	assert.Equal(t, k1, k2, "key must be deterministic for replay")
	assert.Equal(t, "media/problem/42/11111111-1111-1111-1111-111111111111/original.jpg", k1)
}

// TestPrefixOf_And_DeriveKey: web/thumb siblings share the original's prefix.
func TestPrefixOf_And_DeriveKey(t *testing.T) {
	cu := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	orig := buildOriginalKey(oapi.MediaOwnerTypeInspection, 7, cu)
	prefix := prefixOf(orig)
	assert.Equal(t, "media/inspection/7/22222222-2222-2222-2222-222222222222/", prefix)

	web := deriveKey(orig, oapi.Web)
	thumb := deriveKey(orig, oapi.Thumb)
	assert.Equal(t, prefix+"web.jpg", web)
	assert.Equal(t, prefix+"thumb.jpg", thumb)
	assert.True(t, len(web) > len(prefix))
}

// TestEtagMatches_QuotesAndCase: COS quotes/case are tolerated.
func TestEtagMatches_QuotesAndCase(t *testing.T) {
	assert.True(t, etagMatches(`"ABCDEF"`, "abcdef"))
	assert.True(t, etagMatches("abc", `"abc"`))
	assert.False(t, etagMatches("abc", "def"))
}

// jpegOf builds a solid-color JPEG of the given size for resize tests.
func jpegOf(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

// TestResize_Downscales: a large JPEG is downscaled so its longest edge fits
// maxEdge, preserving aspect ratio, and re-encodes as JPEG.
func TestResize_Downscales(t *testing.T) {
	src := jpegOf(t, 2000, 1000, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	out, ct := resizeJPEGOrCopy(src, 320, "image/jpeg")
	assert.Equal(t, "image/jpeg", ct)

	img, _, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	b := img.Bounds()
	assert.Equal(t, 320, b.Dx(), "longest edge scaled to maxEdge")
	assert.Equal(t, 160, b.Dy(), "aspect ratio preserved")
}

// TestResize_NoUpscale: a small image is not enlarged.
func TestResize_NoUpscale(t *testing.T) {
	src := jpegOf(t, 100, 80, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	out, ct := resizeJPEGOrCopy(src, 320, "image/jpeg")
	assert.Equal(t, "image/jpeg", ct)
	img, _, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, 100, img.Bounds().Dx())
	assert.Equal(t, 80, img.Bounds().Dy())
}

// TestResize_NonImage_Copies: undecodable bytes pass through verbatim (the derive
// job must still produce a signable sibling).
func TestResize_NonImage_Copies(t *testing.T) {
	src := []byte("this is not an image")
	out, ct := resizeJPEGOrCopy(src, 320, "application/octet-stream")
	assert.Equal(t, src, out, "non-image is copied unchanged")
	assert.Equal(t, "application/octet-stream", ct)
}

// TestScaledDims: longest-edge clamp logic.
func TestScaledDims(t *testing.T) {
	w, h := scaledDims(1920, 1080, 1920)
	assert.Equal(t, 1920, w)
	assert.Equal(t, 1080, h)

	w, h = scaledDims(4096, 2048, 1920)
	assert.Equal(t, 1920, w)
	assert.Equal(t, 960, h)

	w, h = scaledDims(800, 600, 1920) // smaller than max -> unchanged
	assert.Equal(t, 800, w)
	assert.Equal(t, 600, h)
}
