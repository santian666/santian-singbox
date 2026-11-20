package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sync"
)

const (
	trayIconKindIdle     = "idle"
	trayIconKindRunning  = "running"
	trayIconKindDegraded = "degraded"
	trayIconKindFault    = "fault"
)

var (
	trayIconVariantsOnce sync.Once
	trayIconVariants     map[string][]byte
)

// trayIconForBadge returns the tray icon ICO bytes for idle/running/degraded/fault states.
func trayIconForBadge(kind string) []byte {
	trayIconVariantsOnce.Do(func() {
		trayIconVariants = map[string][]byte{
			trayIconKindIdle: trayIcon,
		}
		specs := map[string]struct {
			fill color.NRGBA
			ring color.NRGBA
		}{
			trayIconKindRunning: {
				fill: color.NRGBA{R: 16, G: 185, B: 129, A: 255}, // #10b981
				ring: color.NRGBA{R: 124, G: 58, B: 237, A: 255}, // #7c3aed
			},
			trayIconKindDegraded: {
				fill: color.NRGBA{R: 245, G: 158, B: 11, A: 255}, // #f59e0b
				ring: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
			},
			trayIconKindFault: {
				fill: color.NRGBA{R: 239, G: 68, B: 68, A: 255}, // #ef4444
				ring: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
			},
		}
		for k, spec := range specs {
			if out, err := buildBadgedICO(trayIcon, spec.fill, spec.ring); err == nil && len(out) > 0 {
				trayIconVariants[k] = out
			} else {
				trayIconVariants[k] = trayIcon
			}
		}
	})
	if data, ok := trayIconVariants[kind]; ok && len(data) > 0 {
		return data
	}
	return trayIcon
}

func buildBadgedICO(src []byte, fill, ring color.NRGBA) ([]byte, error) {
	if len(src) < 6 {
		return src, nil
	}
	reserved := binary.LittleEndian.Uint16(src[0:2])
	typ := binary.LittleEndian.Uint16(src[2:4])
	count := int(binary.LittleEndian.Uint16(src[4:6]))
	if reserved != 0 || typ != 1 || count <= 0 || len(src) < 6+count*16 {
		return src, nil
	}

	pngSig := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	type icoEntry struct {
		dir     [16]byte
		payload []byte
	}
	entries := make([]icoEntry, count)
	for i := 0; i < count; i++ {
		off := 6 + i*16
		var dir [16]byte
		copy(dir[:], src[off:off+16])
		size := int(binary.LittleEndian.Uint32(dir[8:12]))
		imgOff := int(binary.LittleEndian.Uint32(dir[12:16]))
		if size <= 0 || imgOff < 6+count*16 || imgOff+size > len(src) {
			return src, nil
		}
		payload := src[imgOff : imgOff+size]
		if bytes.HasPrefix(payload, pngSig) {
			img, err := png.Decode(bytes.NewReader(payload))
			if err == nil {
				badged := compositeTrayStatusBadge(img, fill, ring)
				var buf bytes.Buffer
				if encErr := png.Encode(&buf, badged); encErr == nil {
					payload = buf.Bytes()
				}
			}
		}
		entries[i] = icoEntry{dir: dir, payload: payload}
	}

	var out bytes.Buffer
	headerSize := 6 + count*16
	totalSize := headerSize
	for _, e := range entries {
		totalSize += len(e.payload)
	}
	out.Grow(totalSize)
	out.Write(src[0:6])

	cursor := uint32(headerSize)
	for i := range entries {
		binary.LittleEndian.PutUint32(entries[i].dir[8:12], uint32(len(entries[i].payload)))
		binary.LittleEndian.PutUint32(entries[i].dir[12:16], cursor)
		out.Write(entries[i].dir[:])
		cursor += uint32(len(entries[i].payload))
	}
	for i := range entries {
		out.Write(entries[i].payload)
	}
	return out.Bytes(), nil
}

func compositeTrayStatusBadge(src image.Image, fill, ring color.NRGBA) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)

	minDim := float64(w)
	if float64(h) < minDim {
		minDim = float64(h)
	}
	outerR := math.Max(3.0, minDim*0.24)
	ringWidth := math.Max(1.2, minDim*0.055)
	innerR := math.Max(1.8, outerR-ringWidth)
	margin := math.Max(0.5, minDim*0.04)
	cx := float64(w) - outerR - margin
	cy := float64(h) - outerR - margin

	startX := int(math.Floor(cx - outerR - 1))
	endX := int(math.Ceil(cx + outerR + 1))
	startY := int(math.Floor(cy - outerR - 1))
	endY := int(math.Ceil(cy + outerR + 1))
	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}
	if endX >= w {
		endX = w - 1
	}
	if endY >= h {
		endY = h - 1
	}

	for y := startY; y <= endY; y++ {
		for x := startX; x <= endX; x++ {
			dist := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			if dist > outerR+0.75 {
				continue
			}
			var overlay color.NRGBA
			if dist <= innerR-0.5 {
				overlay = fill
			} else if dist <= innerR+0.5 {
				t := dist - (innerR - 0.5)
				overlay = lerpNRGBA(fill, ring, t)
			} else if dist <= outerR-0.5 {
				overlay = ring
			} else {
				cov := 1.0 - (dist-(outerR-0.5))/1.25
				if cov <= 0 {
					continue
				}
				overlay = ring
				overlay.A = uint8(math.Round(float64(ring.A) * cov))
			}
			dst.SetNRGBA(x, y, blendNRGBAOver(dst.NRGBAAt(x, y), overlay))
		}
	}
	return dst
}

func lerpNRGBA(a, b color.NRGBA, t float64) color.NRGBA {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return color.NRGBA{
		R: uint8(math.Round(float64(a.R)*(1-t) + float64(b.R)*t)),
		G: uint8(math.Round(float64(a.G)*(1-t) + float64(b.G)*t)),
		B: uint8(math.Round(float64(a.B)*(1-t) + float64(b.B)*t)),
		A: uint8(math.Round(float64(a.A)*(1-t) + float64(b.A)*t)),
	}
}

func blendNRGBAOver(dst, src color.NRGBA) color.NRGBA {
	sa := float64(src.A) / 255.0
	da := float64(dst.A) / 255.0
	outA := sa + da*(1-sa)
	if outA <= 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{
		R: uint8(math.Round((float64(src.R)*sa + float64(dst.R)*da*(1-sa)) / outA)),
		G: uint8(math.Round((float64(src.G)*sa + float64(dst.G)*da*(1-sa)) / outA)),
		B: uint8(math.Round((float64(src.B)*sa + float64(dst.B)*da*(1-sa)) / outA)),
		A: uint8(math.Round(outA * 255.0)),
	}
}
