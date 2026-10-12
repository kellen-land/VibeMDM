package appicons

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func img(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.SetNRGBA(x, y, color.NRGBA{200, 50, 50, 255})
		}
	}
	return m
}

func pngOf(w, h int) []byte { var b bytes.Buffer; png.Encode(&b, img(w, h)); return b.Bytes() }

func icnsOf(entries ...[]byte) []byte {
	var body bytes.Buffer
	for i, e := range entries {
		body.WriteString([]string{"ic07", "ic09", "ic10"}[i%3])
		binary.Write(&body, binary.BigEndian, uint32(8+len(e)))
		body.Write(e)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

func size(t *testing.T, b []byte) (int, int) {
	m, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return m.Bounds().Dx(), m.Bounds().Dy()
}

func TestSanitize(t *testing.T) {
	out, err := Sanitize(pngOf(1024, 1024))
	if err != nil {
		t.Fatal(err)
	}
	if w, h := size(t, out); w != MaxSide || h != MaxSide {
		t.Fatalf("not scaled: %dx%d", w, h)
	}
	var jb bytes.Buffer
	jpeg.Encode(&jb, img(300, 150), nil)
	if out, err := Sanitize(jb.Bytes()); err != nil {
		t.Fatal(err)
	} else if w, h := size(t, out); w != 256 || h != 128 {
		t.Fatalf("jpeg: %dx%d", w, h)
	}
	// .icns: the largest PNG inside is used.
	if out, err := Sanitize(icnsOf(pngOf(128, 128), pngOf(512, 512))); err != nil {
		t.Fatal(err)
	} else if w, _ := size(t, out); w != 256 {
		t.Fatalf("icns: %d", w)
	}
	if _, err := Sanitize(icnsOf([]byte("not an image"))); err == nil {
		t.Error("icns without PNG accepted")
	}
	tainted := append(pngOf(64, 64), []byte("<script>alert(1)</script>")...)
	if out, err := Sanitize(tainted); err != nil || bytes.Contains(out, []byte("script")) {
		t.Fatalf("trailing data kept: %v", err)
	}
	for name, bad := range map[string][]byte{"empty": nil, "text": []byte("hello"), "svg": []byte("<svg xmlns='http://www.w3.org/2000/svg'/>"), "tiny": pngOf(8, 8)} {
		if _, err := Sanitize(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
