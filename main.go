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
	GlyphSizeU float32
	GlyphSizeV float32
}

type FontTexAtlasGlyph struct {
	U float32
	V float32
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

	pointSize := 40
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
		GlyphSizeU: float32(charWidth) / float32(atlasSizeX),
		GlyphSizeV: float32(lineHeight) / float32(atlasSizeY),
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
	p.drawTextOpenGL(atlas, "Hello friend", gglm.NewVec3(-9, 0, 0))
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

func (p *program) drawTextOpenGL(atlas *FontTexAtlas, text string, pos *gglm.Vec3) {

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

	glyphMat.DiffuseTex = atlasTex.TexID
	glyphMat.SetAttribute(glyphMesh.Buf)

	projMtx := gglm.Ortho(-10, 10, 10, -10, 0.1, 10)
	viewMat := gglm.LookAt(gglm.NewVec3(0, 0, -1), gglm.NewVec3(0, 0, 0), gglm.NewVec3(0, 1, 0))

	glyphMat.SetUnifMat4("projMat", &projMtx.Mat4)
	glyphMat.SetUnifMat4("viewMat", &viewMat.Mat4)
	glyphMat.Bind()

	rs := []rune(text)
	tr := gglm.NewTrMatId()
	tr.Translate(pos)
	tr.Scale(gglm.NewVec3(1, 1, 1))
	for i := 0; i < len(rs); i++ {

		r := rs[i]
		g := atlas.Glyphs[r]
		if g.U < 0 {
			print(g.U)
		}

		glyphMesh.Buf.SetData([]float32{
			-0.5, -0.5, 0,
			g.U, g.V,
			1, 1, 1, 1,

			0.5, -0.5, 0,
			g.U + atlas.GlyphSizeU, g.V,
			1, 1, 1, 1,

			-0.5, 0.5, 0,
			g.U, g.V + atlas.GlyphSizeV,
			1, 1, 1, 1,

			0.5, 0.5, 0,
			g.U + atlas.GlyphSizeU, g.V + atlas.GlyphSizeV,
			1, 1, 1, 1,
		})
		glyphMesh.Buf.Bind()

		p.rend.Draw(glyphMesh, tr, glyphMat)

		tr.Translate(gglm.NewVec3(1, 0, 0))
	}

}
