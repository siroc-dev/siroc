package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/big"
	"strings"
	"time"
)

const (
	CaptchaAfter  = 2
	CaptchaWindow = 30 * time.Minute
	captchaTTL    = 5 * time.Minute
	captchaChars  = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	captchaLen    = 5
	lockoutAfter  = 8
)

type captchaEntry struct {
	answer string
	exp    time.Time
	ip     string
}

type Captcha struct {
	ID    string `json:"id"`
	Image string `json:"image"`
}

func (s *Service) pruneFails(st failState, now time.Time) failState {
	cut := now.Add(-CaptchaWindow)
	kept := st.at[:0]
	for _, t := range st.at {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	st.at = kept
	if !st.until.IsZero() && now.After(st.until) {
		st.until = time.Time{}
	}
	return st
}

func (s *Service) NeedCaptcha(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.fails[ip]
	if !ok {
		return false
	}
	st = s.pruneFails(st, time.Now())
	s.fails[ip] = st
	if len(st.at) == 0 && st.until.IsZero() {
		delete(s.fails, ip)
		return false
	}
	return len(st.at) >= CaptchaAfter
}

func (s *Service) IssueCaptcha(ip string) (*Captcha, error) {
	answer := randomCaptcha()
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	img, err := renderCaptcha(answer)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.captcha == nil {
		s.captcha = map[string]captchaEntry{}
	}
	now := time.Now()
	for k, v := range s.captcha {
		if now.After(v.exp) {
			delete(s.captcha, k)
		}
	}
	s.captcha[id] = captchaEntry{answer: strings.ToUpper(answer), exp: now.Add(captchaTTL), ip: ip}
	s.mu.Unlock()
	return &Captcha{ID: id, Image: "data:image/png;base64," + base64.StdEncoding.EncodeToString(img)}, nil
}

func (s *Service) CheckCaptcha(ip, id, value string) bool {
	id = strings.TrimSpace(id)
	value = strings.ToUpper(strings.TrimSpace(value))
	if id == "" || value == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.captcha == nil {
		return false
	}
	ent, ok := s.captcha[id]
	delete(s.captcha, id)
	if !ok || time.Now().After(ent.exp) || ent.ip != ip {
		return false
	}
	return ent.answer == value
}

func randomCaptcha() string {
	b := make([]byte, captchaLen)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(captchaChars))))
		if err != nil {
			b[i] = captchaChars[i%len(captchaChars)]
			continue
		}
		b[i] = captchaChars[n.Int64()]
	}
	return string(b)
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func renderCaptcha(text string) ([]byte, error) {
	const w, h = 168, 52
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{R: 248, G: 250, B: 252, A: 255}}, image.Point{}, draw.Src)
	for i := 0; i < 80; i++ {
		x := intn(w)
		y := intn(h)
		img.Set(x, y, color.RGBA{R: 160, G: 170, B: 190, A: 255})
	}
	for i, c := range text {
		col := color.RGBA{R: 30, G: 41, B: 59, A: 255}
		drawGlyph(img, 14+i*30+intn(4), 10+intn(8), c, col)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawGlyph(img *image.RGBA, x, y int, r rune, col color.RGBA) {
	bits := glyph(r)
	for row := 0; row < 7; row++ {
		for coln := 0; coln < 5; coln++ {
			if bits[row]&(1<<uint(4-coln)) == 0 {
				continue
			}
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 4; dx++ {
					img.Set(x+coln*4+dx, y+row*4+dy, col)
				}
			}
		}
	}
}

func glyph(r rune) [7]byte {
	switch r {
	case 'A':
		return [7]byte{0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11}
	case 'B':
		return [7]byte{0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E}
	case 'C':
		return [7]byte{0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E}
	case 'D':
		return [7]byte{0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E}
	case 'E':
		return [7]byte{0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F}
	case 'F':
		return [7]byte{0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10}
	case 'G':
		return [7]byte{0x0E, 0x11, 0x10, 0x17, 0x11, 0x11, 0x0E}
	case 'H':
		return [7]byte{0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11}
	case 'J':
		return [7]byte{0x01, 0x01, 0x01, 0x01, 0x11, 0x11, 0x0E}
	case 'K':
		return [7]byte{0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11}
	case 'L':
		return [7]byte{0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F}
	case 'M':
		return [7]byte{0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11}
	case 'N':
		return [7]byte{0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11}
	case 'P':
		return [7]byte{0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10}
	case 'Q':
		return [7]byte{0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D}
	case 'R':
		return [7]byte{0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11}
	case 'S':
		return [7]byte{0x0E, 0x11, 0x10, 0x0E, 0x01, 0x11, 0x0E}
	case 'T':
		return [7]byte{0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04}
	case 'U':
		return [7]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E}
	case 'V':
		return [7]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04}
	case 'W':
		return [7]byte{0x11, 0x11, 0x11, 0x15, 0x15, 0x1B, 0x11}
	case 'X':
		return [7]byte{0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11}
	case 'Y':
		return [7]byte{0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04}
	case 'Z':
		return [7]byte{0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F}
	case '2':
		return [7]byte{0x0E, 0x11, 0x01, 0x06, 0x08, 0x10, 0x1F}
	case '3':
		return [7]byte{0x0E, 0x11, 0x01, 0x06, 0x01, 0x11, 0x0E}
	case '4':
		return [7]byte{0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02}
	case '5':
		return [7]byte{0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E}
	case '6':
		return [7]byte{0x0E, 0x10, 0x10, 0x1E, 0x11, 0x11, 0x0E}
	case '7':
		return [7]byte{0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08}
	case '8':
		return [7]byte{0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E}
	case '9':
		return [7]byte{0x0E, 0x11, 0x11, 0x0F, 0x01, 0x01, 0x0E}
	default:
		return [7]byte{0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E}
	}
}

func intn(n int) int {
	if n <= 1 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}
