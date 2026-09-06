package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/disintegration/imaging"
	"github.com/gabriel-vasile/mimetype"
	"github.com/mat/besticon/ico"
	"github.com/mattn/go-isatty"
	"golang.org/x/image/webp"
)

const (
	RESIZE_FACTOR_Y   = 2
	RESIZE_FACTOR_X   = 1
	DEFAULT_TERM_COLS = 80
	DEFAULT_TERM_ROWS = 24
	FPS               = 15
)

const (
	ANSI_CURSOR_UP            = "\x1B[%dA"
	ANSI_CURSOR_HIDE          = "\x1B[?25l"
	ANSI_CURSOR_SHOW          = "\x1B[?25h"
	ANSI_BG_TRANSPARENT_COLOR = "\x1b[0;39;49m"
	ANSI_BG_RGB_COLOR         = "\x1b[48;2;%d;%d;%dm"
	ANSI_FG_TRANSPARENT_COLOR = "\x1b[0m "
	ANSI_FG_RGB_COLOR         = "\x1b[38;2;%d;%d;%dm"
	ANSI_RESET                = "\x1b[0m"

	ANSI_FG_DEFAULT = "\x1b[39m"
	ANSI_BG_DEFAULT = "\x1b[49m"

	BOTTOM_BLOCK = "▄"
	TOP_BLOCK    = "▀"
	FULL_BLOCK   = "█"
	EMPTY_BLOCK  = " "
)

var (
	interpolationType = imaging.Lanczos
	optimize          = false
	imageOperation    = imaging.Fit
	termCols          = 0
	termRows          = 0
	topOffset         = 1
	silent            = false
)

type lineRenderer func(f image.Image, y, maxX int) string

var renderLine lineRenderer = renderLineNoOptimization

func renderLineNoOptimization(f image.Image, y, maxX int) string {
	var sb strings.Builder

	for x := 0; x < maxX; x++ {
		r, g, b, a := f.At(x, y).RGBA()

		if a>>8 < 128 {
			sb.WriteString(ANSI_BG_TRANSPARENT_COLOR)
		} else {
			sb.WriteString(fmt.Sprintf(ANSI_BG_RGB_COLOR, r>>8, g>>8, b>>8))
		}

		r, g, b, a = f.At(x, y+1).RGBA()
		if a>>8 < 128 {
			sb.WriteString(ANSI_FG_TRANSPARENT_COLOR)
		} else {
			sb.WriteString(fmt.Sprintf(ANSI_FG_RGB_COLOR, r>>8, g>>8, b>>8))
			sb.WriteString(BOTTOM_BLOCK)
		}
	}

	sb.WriteString(ANSI_RESET)
	sb.WriteString("\n")

	return sb.String()
}

type colorState struct {
	FG color.Color
	BG color.Color
}

const ()

func printColor(cs *colorState, c color.Color, isBackground bool) string {
	r, g, b, a := c.RGBA()
	if a>>8 < 128 {
		if isBackground {
			cs.BG = color.Transparent
			return ANSI_BG_DEFAULT
		} else {
			cs.FG = color.Transparent
			return ANSI_FG_DEFAULT
		}
	} else {
		if isBackground {
			cs.BG = color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
			return fmt.Sprintf(ANSI_BG_RGB_COLOR, r>>8, g>>8, b>>8)
		} else {
			cs.FG = color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
			return fmt.Sprintf(ANSI_FG_RGB_COLOR, r>>8, g>>8, b>>8)
		}
	}
}

func isSameColor(c1, c2 color.Color) bool {
	r1, g1, b1, a1 := c1.RGBA()
	r2, g2, b2, a2 := c2.RGBA()
	// Since transparency is binary, we can consider two colors the same
	// if they are both transparent regardless of their RGB values. Otherwise,
	// we compare the RGBA values.
	t1 := a1>>8 < 128
	t2 := a2>>8 < 128
	if t1 || t2 {
		return t1 && t2
	}
	return r1>>8 == r2>>8 && g1>>8 == g2>>8 && b1>>8 == b2>>8
}

func lookaheadColor(cs *colorState, f image.Image, x, y, maxX int) (color.Color, color.Color) {
	// look ahead to see the next 2 colors
	// TODO
	// the only reason to call this function is if both pixels are the same color and neither of them is the same as the current foreground or background color.

	current := f.At(x, y)

	for i := x + 1; i < maxX; i++ {
		c1 := f.At(i, y)
		c2 := f.At(i, y+1)
		// we need to fill out 2 color slots using the current single color and try to avoid moving or overwriting the current foreground and background colors. If we find a color that is the same as the current foreground or background color, we can use that color to fill out the 2 color slots. Otherwise, we can use the current single color to fill out the 2 color slots.
		if isSameColor(c1, cs.FG) || isSameColor(c2, cs.FG) {
			return current, cs.BG
		}
		if isSameColor(c1, cs.BG) || isSameColor(c2, cs.BG) {
			return cs.FG, current
		}
	}
	return cs.FG, cs.BG
}

func renderLineWithOptimization(f image.Image, y, maxX int) string {
	cs := &colorState{FG: color.Transparent, BG: color.Transparent}

	var sb strings.Builder
	for x := 0; x < maxX; x++ {
		if isSameColor(f.At(x, y), f.At(x, y+1)) {
			// check if the one color is the same as the current background or foreground color
			if isSameColor(f.At(x, y), cs.BG) {
				sb.WriteString(EMPTY_BLOCK)
			} else if isSameColor(f.At(x, y), cs.FG) {
				sb.WriteString(FULL_BLOCK)
			} else {
				// look ahead to see whether a future column can reuse the current
				// FG or BG slot; assign this pixel to whichever slot isn't worth preserving.
				nextFG, _ := lookaheadColor(cs, f, x, y, maxX)
				if !isSameColor(nextFG, cs.FG) {
					sb.WriteString(printColor(cs, f.At(x, y), false))
					sb.WriteString(FULL_BLOCK)
				} else {
					sb.WriteString(printColor(cs, f.At(x, y), true))
					sb.WriteString(EMPTY_BLOCK)
				}
			}
		} else {
			// top and bottom colors are different, check if they are the same as previous colors
			if isSameColor(f.At(x, y), cs.FG) && isSameColor(f.At(x, y+1), cs.BG) {
				sb.WriteString(TOP_BLOCK)
			} else if isSameColor(f.At(x, y), cs.BG) && isSameColor(f.At(x, y+1), cs.FG) {
				sb.WriteString(BOTTOM_BLOCK)
			} else if isSameColor(f.At(x, y), cs.FG) {
				// keep FG for the top pixel, put the new bottom color in BG
				sb.WriteString(printColor(cs, f.At(x, y+1), true))
				sb.WriteString(TOP_BLOCK)
			} else if isSameColor(f.At(x, y+1), cs.FG) {
				// keep FG for the bottom pixel, put the new top color in BG
				sb.WriteString(printColor(cs, f.At(x, y), true))
				sb.WriteString(BOTTOM_BLOCK)
			} else if isSameColor(f.At(x, y), cs.BG) {
				// keep BG for the top pixel, put the new bottom color in FG
				sb.WriteString(printColor(cs, f.At(x, y+1), false))
				sb.WriteString(BOTTOM_BLOCK)
			} else if isSameColor(f.At(x, y+1), cs.BG) {
				// keep BG for the bottom pixel, put the new top color in FG
				sb.WriteString(printColor(cs, f.At(x, y), false))
				sb.WriteString(TOP_BLOCK)
			} else {
				// both new colors are different from previous colors, print both colors
				sb.WriteString(printColor(cs, f.At(x, y), true))
				sb.WriteString(printColor(cs, f.At(x, y+1), false))
				sb.WriteString(BOTTOM_BLOCK)
			}
		}
	}

	sb.WriteString(ANSI_RESET)
	sb.WriteString("\n")

	return sb.String()
}

func read(input string) []byte {
	var err error
	var buf []byte

	if input == "" {
		if buf, err = io.ReadAll(os.Stdin); err != nil {
			log.Panicf("failed to read the stdin: %v", err)
		}
	} else {
		if buf, err = os.ReadFile(input); err != nil {
			log.Panicf("failed to read the input file: %v", err)
		}
	}

	return buf
}

func decode(buf []byte) []image.Image {
	mime, err := mimetype.DetectReader(bytes.NewReader(buf))
	if err != nil {
		log.Panicf("failed to detect the mime type: %v", err)
	}

	allowed := []string{"image/gif", "image/png", "image/jpeg", "image/bmp", "image/x-icon", "image/webp"}
	if !mimetype.EqualsAny(mime.String(), allowed...) {
		log.Fatalf("invalid MIME type: %s", mime.String())
	}

	frames := make([]image.Image, 0)

	if mime.Is("image/gif") {
		gifImage, err := gif.DecodeAll(bytes.NewReader(buf))

		if err != nil {
			log.Panicf("failed to decode the gif: %v", err)
		}

		var lowestX int
		var lowestY int
		var highestX int
		var highestY int

		for _, img := range gifImage.Image {
			if img.Rect.Min.X < lowestX {
				lowestX = img.Rect.Min.X
			}
			if img.Rect.Min.Y < lowestY {
				lowestY = img.Rect.Min.Y
			}
			if img.Rect.Max.X > highestX {
				highestX = img.Rect.Max.X
			}
			if img.Rect.Max.Y > highestY {
				highestY = img.Rect.Max.Y
			}
		}

		imgWidth := highestX - lowestX
		imgHeight := highestY - lowestY

		overPaintImage := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))
		draw.Draw(overPaintImage, overPaintImage.Bounds(), gifImage.Image[0], image.Point{}, draw.Src)

		for _, srcImg := range gifImage.Image {
			draw.Draw(overPaintImage, overPaintImage.Bounds(), srcImg, image.Point{}, draw.Over)
			frame := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))
			draw.Draw(frame, frame.Bounds(), overPaintImage, image.Point{}, draw.Over)
			frames = append(frames, frame)
		}

		return frames
	} else {
		var frame image.Image
		var err error

		if mime.Is("image/x-icon") {
			frame, err = ico.Decode(bytes.NewReader(buf))
		} else if mime.Is("image/webp") {
			frame, err = webp.Decode(bytes.NewReader(buf))
		} else {
			frame, _, err = image.Decode(bytes.NewReader(buf))
		}

		if err != nil {
			log.Panicf("failed to decode the image: %v", err)
		}

		imb := frame.Bounds()
		if imb.Max.X < 2 || imb.Max.Y < 2 {
			log.Fatal("the input image is too small")
		}

		frames = append(frames, frame)
	}

	return frames
}

func imgSize() (rows, cols int) {
	// figure out real terminal size
	if isatty.IsTerminal(os.Stdout.Fd()) {
		rows, cols, _ = pty.Getsize(os.Stdout)
	}

	// account user specified size override
	if termRows > 0 {
		rows = termRows
	}
	if termCols > 0 {
		cols = termCols
	}

	// fallback to default terminal size
	if rows < 1 {
		rows = DEFAULT_TERM_ROWS
	}
	if cols < 1 {
		cols = DEFAULT_TERM_COLS
	}
	return
}

func scale(frames []image.Image) []image.Image {
	type data struct {
		i  int
		im image.Image
	}

	rows, cols := imgSize()

	w := cols * RESIZE_FACTOR_X
	h := (rows - topOffset) * RESIZE_FACTOR_Y

	l := len(frames)
	r := make([]image.Image, l)
	c := make(chan *data, l)

	for i, f := range frames {
		go func(i int, f image.Image) {
			c <- &data{i, imageOperation(f, w, h, interpolationType)}
		}(i, f)
	}

	for range r {
		d := <-c
		r[d.i] = d.im
	}

	return r
}

func escape(frames []image.Image) [][]string {
	type data struct {
		i   int
		str string
	}

	escaped := make([][]string, 0)

	for _, f := range frames {
		imb := f.Bounds()
		maxY := imb.Max.Y - imb.Max.Y%2
		maxX := imb.Max.X

		c := make(chan *data, maxY/2)
		lines := make([]string, maxY/2)

		for y := 0; y < maxY; y += 2 {
			go func(y int) {
				c <- &data{y / 2, renderLine(f, y, maxX)}
			}(y)
		}

		for range lines {
			line := <-c
			lines[line.i] = line.str
		}

		escaped = append(escaped, lines)
	}

	return escaped
}

func print(frames [][]string) {
	if isatty.IsTerminal(os.Stdout.Fd()) {
		defer enableEcho(disableEcho())
	}

	os.Stdout.WriteString(ANSI_CURSOR_HIDE)
	for i := 0; i < topOffset; i++ {
		os.Stdout.WriteString("\n")
	}

	frameCount := len(frames)

	if frameCount == 1 {
		os.Stdout.WriteString(strings.Join(frames[0], ""))
	} else {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		defer signal.Stop(c)

		h := len(frames[0])
		if !silent {
			h += 2 // two extra lines for the exit msg
		}
		playing := true

		go func() {
			<-c
			playing = false
		}()

		tick := time.NewTicker(time.Second / time.Duration(FPS))
		defer tick.Stop()

		for i := 0; playing; i++ {
			if i != 0 {
				os.Stdout.WriteString(fmt.Sprintf(ANSI_CURSOR_UP, h))
			}

			os.Stdout.WriteString(strings.Join(frames[i%frameCount], ""))
			if !silent {
				os.Stdout.WriteString("\npress `ctrl c` to exit\n")
			}

			<-tick.C
		}
	}
	os.Stdout.WriteString(ANSI_RESET)
	os.Stdout.WriteString(ANSI_CURSOR_SHOW)
}

func main() {
	interpolation := flag.String("interpolation", "lanczos", "Interpolation method. Options: lanczos, nearest")
	resizeType := flag.String("type", "fit", "Image resize type. Options: fit, resize")
	flag.IntVar(&termCols, "cols", termCols, "Number of terminal columns to use for rendering the image")
	flag.IntVar(&termRows, "rows", termRows, "Number of terminal rows to use for rendering the image")
	flag.IntVar(&topOffset, "top-offset", topOffset, "Offset from the top of the terminal to start rendering the image")
	flag.BoolVar(&optimize, "optimize", false, "Enable output optimization to reduce the number of ANSI escape sequences used for rendering the image")
	flag.BoolVar(&silent, "silent", false, "Hide exit message")

	ParseFlags()

	input := ""
	if len(flag.Args()) > 0 {
		args := flag.Args()
		input = args[0]
	} else if isatty.IsTerminal(os.Stdin.Fd()) {
		flag.Usage()
		os.Exit(1)
	}

	switch *interpolation {
	case "nearest":
		interpolationType = imaging.NearestNeighbor
	default:
		interpolationType = imaging.Lanczos
	}

	switch *resizeType {
	case "resize":
		imageOperation = imaging.Resize
	default:
		imageOperation = imaging.Fit
	}

	if optimize {
		renderLine = renderLineWithOptimization
	}

	print(escape(scale(decode(read(input)))))
}
