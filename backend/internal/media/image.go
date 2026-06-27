package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"io"
)

// jpegQuality is the encode quality for derived tiers — high enough for web
// viewing/thumbnails while keeping the derived objects small.
const jpegQuality = 82

// resizeJPEGOrCopy decodes src as an image, downscales it so its longest edge is
// at most maxEdge (preserving aspect ratio; never upscales), and re-encodes JPEG.
// If src cannot be decoded as an image, it returns src unchanged with the
// original content type — the derive job must still produce a signable sibling
// row rather than fail (task DoD), so a non-image or already-tiny original simply
// passes through verbatim.
//
// The downscaler is a stdlib-only area-average (box) filter so the media package
// needs no external image dependency. It is more than adequate for web/thumb
// previews; the panorama viewer reads the original tier for full fidelity.
func resizeJPEGOrCopy(src []byte, maxEdge int, originalContentType string) (data []byte, contentType string) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return src, fallbackContentType(originalContentType)
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src, fallbackContentType(originalContentType)
	}

	nw, nh := scaledDims(w, h, maxEdge)
	if nw == w && nh == h {
		// Already within target — re-encode to normalize; fall back to source on
		// an encode error.
		if out, ok := encodeJPEG(img); ok {
			return out, "image/jpeg"
		}
		return src, fallbackContentType(originalContentType)
	}

	dst := boxDownscale(img, nw, nh)
	if out, ok := encodeJPEG(dst); ok {
		return out, "image/jpeg"
	}
	return src, fallbackContentType(originalContentType)
}

// scaledDims returns the dimensions that fit (w,h) within a maxEdge longest edge,
// preserving aspect ratio and never upscaling.
func scaledDims(w, h, maxEdge int) (int, int) {
	longest := w
	if h > longest {
		longest = h
	}
	if maxEdge <= 0 || longest <= maxEdge {
		return w, h
	}
	ratio := float64(maxEdge) / float64(longest)
	nw := int(float64(w) * ratio)
	nh := int(float64(h) * ratio)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	return nw, nh
}

// boxDownscale resizes src down to (nw,nh) by averaging each destination pixel's
// source-region color (a simple, allocation-light box filter). Inputs are
// assumed already validated as a downscale (nw<=w, nh<=h).
func boxDownscale(src image.Image, nw, nh int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))

	for dy := 0; dy < nh; dy++ {
		sy0 := b.Min.Y + dy*h/nh
		sy1 := b.Min.Y + (dy+1)*h/nh
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for dx := 0; dx < nw; dx++ {
			sx0 := b.Min.X + dx*w/nw
			sx1 := b.Min.X + (dx+1)*w/nw
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			dst.Set(dx, dy, averageRegion(src, sx0, sy0, sx1, sy1))
		}
	}
	return dst
}

// averageRegion returns the mean RGBA color over the half-open source rectangle
// [x0,x1)×[y0,y1). Color channels are 16-bit (the image/color model) averaged
// then narrowed to 8-bit for the RGBA destination.
func averageRegion(src image.Image, x0, y0, x1, y1 int) color.RGBA {
	var rSum, gSum, bSum, aSum uint64
	var n uint64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, b, a := src.At(x, y).RGBA() // 0..65535
			rSum += uint64(r)
			gSum += uint64(g)
			bSum += uint64(b)
			aSum += uint64(a)
			n++
		}
	}
	if n == 0 {
		return color.RGBA{}
	}
	return color.RGBA{
		R: uint8((rSum / n) >> 8),
		G: uint8((gSum / n) >> 8),
		B: uint8((bSum / n) >> 8),
		A: uint8((aSum / n) >> 8),
	}
}

// encodeJPEG encodes img to JPEG bytes; ok is false on an encode error.
func encodeJPEG(img image.Image) ([]byte, bool) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// fallbackContentType returns the original content type, defaulting to
// image/jpeg when empty.
func fallbackContentType(ct string) string {
	if ct == "" {
		return "image/jpeg"
	}
	return ct
}

// bytesReader wraps a byte slice in an io.Reader the COS client can size.
func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// readAllClose reads all of rc and closes it.
func readAllClose(rc io.ReadCloser) ([]byte, error) {
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}
