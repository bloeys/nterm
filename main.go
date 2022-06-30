package main

import (
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"unicode"

	"github.com/bloeys/nmage/engine"
	"github.com/bloeys/nmage/input"
	"github.com/bloeys/nmage/renderer/rend3dgl"
	nmageimgui "github.com/bloeys/nmage/ui/imgui"
	"github.com/bloeys/nterm/assert"
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

	pointSize := 40
	face := truetype.NewFace(f, &truetype.Options{Size: float64(pointSize), DPI: 72})
	genTextureAtlas(f, face, pointSize)
}

type FontTexAtlas struct {
	Img    *image.RGBA
	Glyphs map[rune]FontTexAtlasGlyph
}

type FontTexAtlasGlyph struct {
	U float32
	V float32
}

func genTextureAtlas(f *truetype.Font, face font.Face, textSize int) *FontTexAtlas {

	const maxAtlasSize = 8192

	glyphs := getGlyphsFromRanges(getGlyphRanges(f))

	assert.T(len(glyphs) > 0, "no glyphs")

	//Choose atlas size
	atlasSizeX := 512
	atlasSizeY := 512

	_, charWidthFixed, _ := face.GlyphBounds(glyphs[0])
	charWidth := charWidthFixed.Round()
	lineHeight := face.Metrics().Height.Round()

	maxLinesInAtlas := atlasSizeY/lineHeight - 1
	charsPerLine := atlasSizeX / charWidth
	linesNeeded := int(math.Ceil(float64(len(glyphs)) / float64(charsPerLine)))

	for linesNeeded > maxLinesInAtlas {

		atlasSizeX *= 2
		atlasSizeY *= 2

		maxLinesInAtlas = atlasSizeY/lineHeight - 1

		charsPerLine = atlasSizeX / charWidth
		linesNeeded = int(math.Ceil(float64(len(glyphs)) / float64(charsPerLine)))
	}
	assert.T(atlasSizeX <= maxAtlasSize, "Atlas size went beyond maximum")

	//Create atlas
	atlas := &FontTexAtlas{
		Img:    image.NewRGBA(image.Rect(0, 0, atlasSizeX, atlasSizeY)),
		Glyphs: make(map[rune]FontTexAtlasGlyph, len(glyphs)),
	}

	//Clear img to white
	draw.Draw(atlas.Img, atlas.Img.Bounds(), image.White, image.Point{}, draw.Src)

	drawer := &font.Drawer{
		Dst:  atlas.Img,
		Src:  image.Black,
		Face: face,
	}

	//Put glyphs on atlas
	charsOnLine := 0
	lineDx := fixed.P(0, lineHeight)
	drawer.Dot = fixed.P(0, lineHeight)
	for _, g := range glyphs {

		atlas.Glyphs[g] = FontTexAtlasGlyph{
			U: float32(drawer.Dot.X.Floor()) / float32(atlasSizeX),
			V: (float32(atlasSizeY-drawer.Dot.Y.Floor()) / float32(atlasSizeY)),
		}
		drawer.DrawString(string(g))

		charsOnLine++
		if charsOnLine == charsPerLine {

			charsOnLine = 0
			drawer.Dot.X = 0
			drawer.Dot = drawer.Dot.Add(lineDx)
		}
	}

	// fmt.Println(atlas.Glyphs)
	saveImgToDisk(atlas.Img, "atlas.png")
	return atlas
}

func saveImgToDisk(img image.Image, file string) {

	outFile, err := os.Create(file)
	if err != nil {
		panic(err)
	}
	defer outFile.Close()

	err = png.Encode(outFile, img)
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

//getGlyphRanges returns a list of ranges, each range is: [i][0]<=range<[i][1]
func getGlyphRanges(f *truetype.Font) (ret [][2]rune) {
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

//getGlyphsFromRanges takes ranges of runes and produces an array of all the runes in these ranges
func getGlyphsFromRanges(ranges [][2]rune) []rune {

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
