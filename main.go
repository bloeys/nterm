package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"
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

//nMage TODO:
//* Assert that engine is inited
//* Create VAO struct independent from VBO to support multi-VBO use cases (e.g. instancing)
//* Move SetAttribute away from material struct

func main() {

	err := engine.Init()
	if err != nil {
		panic("Failed to init engine. Err: " + err.Error())
	}

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

var fontPointSize uint = 32
var atlas *FontTexAtlas
var atlasTex assets.Texture
var glyphRend *GlyphRend

func (p *program) Init() {

	var err error
	atlas, err = createAtlasFromFontFile("./res/fonts/Consolas.ttf", &truetype.Options{Size: float64(fontPointSize), DPI: 72})
	if err != nil {
		panic("Failed to create atlas from font file. Err: " + err.Error())
	}

	glyphRend = NewGlyphRend()
}

func createAtlasFromFontFile(fontFile string, faceOptions *truetype.Options) (*FontTexAtlas, error) {

	fBytes, err := os.ReadFile(fontFile)
	if err != nil {
		return nil, err
	}

	f, err := truetype.Parse(fBytes)
	if err != nil {
		return nil, err
	}

	face := truetype.NewFace(f, faceOptions)
	atlas := genTextureAtlasFromFace(f, face, uint(faceOptions.Size))

	return atlas, nil
}

func genTextureAtlasFromFace(f *truetype.Font, face font.Face, pointSize uint) *FontTexAtlas {

	const maxAtlasSize = 8192

	glyphs := getGlyphsFromRanges(getGlyphRanges(f))

	assert.T(len(glyphs) > 0, "no glyphs")

	//Choose atlas size
	atlasSizeX := 512
	atlasSizeY := 512

	_, charWidthFixed, _ := face.GlyphBounds(glyphs[0])
	charWidth := charWidthFixed.Floor()
	lineHeight := face.Metrics().Height.Floor()

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
		advanceRounded := gAdvanceFixed.Floor()
		ascent := -gBounds.Min.Y

		heightRounded := (ascent + descent).Floor()

		atlas.Glyphs[g] = FontTexAtlasGlyph{
			U: float32(drawer.Dot.X.Floor()) / float32(atlasSizeX),
			V: (float32(atlasSizeY-(drawer.Dot.Y+descent).Floor()) / float32(atlasSizeY)),

			SizeU: float32(advanceRounded) / float32(atlasSizeX),
			SizeV: (float32(heightRounded) / float32(atlasSizeY)),

			Ascent:  float32(ascent.Floor()),
			Descent: float32(descent.Floor()),
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

var frameStartTime time.Time
var frameTime time.Duration = 16 * time.Millisecond

func (p *program) FrameStart() {
	frameStartTime = time.Now()
}

func (p *program) Update() {

	if input.IsQuitClicked() || input.KeyClicked(sdl.K_ESCAPE) {
		p.shouldRun = false
	}

	if input.KeyClicked(sdl.K_KP_PLUS) {

		fontPointSize += 2

		var err error
		atlas, err = createAtlasFromFontFile("./res/fonts/Consolas.ttf", &truetype.Options{Size: float64(fontPointSize), DPI: 72})
		if err != nil {
			panic("Failed to create atlas from font file. Err: " + err.Error())
		}

		//Delete old opengl texture
		gl.DeleteTextures(1, &atlasTex.TexID)
		delete(assets.TexturePaths, "./res/fonts/Consolas.ttf")
		delete(assets.Textures, atlasTex.TexID)

		atlasTex.TexID = 0
	}

	if input.KeyClicked(sdl.K_KP_MINUS) {

		fontPointSize -= 2

		var err error
		atlas, err = createAtlasFromFontFile("./res/fonts/Consolas.ttf", &truetype.Options{Size: float64(fontPointSize), DPI: 72})
		if err != nil {
			panic("Failed to create atlas from font file. Err: " + err.Error())
		}

		//Delete old opengl texture
		gl.DeleteTextures(1, &atlasTex.TexID)
		delete(assets.TexturePaths, "./res/fonts/Consolas.ttf")
		delete(assets.Textures, atlasTex.TexID)
		atlasTex.TexID = 0
	}
}

var gg = sync.Once{}

func (p *program) Render() {

	w, h := p.win.SDLWin.GetSize()

	//Draw FPS
	var fps float32
	if frameTime.Milliseconds() > 0 {
		fps = 1 / float32(frameTime.Milliseconds()) * 1000
	}
	startFromTop := float32(h) - float32(atlas.LineHeight)
	glyphRend.drawTextOpenGL(atlas, fmt.Sprintf("FPS=%f", fps), gglm.NewVec3(float32(w)*0.7, startFromTop, 0), w, h)

	//Draw point and texture sizes
	startFromTop -= float32(atlas.LineHeight)
	glyphRend.drawTextOpenGL(atlas, fmt.Sprintf("Point size=%d", fontPointSize), gglm.NewVec3(float32(w)*0.7, startFromTop, 0), w, h)

	startFromTop -= float32(atlas.LineHeight)
	glyphRend.drawTextOpenGL(atlas, fmt.Sprintf("Texture size=%d*%d", atlasTex.Width, atlasTex.Height), gglm.NewVec3(float32(w)*0.7, startFromTop, 0), w, h)

	//Draw all other
	count := 1000
	startFromBot := float32(atlas.LineHeight)
	for i := 0; i < count; i++ {
		glyphRend.drawTextOpenGL(atlas, "Hello friend, how are you?\n", gglm.NewVec3(0, startFromBot, 0), w, h)
		startFromBot += float32(atlas.LineHeight) * 2
	}

	gg.Do(func() {
		fmt.Printf("Drawn chars: %d\n", glyphRend.GlyphCount)
	})

	glyphRend.Draw()
}

func (p *program) FrameEnd() {
	frameTime = time.Since(frameStartTime)
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

var glyphMat *materials.Material

func (gr *GlyphRend) drawTextOpenGL(atlas *FontTexAtlas, text string, startPos *gglm.Vec3, winWidth, winHeight int32) {

	// if glyphMesh == nil {
	// 	glyphMesh = &meshes.Mesh{
	// 		Name: "glypQuad",

	// 		//VertPos, UV, Color
	// 		Buf: buffers.NewBuffer(
	// 			buffers.Element{ElementType: buffers.DataTypeVec3},
	// 			buffers.Element{ElementType: buffers.DataTypeVec2},
	// 			buffers.Element{ElementType: buffers.DataTypeVec4},
	// 		),
	// 	}

	// 	glyphMesh.Buf.SetData([]float32{
	// 		-0.5, -0.5, 0,
	// 		0, 0,
	// 		1, 1, 1, 1,

	// 		0.5, -0.5, 0,
	// 		1, 0,
	// 		1, 1, 1, 1,

	// 		-0.5, 0.5, 0,
	// 		0, 1,
	// 		1, 1, 1, 1,

	// 		0.5, 0.5, 0,
	// 		1, 1,
	// 		1, 1, 1, 1,
	// 	})

	// 	glyphMesh.Buf.SetIndexBufData([]uint32{
	// 		0, 1, 2,
	// 		1, 3, 2,
	// 	})
	// }

	if glyphMat == nil {
		glyphMat = materials.NewMaterial("glyphMat", "./res/shaders/glyph")
	}

	if atlasTex.TexID == 0 {

		var err error
		atlasTex, err = assets.LoadPNGTexture("./atlas.png")
		if err != nil {
			panic(err.Error())
		}

		gl.BindTexture(gl.TEXTURE_2D, atlasTex.TexID)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
		gl.BindTexture(gl.TEXTURE_2D, 0)
	}

	//Prepare to draw
	// glyphMesh.Buf.Bind()

	//The projection matrix fits the screen size. This is needed so we can size and position characters correctly.
	projMtx := gglm.Ortho(0, float32(winWidth), float32(winHeight), 0, 0.1, 10)
	viewMtx := gglm.LookAt(gglm.NewVec3(0, 0, -1), gglm.NewVec3(0, 0, 0), gglm.NewVec3(0, 1, 0))
	projViewMtx := projMtx.Clone().Mul(viewMtx)

	glyphMat.DiffuseTex = atlasTex.TexID
	glyphMat.SetUnifMat4("projViewMat", &projViewMtx.Mat4)
	glyphMat.Bind()

	//Prepass to pre-allocate the buffer
	rs := []rune(text)
	const floatsPerGlyph = 18

	rCol := rand.Float32()
	gCol := rand.Float32()
	bCol := rand.Float32()

	pos := startPos.Clone()
	instancedData := make([]float32, 0, len(rs)*floatsPerGlyph) //This a larger approximation than needed because we don't count spaces etc
	for i := 0; i < len(rs); i++ {

		r := rs[i]
		g := atlas.Glyphs[r]
		if r == '\n' {
			startPos.SetY(startPos.Y() - float32(atlas.LineHeight))
			pos = startPos.Clone()
			continue
		}
		gr.GlyphCount++

		height := float32(g.Ascent + g.Descent)
		scale := gglm.NewVec3(g.Advance, height, 1)

		//See: https://developer.apple.com/library/archive/documentation/TextFonts/Conceptual/CocoaTextArchitecture/Art/glyph_metrics_2x.png
		//Quads are drawn from the center and so that's our baseline. But chars shouldn't be centered, they should follow ascent/decent/advance.
		//To make them do that vertically, we raise them above the baseline (y+height/2), then since they are sitting on top of the baseline we can simply
		//move them down by the decent amount to put them in the correct vertical position.
		//
		//Horizontally the character should be drawn from the left edge not the center, so we just move it forward by advance/2
		drawPos := *pos
		drawPos.SetX(drawPos.X() + g.Advance*0.5)
		drawPos.SetY(drawPos.Y() + height*0.5 - g.Descent)

		instancedData = append(instancedData, []float32{
			g.U, g.V,
			g.U + g.SizeU, g.V,
			g.U, g.V + g.SizeV,
			g.U + g.SizeU, g.V + g.SizeV,

			rCol, gCol, bCol, 1, //Color
			drawPos.X(), drawPos.Y(), drawPos.Z(), //Model pos
			scale.X(), scale.Y(), scale.Z(), //Model scale
		}...)

		pos.SetX(pos.X() + g.Advance)
	}

	gr.GlyphVBO = append(gr.GlyphVBO, instancedData...)
}

type GlyphRend struct {
	InstancedBuf buffers.Buffer
	GlyphMesh    *meshes.Mesh
	GlyphCount   int32
	GlyphVBO     []float32
}

func (gr *GlyphRend) Draw() {

	gr.InstancedBuf.SetData(gr.GlyphVBO)

	gr.InstancedBuf.Bind()
	glyphMat.Bind()
	//  gl.DrawElements(gl.TRIANGLES, mesh.Buf.IndexBufCount, gl.UNSIGNED_INT, gl.PtrOffset(0))
	gl.DrawElementsInstanced(gl.TRIANGLES, gr.GlyphMesh.Buf.IndexBufCount, gl.UNSIGNED_INT, gl.PtrOffset(0), gr.GlyphCount)
	gr.InstancedBuf.UnBind()

	gr.GlyphCount = 0
	gr.GlyphVBO = []float32{}
}

func NewGlyphRend() *GlyphRend {

	//Create glyph mesh
	glyphMesh := &meshes.Mesh{
		Name: "glypQuad",

		//VertPos, UV, Color; Instanced attributes are stored separately
		Buf: buffers.NewBuffer(
			buffers.Element{ElementType: buffers.DataTypeVec3},
		),
	}

	glyphMesh.Buf.SetData([]float32{
		-0.5, -0.5, 0,
		0.5, -0.5, 0,
		-0.5, 0.5, 0,
		0.5, 0.5, 0,
	})

	glyphMesh.Buf.SetIndexBufData([]uint32{
		0, 1, 2,
		1, 3, 2,
	})

	(*materials.Material).SetAttribute(nil, glyphMesh.Buf)

	//Create instanced buf and set its instanced attributes.
	//Multiple VBOs under one VAO, one VBO for vertex data, and one VBO for instanced data.
	instancedBuf := buffers.Buffer{
		VAOID: glyphMesh.Buf.VAOID,
	}
	instancedBuf.SetLayout(
		buffers.Element{ElementType: buffers.DataTypeVec2}, //UVST0
		buffers.Element{ElementType: buffers.DataTypeVec2}, //UVST1
		buffers.Element{ElementType: buffers.DataTypeVec2}, //UVST2
		buffers.Element{ElementType: buffers.DataTypeVec2}, //UVST3

		buffers.Element{ElementType: buffers.DataTypeVec4}, //Color
		buffers.Element{ElementType: buffers.DataTypeVec3}, //ModelPos
		buffers.Element{ElementType: buffers.DataTypeVec3}, //ModelScale
	)

	gl.GenBuffers(1, &instancedBuf.BufID)
	if instancedBuf.BufID == 0 {
		panic("Failed to create openGL buffer")
	}

	instancedBuf.Bind()
	gl.BindBuffer(gl.ARRAY_BUFFER, instancedBuf.BufID)
	layout := instancedBuf.GetLayout()

	//4 UV values
	uvEle := layout[0]
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(1, uvEle.ElementType.CompCount(), uvEle.ElementType.GLType(), false, instancedBuf.Stride, gl.PtrOffset(uvEle.Offset))
	gl.VertexAttribDivisor(1, 1)

	uvEle = layout[1]
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointer(2, uvEle.ElementType.CompCount(), uvEle.ElementType.GLType(), false, instancedBuf.Stride, gl.PtrOffset(uvEle.Offset))
	gl.VertexAttribDivisor(2, 1)

	uvEle = layout[2]
	gl.EnableVertexAttribArray(3)
	gl.VertexAttribPointer(3, uvEle.ElementType.CompCount(), uvEle.ElementType.GLType(), false, instancedBuf.Stride, gl.PtrOffset(uvEle.Offset))
	gl.VertexAttribDivisor(3, 1)

	uvEle = layout[3]
	gl.EnableVertexAttribArray(4)
	gl.VertexAttribPointer(4, uvEle.ElementType.CompCount(), uvEle.ElementType.GLType(), false, instancedBuf.Stride, gl.PtrOffset(uvEle.Offset))
	gl.VertexAttribDivisor(4, 1)

	//Rest of instanced attributes
	colorEle := layout[4]
	gl.EnableVertexAttribArray(5)
	gl.VertexAttribPointer(5, colorEle.ElementType.CompCount(), colorEle.ElementType.GLType(), false, instancedBuf.Stride, gl.PtrOffset(colorEle.Offset))
	gl.VertexAttribDivisor(5, 1)

	posEle := layout[5]
	gl.EnableVertexAttribArray(6)
	gl.VertexAttribPointer(6, posEle.ElementType.CompCount(), posEle.ElementType.GLType(), false, instancedBuf.Stride, gl.PtrOffset(posEle.Offset))
	gl.VertexAttribDivisor(6, 1)

	scaleEle := layout[6]
	gl.EnableVertexAttribArray(7)
	gl.VertexAttribPointer(7, scaleEle.ElementType.CompCount(), scaleEle.ElementType.GLType(), false, instancedBuf.Stride, gl.PtrOffset(scaleEle.Offset))
	gl.VertexAttribDivisor(7, 1)

	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	instancedBuf.UnBind()

	return &GlyphRend{
		GlyphMesh:    glyphMesh,
		InstancedBuf: instancedBuf,
		GlyphCount:   0,
		GlyphVBO:     make([]float32, 0),
	}
}
