package api

import (
	"fmt"
	"net/http"
	"strings"

	"rsc.io/qr"
)

// qrSVG renders text as a QR code (SVG, 4-module quiet zone). Used by the console to show
// enrollment links that an iPhone or iPad can open with its camera.
func qrSVG(text string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	n := code.Size + 8
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="`, n, n, n, n)
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x+4, y+4)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String(), nil
}

// qrCode serves GET /api/v1/qr?text= as an SVG image.
func (h *Handler) qrCode(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" || len(text) > 1024 {
		writeErr(w, 400, "text must be 1-1024 characters")
		return
	}
	svg, err := qrSVG(text)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Write([]byte(svg))
}
