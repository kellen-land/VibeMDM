// Package appicons turns an uploaded icon file (PNG, JPEG or .icns) into a safe PNG for a Mac
// App: the image is decoded and re-encoded, so nothing but its pixels survives, and scaled to
// at most MaxSide pixels. Self Service and notifications show it.
package appicons

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	_ "image/jpeg" // JPEG uploads
	"image/png"
)

const (
	// MaxUpload is the largest icon file accepted.
	MaxUpload = 4 << 20
	// MaxSide is the largest stored icon (pixels); larger images are scaled down.
	MaxSide = 256
)

var errFormat = errors.New("upload a PNG, JPEG or .icns file")

// Sanitize decodes an uploaded icon file and returns a fresh PNG of at most MaxSide pixels a side.
func Sanitize(b []byte) ([]byte, error) {
	if len(b) == 0 || len(b) > MaxUpload {
		return nil, errors.New("the icon file must be at most 4 MB")
	}
	if bytes.HasPrefix(b, []byte("icns")) {
		var err error
		if b, err = largestICNSImage(b); err != nil {
			return nil, err
		}
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, errFormat
	}
	if cfg.Width < 16 || cfg.Height < 16 || cfg.Width > 4096 || cfg.Height > 4096 {
		return nil, errors.New("the icon must be between 16 and 4096 pixels a side")
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, errFormat
	}
	var out bytes.Buffer
	if err := png.Encode(&out, downscale(img, MaxSide)); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// largestICNSImage returns the largest PNG or JPEG image inside an .icns file. (Old-style
// bitmap and JPEG 2000 entries are skipped; current icons store PNG.)
func largestICNSImage(b []byte) ([]byte, error) {
	if len(b) < 8 || int(binary.BigEndian.Uint32(b[4:8])) > len(b) {
		return nil, errors.New("the .icns file is damaged")
	}
	var best []byte
	bestW := 0
	for off := 8; off+8 <= len(b); {
		n := int(binary.BigEndian.Uint32(b[off+4 : off+8]))
		if n < 8 || off+n > len(b) {
			break
		}
		data := b[off+8 : off+n]
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil && cfg.Width > bestW {
			best, bestW = data, cfg.Width
		}
		off += n
	}
	if best == nil {
		return nil, errors.New("the .icns file has no PNG image inside; export the icon as PNG instead")
	}
	return best, nil
}

// downscale copies src into a new image, shrinking it (box filter) so neither side exceeds max.
func downscale(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w, h
	if w > max || h > max {
		if w >= h {
			nw, nh = max, max*h/w
		} else {
			nw, nh = max*w/h, max
		}
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := y*h/nh, (y+1)*h/nh
		if y1 == y0 {
			y1 = y0 + 1
		}
		for x := 0; x < nw; x++ {
			x0, x1 := x*w/nw, (x+1)*w/nw
			if x1 == x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := color.NRGBAModel.Convert(src.At(b.Min.X+xx, b.Min.Y+yy)).(color.NRGBA)
					r += uint64(c.R) * uint64(c.A)
					g += uint64(c.G) * uint64(c.A)
					bl += uint64(c.B) * uint64(c.A)
					a += uint64(c.A)
					n++
				}
			}
			if a == 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(bl / a), uint8(a / n)})
		}
	}
	return dst
}
