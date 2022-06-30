package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"unicode"

	"github.com/bloeys/nmage/engine"
	"github.com/bloeys/nmage/input"
	"github.com/bloeys/nmage/renderer/rend3dgl"
	nmageimgui "github.com/bloeys/nmage/ui/imgui"
	"github.com/golang/freetype/truetype"
	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

func main() {

	win, err := engine.CreateOpenGLWindowCentered("nTerm", 1280, 720, engine.WindowFlags_ALLOW_HIGHDPI|engine.WindowFlags_RESIZABLE, rend3dgl.NewRend3DGL())
	if err != nil {
		panic("Failed to create window. Err: " + err.Error())
	}

	engine.SetVSync(true)

	p := &program{
		shouldRun: true,
		win:       win,
		imguiInfo: nmageimgui.NewImGUI(),
	}

	engine.Run(p)
}

var _ engine.Game = &program{}

type program struct {
	shouldRun bool
	win       *engine.Window
	imguiInfo nmageimgui.ImguiInfo
}

func (p *program) Init() {

	fBytes, err := os.ReadFile("./res/fonts/Consolas.ttf")
	if err != nil {
		panic("Failed to read font. Err: " + err.Error())
	}

	f, err := truetype.Parse(fBytes)
	if err != nil {
		panic("Failed to parse font. Err: " + err.Error())
	}

	size := 40
	face := truetype.NewFace(f, &truetype.Options{Size: float64(size), DPI: 72})
	imgFromText("Hello there my friend", size, face, "./text.png")
	fmt.Println(string(getGlyphs(loadFontRanges(f))))
}

func imgFromText(text string, textSize int, face font.Face, file string) {

	//Create a white image
	rgbaDest := image.NewRGBA(image.Rect(0, 0, 640, 480))
	draw.Draw(rgbaDest, rgbaDest.Bounds(), image.White, image.Point{}, draw.Src)

	//Draw black text on image
	drawer := &font.Drawer{
		Dst:  rgbaDest,
		Src:  image.Black,
		Face: face,
	}

	drawer.Dot = fixed.P(0, textSize)
	drawer.DrawString(text)

	// Save that RGBA image to disk.
	outFile, err := os.Create(file)
	if err != nil {
		panic(err)
	}
	defer outFile.Close()

	err = png.Encode(outFile, rgbaDest)
	if err != nil {
		panic(err)
	}
}

func (p *program) Start() {

}

func (p *program) FrameStart() {

}

func (p *program) Update() {

	if input.IsQuitClicked() || input.KeyClicked(sdl.K_ESCAPE) {
		p.shouldRun = false
	}
}

func (p *program) Render() {

}

func (p *program) FrameEnd() {

}

func (g *program) GetWindow() *engine.Window {
	return g.win
}

func (g *program) GetImGUI() nmageimgui.ImguiInfo {
	return g.imguiInfo
}

func (p *program) ShouldRun() bool {
	return p.shouldRun
}

func (p *program) Deinit() {

}

//loadFontRanges returns a list of ranges, each range is: [i][0]<=range<[i][1]
func loadFontRanges(f *truetype.Font) (ret [][2]rune) {
	rr := [2]rune{-1, -1}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if privateUseArea(r) {
			continue
		}
		if f.Index(r) == 0 {
			continue
		}
		if rr[1] == r {
			rr[1] = r + 1
			continue
		}
		if rr[0] != -1 {
			ret = append(ret, rr)
		}
		rr = [2]rune{r, r + 1}
	}
	if rr[0] != -1 {
		ret = append(ret, rr)
	}
	return ret
}

func privateUseArea(r rune) bool {
	return 0xe000 <= r && r <= 0xf8ff ||
		0xf0000 <= r && r <= 0xffffd ||
		0x100000 <= r && r <= 0x10fffd
}

//getGlyphs takes ranges of runes and produces an array of all the runes in these ranges
func getGlyphs(ranges [][2]rune) []rune {

	out := make([]rune, 0)
	for _, rr := range ranges {

		temp := make([]rune, 0, rr[1]-rr[0])
		for r := rr[0]; r < rr[1]; r++ {
			temp = append(temp, r)
		}

		out = append(out, temp...)
	}

	return out
}
