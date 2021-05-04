package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/gabriel-vasile/mimetype"
	"github.com/integrii/flaggy"
	"github.com/mat/besticon/ico"
	"golang.org/x/crypto/ssh/terminal"
)

const RESIZE_OFFSET_Y = 8
const RESIZE_FACTOR_Y = 2
const RESIZE_FACTOR_X = 1
const DEFAULT_TERM_WIDTH = 80
const DEFAULT_TERM_HEIGHT = 24
const FPS = 15

const ANSI_CURSOR_UP = "\x1B[%dA"
const ANSI_CURSOR_HIDE = "\x1B[?25l"
const ANSI_CURSOR_SHOW = "\x1B[?25h"
const ANSI_BG_TRANSPARENT_COLOR = "\x1b[0;39;49m"
const ANSI_BG_RGB_COLOR = "\x1b[48;2;%d;%d;%dm"
const ANSI_FG_TRANSPARENT_COLOR = "\x1b[0m "
const ANSI_FG_RGB_COLOR = "\x1b[38;2;%d;%d;%dm▄"
const ANSI_RESET = "\x1b[0m"

func read(input string) io.Reader {
	if input == "stdin" {
		return bufio.NewReader(os.Stdin)
	} else {
		f, err := os.Open(input)
		if err != nil {
			log.Fatalf("failed to read the image: %v", err)
		}

		return bufio.NewReader(f)
	}
}

func decode(r io.Reader) []image.Image {
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		log.Fatal("failed to read the input")
	}

	mime, err := mimetype.DetectReader(bytes.NewReader(buf))
	if err != nil {
		log.Fatal("failed to read the input MIME type")
	}

	allowed := []string{"image/gif", "image/png", "image/jpeg", "image/bmp", "image/x-icon"}
	if !mimetype.EqualsAny(mime.String(), allowed...) {
		log.Fatal("Invalid MIME type")
	}

	frames := make([]image.Image, 0)

	if mime.Is("image/gif") {
		gifImage, err := gif.DecodeAll(bytes.NewReader(buf))

		if err != nil {
			log.Fatalf("failed to decode the gif: %v", err)
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

		if mime.Is("image/gif") {
			frame, err = ico.Decode(bytes.NewReader(buf))
		} else {
			frame, _, err = image.Decode(bytes.NewReader(buf))
		}

		if err != nil {
			log.Fatalf("failed to decode the image: %v", err)
		}

		imb := frame.Bounds()
		if imb.Max.X < 2 || imb.Max.Y < 2 {
			log.Fatal("the input image is to small")
		}

		frames = append(frames, frame)
	}

	return frames
}

func scale(frames []image.Image) []image.Image {
	type data struct {
		i  int
		im image.Image
	}

	var err error

	width := DEFAULT_TERM_WIDTH
	height := DEFAULT_TERM_HEIGHT

	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		if width, height, err = terminal.GetSize(int(os.Stdout.Fd())); err != nil {
			log.Fatalf("failed to get the terminal size: %v", err)
		}
	}

	w := width * RESIZE_FACTOR_X
	h := (height - RESIZE_OFFSET_Y) * RESIZE_FACTOR_Y

	l := len(frames)
	r := make([]image.Image, l)
	c := make(chan *data, l)

	for i, f := range frames {
		go func(i int, f image.Image) {
			c <- &data{i, imaging.Fit(f, w, h, imaging.Lanczos)}
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
					}
				}

				sb.WriteString(ANSI_RESET)
				sb.WriteString("\n")

				c <- &data{y / 2, sb.String()}
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
	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		defer enableEcho(disableEcho())
	}

	os.Stdout.WriteString(ANSI_CURSOR_HIDE)
	os.Stdout.WriteString("\n")

	frameCount := len(frames)

	if frameCount == 1 {
		os.Stdout.WriteString(strings.Join(frames[0], ""))
	} else {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)

		tick := time.Tick(time.Second / time.Duration(FPS))
		h := len(frames[0]) + 2 // two extra lines for the exit msg
		playing := true

		go func() {
			<-c
			playing = false
		}()

		for i := 0; playing; i++ {
			if i != 0 {
				os.Stdout.WriteString(fmt.Sprintf(ANSI_CURSOR_UP, h))
			}

			os.Stdout.WriteString(strings.Join(frames[i%frameCount], ""))
			os.Stdout.WriteString("\npress `ctrl c` to exit\n")

			<-tick
		}
	}

	os.Stdout.WriteString(ANSI_CURSOR_SHOW)
}

func main() {
	input := "stdin"

	flaggy.DefaultParser.Name = "imgcat"
	flaggy.DefaultParser.Version = "1.0.7"
	flaggy.AddPositionalValue(&input, "input", 1, false, "The input image.")
	flaggy.Parse()

	print(escape(scale(decode(read(input)))))
}
