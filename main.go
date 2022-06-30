package main

import (
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"unicode"

	"github.com/bloeys/gglm/gglm"
	"github.com/bloeys/nmage/assets"
	"github.com/bloeys/nmage/buffers"
	"github.com/bloeys/nmage/engine"
	"github.com/bloeys/nmage/input"
	"github.com/bloeys/nmage/materials"
	"github.com/bloeys/nmage/meshes"
	"github.com/bloeys/nmage/renderer/rend3dgl"
	nmageimgui "github.com/bloeys/nmage/ui/imgui"
	"github.com/bloeys/nterm/assert"
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/golang/freetype/truetype"
	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

var _ engine.Game = &program{}

type program struct {
	shouldRun bool
	win       *engine.Window
	rend      *rend3dgl.Rend3DGL
	imguiInfo nmageimgui.ImguiInfo
}

func main() {

	rend := rend3dgl.NewRend3DGL()
	win, err := engine.CreateOpenGLWindowCentered("nTerm", 1280, 720, engine.WindowFlags_ALLOW_HIGHDPI|engine.WindowFlags_RESIZABLE, rend)
	if err != nil {
		panic("Failed to create window. Err: " + err.Error())
	}

	engine.SetVSync(true)

	p := &program{
		shouldRun: true,
		win:       win,
		rend:      rend,
		imguiInfo: nmageimgui.NewImGUI(),
	}

	engine.Run(p)
}

type FontTexAtlas struct {
	Img        *image.RGBA
	Glyphs     map[rune]FontTexAtlasGlyph
	LineHeight int
}

type FontTexAtlasGlyph struct {
	U     float32
	V     float32
	SizeU float32
	SizeV float32

	Ascent  float32
	Descent float32
	Advance float32
}

var atlas *FontTexAtlas

func (p *program) Init() {

	fBytes, err := os.ReadFile("./res/fonts/Consolas.ttf")
	if err != nil {
		panic("Failed to read font. Err: " + err.Error())
	}

	f, err := truetype.Parse(fBytes)
	if err != nil {
		panic("Failed to parse font. Err: " + err.Error())
	}

	pointSize := 64
	face := truetype.NewFace(f, &truetype.Options{Size: float64(pointSize), DPI: 72})
	atlas = genTextureAtlas(f, face, pointSize)
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
		Img:        image.NewRGBA(image.Rect(0, 0, atlasSizeX, atlasSizeY)),
		Glyphs:     make(map[rune]FontTexAtlasGlyph, len(glyphs)),
		LineHeight: lineHeight,
	}

	//Clear background to black
	draw.Draw(atlas.Img, atlas.Img.Bounds(), image.Black, image.Point{}, draw.Src)

	drawer := &font.Drawer{
		Dst:  atlas.Img,
		Src:  image.White,
		Face: face,
	}

	//Put glyphs on atlas
	charsOnLine := 0
	lineDx := fixed.P(0, lineHeight)
	drawer.Dot = fixed.P(0, lineHeight)
	for _, g := range glyphs {

		gBounds, gAdvanceFixed, _ := face.GlyphBounds(g)

		descent := gBounds.Max.Y
		advanceRounded := gAdvanceFixed.Round()
		ascent := -gBounds.Min.Y

		heightRounded := (ascent + descent).Round()

		atlas.Glyphs[g] = FontTexAtlasGlyph{
			U: float32(drawer.Dot.X.Round()) / float32(atlasSizeX),
			V: (float32(atlasSizeY-(drawer.Dot.Y+descent).Round()) / float32(atlasSizeY)),

			SizeU: float32(advanceRounded) / float32(atlasSizeX),
			SizeV: (float32(heightRounded) / float32(atlasSizeY)),

			Ascent:  float32(ascent.Round()),
			Descent: float32(descent.Round()),
			Advance: float32(advanceRounded),
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
	p.drawTextOpenGL(atlas, "Hello friend.\nHow are you?", gglm.NewVec3(0, 100, 0))
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

var glyphMesh *meshes.Mesh
var glyphMat *materials.Material

func (p *program) drawTextOpenGL(atlas *FontTexAtlas, text string, startPos *gglm.Vec3) {

	if glyphMesh == nil {
		glyphMesh = &meshes.Mesh{
			Name: "glypQuad",

			//VertPos, UV, Color
			Buf: buffers.NewBuffer(
				buffers.Element{ElementType: buffers.DataTypeVec3},
				buffers.Element{ElementType: buffers.DataTypeVec2},
				buffers.Element{ElementType: buffers.DataTypeVec4},
			),
		}

		glyphMesh.Buf.SetData([]float32{
			-0.5, -0.5, 0,
			0, 0,
			1, 1, 1, 1,

			0.5, -0.5, 0,
			1, 0,
			1, 1, 1, 1,

			-0.5, 0.5, 0,
			0, 1,
			1, 1, 1, 1,

			0.5, 0.5, 0,
			1, 1,
			1, 1, 1, 1,
		})

		glyphMesh.Buf.SetIndexBufData([]uint32{
			0, 1, 2,
			1, 3, 2,
		})
	}

	if glyphMat == nil {
		glyphMat = materials.NewMaterial("glyphMat", "./res/shaders/glyph")
	}

	//Load texture
	atlasTex, err := assets.LoadPNGTexture("./atlas.png")
	if err != nil {
		panic(err.Error())
	}

	gl.BindTexture(gl.TEXTURE_2D, atlasTex.TexID)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.BindTexture(gl.TEXTURE_2D, 0)

	//Prepare to draw
	glyphMesh.Buf.Bind()

	//The projection matrix fits the screen size. This is needed so we can size and position characters correctly.
	w, h := p.win.SDLWin.GetSize()
	projMtx := gglm.Ortho(0, float32(w), float32(h), 0, 0.1, 10)
	viewMat := gglm.LookAt(gglm.NewVec3(0, 0, -1), gglm.NewVec3(0, 0, 0), gglm.NewVec3(0, 1, 0))

	glyphMat.DiffuseTex = atlasTex.TexID
	glyphMat.SetAttribute(glyphMesh.Buf)
	glyphMat.SetUnifMat4("projMat", &projMtx.Mat4)
	glyphMat.SetUnifMat4("viewMat", &viewMat.Mat4)
	glyphMat.Bind()

	//Draw
	pos := startPos.Clone()
	rs := []rune(text)
	for i := 0; i < len(rs); i++ {

		r := rs[i]
		g := atlas.Glyphs[r]
		if r == '\n' {
			startPos.SetY(startPos.Y() - float32(atlas.LineHeight))
			pos = startPos.Clone()
			continue
		}

		glyphMesh.Buf.SetData([]float32{
			-0.5, -0.5, 0,
			g.U, g.V,
			1, 1, 1, 1,

			0.5, -0.5, 0,
			g.U + g.SizeU, g.V,
			1, 1, 1, 1,

			-0.5, 0.5, 0,
			g.U, g.V + g.SizeV,
			1, 1, 1, 1,

			0.5, 0.5, 0,
			g.U + g.SizeU, g.V + g.SizeV,
			1, 1, 1, 1,
		})
		glyphMesh.Buf.Bind()

		height := float32(g.Ascent + g.Descent)
		scale := gglm.NewVec3(g.Advance, height, 1)

		//See: https://developer.apple.com/library/archive/documentation/TextFonts/Conceptual/CocoaTextArchitecture/Art/glyph_metrics_2x.png
		//Quads are drawn from the center and so that's our baseline. But chars shouldn't be centered, they should follow ascent/decent/advance.
		//To make them do that vertically, we raise them above the baseline (y+height/2), then since they are sitting on top of the baseline we can simply
		//move them down by the decent amount to put them in the correct vertical position.
		//
		//Horizontally the character should be drawn from the left edge not the center, so we just move it forward by advance/2
		drawPos := *pos
		drawPos.Add(gglm.NewVec3(g.Advance*0.5, height*0.5-g.Descent, 0))

		p.rend.Draw(glyphMesh, gglm.NewTrMatId().Translate(&drawPos).Scale(scale), glyphMat)
		pos.SetX(pos.X() + g.Advance)
	}
}
