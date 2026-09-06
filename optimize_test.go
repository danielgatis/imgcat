package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// imageFile, when set (go test -file=path/to/image.png), checks that exact
// image against ground truth instead of running the random fuzz check.
var imageFile = flag.String("file", "", "image file to check against ground truth; if empty, fuzzes random images instead")

type simColor struct {
	set     bool
	r, g, b uint8
}

func (c simColor) String() string {
	if !c.set {
		return "transparent"
	}
	return fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b)
}

var ansiRe = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// simulateLine acts like a terminal: it decodes the ANSI-escaped line into
// the actual visible top/bottom color for every column cell.
func simulateLine(s string) (top, bottom []simColor) {
	var fg, bg simColor
	idx := 0
	for idx < len(s) {
		if s[idx] == 0x1b {
			loc := ansiRe.FindStringIndex(s[idx:])
			if loc == nil || loc[0] != 0 {
				idx++
				continue
			}
			match := ansiRe.FindStringSubmatch(s[idx : idx+loc[1]])
			params := match[1]
			parts := strings.Split(params, ";")
			i := 0
			for i < len(parts) {
				switch parts[i] {
				case "", "0":
					fg = simColor{}
					bg = simColor{}
				case "39":
					fg = simColor{}
				case "49":
					bg = simColor{}
				case "38":
					if i+4 < len(parts) && parts[i+1] == "2" {
						r, _ := strconv.Atoi(parts[i+2])
						g, _ := strconv.Atoi(parts[i+3])
						b, _ := strconv.Atoi(parts[i+4])
						fg = simColor{true, uint8(r), uint8(g), uint8(b)}
						i += 4
					}
				case "48":
					if i+4 < len(parts) && parts[i+1] == "2" {
						r, _ := strconv.Atoi(parts[i+2])
						g, _ := strconv.Atoi(parts[i+3])
						b, _ := strconv.Atoi(parts[i+4])
						bg = simColor{true, uint8(r), uint8(g), uint8(b)}
						i += 4
					}
				}
				i++
			}
			idx += loc[1]
			continue
		}
		r, size := utf8.DecodeRuneInString(s[idx:])
		idx += size
		switch r {
		case '█':
			top = append(top, fg)
			bottom = append(bottom, fg)
		case ' ':
			top = append(top, bg)
			bottom = append(bottom, bg)
		case '▄':
			top = append(top, bg)
			bottom = append(bottom, fg)
		case '▀':
			top = append(top, fg)
			bottom = append(bottom, bg)
		case '\n':
		default:
		}
	}
	return
}

func pixelColor(c color.Color) simColor {
	r, g, b, a := c.RGBA()
	if a>>8 < 128 {
		return simColor{}
	}
	return simColor{true, uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
}

func groundTruth(f image.Image, y, maxX int) (top, bottom []simColor) {
	for x := 0; x < maxX; x++ {
		top = append(top, pixelColor(f.At(x, y)))
		bottom = append(bottom, pixelColor(f.At(x, y+1)))
	}
	return
}

func TestOptimizeMatchesGroundTruth(t *testing.T) {
	if *imageFile != "" {
		checkImageFile(t, *imageFile)
		return
	}
	checkFuzz(t)
}

func checkImageFile(t *testing.T, path string) {
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	img, _, err := image.Decode(fh)
	if err != nil {
		t.Fatal(err)
	}

	b := img.Bounds()
	maxX := b.Max.X
	maxY := b.Max.Y - b.Max.Y%2

	mismatches := 0
	for y := 0; y < maxY; y += 2 {
		line := renderLineWithOptimization(img, y, maxX)
		gotTop, gotBottom := simulateLine(line)
		wantTop, wantBottom := groundTruth(img, y, maxX)

		for x := 0; x < maxX; x++ {
			if x >= len(gotTop) || gotTop[x] != wantTop[x] || gotBottom[x] != wantBottom[x] {
				var g, gb simColor
				if x < len(gotTop) {
					g = gotTop[x]
					gb = gotBottom[x]
				}
				t.Errorf("row y=%d col x=%d: want top=%s bottom=%s, got top=%s bottom=%s",
					y, x, wantTop[x], wantBottom[x], g, gb)
				mismatches++
				if mismatches > 40 {
					t.Fatalf("too many mismatches, stopping (row y=%d)", y)
				}
			}
		}
	}
}

func checkFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 1000; trial++ {
		w := rng.Intn(30) + 1
		h := (rng.Intn(15) + 1) * 2
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		palette := []color.NRGBA{
			{0, 0, 0, 0},
			{255, 0, 0, 255},
			{0, 255, 0, 255},
			{0, 0, 255, 255},
			{10, 20, 30, 255},
			{200, 200, 200, 255},
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetNRGBA(x, y, palette[rng.Intn(len(palette))])
			}
		}

		for y := 0; y < h; y += 2 {
			line := renderLineWithOptimization(img, y, w)
			gotTop, gotBottom := simulateLine(line)
			wantTop, wantBottom := groundTruth(img, y, w)
			for x := 0; x < w; x++ {
				if x >= len(gotTop) || gotTop[x] != wantTop[x] || gotBottom[x] != wantBottom[x] {
					var g, gb simColor
					if x < len(gotTop) {
						g = gotTop[x]
						gb = gotBottom[x]
					}
					t.Fatalf("trial=%d w=%d h=%d row y=%d col x=%d: want top=%s bottom=%s, got top=%s bottom=%s\nline=%q",
						trial, w, h, y, x, wantTop[x], wantBottom[x], g, gb, line)
				}
			}
		}
	}
}
