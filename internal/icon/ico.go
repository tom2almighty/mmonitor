package icon

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
)

type svgDoc struct {
	ViewBox string   `xml:"viewBox,attr"`
	Rects   []svgRec `xml:"rect"`
	Groups  []svgGrp `xml:"g"`
	Paths   []svgPth `xml:"path"`
}

type svgRec struct {
	Width  float64 `xml:"width,attr"`
	Height float64 `xml:"height,attr"`
	RX     float64 `xml:"rx,attr"`
	Fill   string  `xml:"fill,attr"`
}

type svgGrp struct {
	Paths []svgPth `xml:"path"`
}

type svgPth struct {
	Stroke string `xml:"stroke,attr"`
}

// GenerateICOFromSVG renders a multi-resolution Windows .ico (16/32/48/256px)
// directly from the embedded brand logo SVG, extracting background fill, corner
// radius, and stroke colors from the SVG XML.
func GenerateICOFromSVG(svgData []byte) ([]byte, error) {
	bg := color.NRGBA{R: 0x18, G: 0x18, B: 0x1B, A: 0xFF}
	fg := color.NRGBA{R: 0xFA, G: 0xFA, B: 0xFA, A: 0xFF}
	radiusRatio := 0.22

	var doc svgDoc
	if err := xml.Unmarshal(svgData, &doc); err != nil {
		return nil, fmt.Errorf("parse logo svg: %w", err)
	}
	if len(doc.Rects) > 0 {
		r := doc.Rects[0]
		if parsed, ok := parseHexColor(r.Fill); ok {
			bg = parsed
		}
		if r.Width > 0 && r.RX > 0 {
			radiusRatio = r.RX / r.Width
		}
	}
	for _, g := range doc.Groups {
		for _, p := range g.Paths {
			if parsed, ok := parseHexColor(p.Stroke); ok {
				fg = parsed
			}
		}
	}
	for _, p := range doc.Paths {
		if parsed, ok := parseHexColor(p.Stroke); ok {
			fg = parsed
		}
	}

	sizes := []int{16, 32, 48, 256}
	type entry struct {
		size int
		data []byte
	}
	entries := make([]entry, 0, len(sizes))
	for _, sz := range sizes {
		img := renderActivityIcon(sz, bg, fg, radiusRatio)
		if sz == 256 {
			var pngBuf bytes.Buffer
			if err := png.Encode(&pngBuf, img); err != nil {
				return nil, err
			}
			entries = append(entries, entry{size: sz, data: pngBuf.Bytes()})
		} else {
			entries = append(entries, entry{size: sz, data: encodeBMPForICO(img)})
		}
	}

	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint16(0))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(len(entries)))

	offset := uint32(6 + 16*len(entries))
	for _, e := range entries {
		w, h := byte(e.size), byte(e.size)
		if e.size >= 256 {
			w, h = 0, 0
		}
		out.WriteByte(w)
		out.WriteByte(h)
		out.WriteByte(0)
		out.WriteByte(0)
		_ = binary.Write(&out, binary.LittleEndian, uint16(1))
		_ = binary.Write(&out, binary.LittleEndian, uint16(32))
		_ = binary.Write(&out, binary.LittleEndian, uint32(len(e.data)))
		_ = binary.Write(&out, binary.LittleEndian, offset)
		offset += uint32(len(e.data))
	}
	for _, e := range entries {
		out.Write(e.data)
	}
	return out.Bytes(), nil
}

func parseHexColor(raw string) (color.NRGBA, bool) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "#")
	if len(raw) != 6 {
		return color.NRGBA{}, false
	}
	v, err := strconv.ParseUint(raw, 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{
		R: uint8((v >> 16) & 0xFF),
		G: uint8((v >> 8) & 0xFF),
		B: uint8(v & 0xFF),
		A: 0xFF,
	}, true
}

func renderActivityIcon(size int, bg, fg color.NRGBA, radiusRatio float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	pts := [][2]float64{
		{2.0, 12.0},
		{5.5, 12.0},
		{9.0, 2.8},
		{15.0, 21.2},
		{18.5, 12.0},
		{22.0, 12.0},
	}

	const ss = 4
	s := float64(size)
	radius := s * radiusRatio
	strokeHalf := 1.25
	if size <= 16 {
		strokeHalf = 1.65
	} else if size <= 32 {
		strokeHalf = 1.4
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var rSum, gSum, bSum, aSum float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/ss
					py := float64(y) + (float64(sy)+0.5)/ss
					if !inRoundedRect(px, py, s, s, radius) {
						continue
					}
					pad := s * 0.17
					inner := s - 2*pad
					lx := (px - pad) / inner * 24.0
					ly := (py - pad) / inner * 24.0
					c := bg
					if minDistPolyline(lx, ly, pts) <= strokeHalf {
						c = fg
					}
					rSum += float64(c.R)
					gSum += float64(c.G)
					bSum += float64(c.B)
					aSum += float64(c.A)
				}
			}
			const total = ss * ss
			if aSum > 0 {
				img.SetNRGBA(x, y, color.NRGBA{
					R: uint8(math.Round(rSum / aSum * 255.0)),
					G: uint8(math.Round(gSum / aSum * 255.0)),
					B: uint8(math.Round(bSum / aSum * 255.0)),
					A: uint8(math.Round(aSum / total)),
				})
			}
		}
	}
	return img
}

func inRoundedRect(x, y, w, h, r float64) bool {
	if x < 0 || y < 0 || x > w || y > h {
		return false
	}
	cx := math.Max(r, math.Min(x, w-r))
	cy := math.Max(r, math.Min(y, h-r))
	dx := x - cx
	dy := y - cy
	return dx*dx+dy*dy <= r*r
}

func minDistPolyline(px, py float64, pts [][2]float64) float64 {
	best := math.MaxFloat64
	for i := 0; i < len(pts)-1; i++ {
		d := distToSegment(px, py, pts[i][0], pts[i][1], pts[i+1][0], pts[i+1][1])
		if d < best {
			best = d
		}
	}
	return best
}

func distToSegment(px, py, x1, y1, x2, y2 float64) float64 {
	dx, dy := x2-x1, y2-y1
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-x1, py-y1)
	}
	t := math.Max(0, math.Min(1, ((px-x1)*dx+(py-y1)*dy)/l2))
	return math.Hypot(px-(x1+t*dx), py-(y1+t*dy))
}

func encodeBMPForICO(img *image.NRGBA) []byte {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	andStride := ((w + 31) / 32) * 4

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint32(40))
	_ = binary.Write(&buf, binary.LittleEndian, int32(w))
	_ = binary.Write(&buf, binary.LittleEndian, int32(h*2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(w*h*4+andStride*h))
	_ = binary.Write(&buf, binary.LittleEndian, int32(0))
	_ = binary.Write(&buf, binary.LittleEndian, int32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))

	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x, y)
			buf.WriteByte(c.B)
			buf.WriteByte(c.G)
			buf.WriteByte(c.R)
			buf.WriteByte(c.A)
		}
	}
	buf.Write(make([]byte, andStride*h))
	return buf.Bytes()
}
