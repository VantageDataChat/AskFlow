// Package captcha generates arithmetic-based image CAPTCHAs using vector fonts.
package captcha

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	mrand "math/rand"
	"strconv"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type entry struct {
	answer    string
	expiresAt time.Time
}

var (
	store    = make(map[string]entry)
	mu       sync.Mutex
	fontFace font.Face
)

func init() {
	tt, err := opentype.Parse(gobold.TTF)
	if err != nil {
		panic("captcha: failed to parse font: " + err.Error())
	}
	fontFace, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    36,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		panic("captcha: failed to create font face: " + err.Error())
	}
}

// Response holds the captcha ID and base64-encoded PNG image.
type Response struct {
	ID    string `json:"id"`
	Image string `json:"image"` // data:image/png;base64,...
}

// Generate creates a new arithmetic captcha and returns its ID + base64 PNG.
func Generate() *Response {
	mu.Lock()
	defer mu.Unlock()

	// Clean expired
	now := time.Now()
	for k, v := range store {
		if now.After(v.expiresAt) {
			delete(store, k)
		}
	}

	expr, answer := generateArithmetic()

	id := generateCaptchaID()
	store[id] = entry{
		answer:    answer,
		expiresAt: now.Add(5 * time.Minute),
	}

	img := renderCaptcha(expr + "= ?")

	var buf bytes.Buffer
	png.Encode(&buf, img)
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	return &Response{
		ID:    id,
		Image: "data:image/png;base64," + b64,
	}
}

// generateArithmetic produces a simple arithmetic expression and its integer answer.
func generateArithmetic() (expr string, answer string) {
	ops := []string{"+", "-", "*", "/"}
	op := ops[mrand.Intn(len(ops))]

	var a, b, result int
	switch op {
	case "+":
		a = 10 + mrand.Intn(90)
		b = 1 + mrand.Intn(9)
		result = a + b
	case "-":
		a = 10 + mrand.Intn(90)
		b = 1 + mrand.Intn(9)
		if a < b {
			a, b = b, a
		}
		result = a - b
	case "*":
		a = 10 + mrand.Intn(10)
		b = 2 + mrand.Intn(8)
		result = a * b
	case "/":
		b = 2 + mrand.Intn(8)
		quotient := 2 + mrand.Intn(18)
		a = b * quotient
		result = quotient
	}

	expr = fmt.Sprintf("%d %s %d ", a, op, b)
	answer = strconv.Itoa(result)
	return
}

// Validate checks the answer and consumes the captcha.
func Validate(id, answer string) bool {
	mu.Lock()
	defer mu.Unlock()

	e, ok := store[id]
	if !ok {
		return false
	}
	delete(store, id)
	if time.Now().After(e.expiresAt) {
		return false
	}
	return answer == e.answer
}

// renderCaptcha draws the expression using Go Bold font onto an image.
func renderCaptcha(text string) *image.RGBA {
	// Measure text width to size the image
	d := &font.Drawer{Face: fontFace}
	textWidth := d.MeasureString(text).Ceil()

	width := textWidth + 40 // 20px padding each side
	if width < 200 {
		width = 200
	}
	height := 60

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Background: light color
	bg := color.RGBA{
		uint8(240 + mrand.Intn(15)),
		uint8(240 + mrand.Intn(15)),
		uint8(240 + mrand.Intn(15)),
		255,
	}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	// Sparse noise dots
	for i := 0; i < 50; i++ {
		img.Set(mrand.Intn(width), mrand.Intn(height), color.RGBA{
			uint8(180 + mrand.Intn(70)),
			uint8(180 + mrand.Intn(70)),
			uint8(180 + mrand.Intn(70)),
			255,
		})
	}

	// Draw text centered
	textColor := color.RGBA{
		uint8(20 + mrand.Intn(40)),
		uint8(20 + mrand.Intn(40)),
		uint8(20 + mrand.Intn(40)),
		255,
	}
	x := (width - textWidth) / 2
	y := height/2 + 12 // baseline offset for 36pt font

	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{textColor},
		Face: fontFace,
		Dot:  fixed.P(x, y),
	}
	drawer.DrawString(text)

	return img
}

// generateCaptchaID creates a cryptographically random captcha ID.
func generateCaptchaID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("cap_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("cap_%x", b)
}
