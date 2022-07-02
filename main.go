package main

import (
	"fmt"

	"github.com/bloeys/gglm/gglm"
	"github.com/bloeys/nmage/engine"
	"github.com/bloeys/nmage/input"
	"github.com/bloeys/nmage/materials"
	"github.com/bloeys/nmage/meshes"
	"github.com/bloeys/nmage/renderer/rend3dgl"
	nmageimgui "github.com/bloeys/nmage/ui/imgui"
	"github.com/bloeys/nterm/glyphs"
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/golang/freetype/truetype"
	"github.com/veandco/go-sdl2/sdl"
)

var _ engine.Game = &program{}

type program struct {
	shouldRun bool
	win       *engine.Window
	rend      *rend3dgl.Rend3DGL
	imguiInfo nmageimgui.ImguiInfo

	FontSize  uint32
	Dpi       float64
	GlyphRend *glyphs.GlyphRend

	gridMesh *meshes.Mesh
	gridMat  *materials.Material

	shouldDrawGrid bool
}

//nMage TODO:
//	* Assert that engine is inited
//	* Create VAO struct independent from VBO to support multi-VBO use cases (e.g. instancing)
//	* Move SetAttribute away from material struct
//	* Fix FPS counter
//	* Allow texture loading without cache
//	* Reduce/remove Game interface

const subPixelX = 64
const subPixelY = 64

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

		FontSize: 14,
	}

	p.win.EventCallbacks = append(p.win.EventCallbacks, func(e sdl.Event) {
		switch e := e.(type) {
		case *sdl.WindowEvent:
			if e.Event == sdl.WINDOWEVENT_SIZE_CHANGED {
				p.handleWindowResize()
			}
		}
	})

	//Don't flash white
	gl.ClearColor(0, 0, 0, 0)
	p.win.SDLWin.GLSwap()

	engine.Run(p)
}

func (p *program) Init() {

	dpi, _, _, err := sdl.GetDisplayDPI(0)
	if err != nil {
		panic("Failed to get display DPI. Err: " + err.Error())
	}
	fmt.Println("DPI:", dpi)

	w, h := p.win.SDLWin.GetSize()
	p.GlyphRend, err = glyphs.NewGlyphRend("./res/fonts/Consolas.ttf", &truetype.Options{Size: float64(p.FontSize), DPI: p.Dpi, SubPixelsX: subPixelX, SubPixelsY: subPixelY}, w, h)
	if err != nil {
		panic("Failed to create atlas from font file. Err: " + err.Error())
	}

	glyphs.SaveImgToPNG(p.GlyphRend.Atlas.Img, "./debug-atlas.png")
}

func (p *program) Start() {

	var err error
	p.gridMesh, err = meshes.NewMesh("grid", "./res/models/quad.obj", 0)
	if err != nil {
		panic(err.Error())
	}

	p.gridMat = materials.NewMaterial("grid", "./res/shaders/grid")
	p.gridMat.SetAttribute(p.gridMesh.Buf)
	p.handleWindowResize()
}

func (p *program) FrameStart() {
}

func (p *program) Update() {

	if input.IsQuitClicked() || input.KeyClicked(sdl.K_ESCAPE) {
		p.shouldRun = false
	}

	if input.KeyClicked(sdl.K_SPACE) {
		p.handleWindowResize()
	}

	//Font sizing
	oldFont := p.FontSize
	fontSizeChanged := false
	if input.KeyClicked(sdl.K_KP_PLUS) {
		p.FontSize += 2
		fontSizeChanged = true
	} else if input.KeyClicked(sdl.K_KP_MINUS) {
		p.FontSize -= 2
		fontSizeChanged = true
	}

	if fontSizeChanged {

		err := p.GlyphRend.SetFace(&truetype.Options{Size: float64(p.FontSize), DPI: p.Dpi, SubPixelsX: subPixelX, SubPixelsY: subPixelY})
		if err != nil {
			p.FontSize = oldFont
			println("Failed to update font face. Err: " + err.Error())
		} else {
			glyphs.SaveImgToPNG(p.GlyphRend.Atlas.Img, "./debug-atlas.png")
			println("New font size:", p.FontSize, "; New texture size:", p.GlyphRend.Atlas.Img.Rect.Max.X, "\n")
		}
	}

	//Move text
	var speed float32 = 1
	if input.KeyDown(sdl.K_RIGHT) {
		xOff += speed
	} else if input.KeyDown(sdl.K_LEFT) {
		xOff -= speed
	}

	if input.KeyDown(sdl.K_UP) {
		yOff += speed
	} else if input.KeyDown(sdl.K_DOWN) {
		yOff -= speed
	}

	//Grid
	if input.KeyClicked(sdl.K_SPACE) {
		p.shouldDrawGrid = !p.shouldDrawGrid
	}
}

var xOff float32 = 0
var yOff float32 = 0

func (p *program) Render() {

	defer p.GlyphRend.Draw()

	textColor := gglm.NewVec4(1, 1, 1, 1)
	// p.GlyphRend.DrawTextOpenGL("y", gglm.NewVec3(0+xOff, 0+yOff, 0), textColor)
	// p.GlyphRend.DrawTextOpenGL("A\np-+_; This is", gglm.NewVec3(0.3+xOff, 0.5+yOff, 0), textColor)
	// p.GlyphRend.DrawTextOpenGLAbs("Hello there, friend.\nABCDEFGHIJKLMNOPQRSTUVWXYZ", gglm.NewVec3(0, 0, 0), textColor)
	p.GlyphRend.DrawTextOpenGLAbs(" Hello there, friend|.\n ABCDEFGHIJKLMNOPQRSTUVWXYZ", gglm.NewVec3(xOff, float32(p.GlyphRend.Atlas.LineHeight)*2+yOff, 0), textColor)

	if p.shouldDrawGrid {
		p.drawGrid()
	}
}

func (p *program) drawGrid() {

	sizeX := float32(p.GlyphRend.ScreenWidth)
	sizeY := float32(p.GlyphRend.ScreenHeight)

	//columns
	adv := p.GlyphRend.Atlas.Glyphs['A'].Advance
	for i := int32(0); i < p.GlyphRend.ScreenWidth; i += int32(adv) {
		p.rend.Draw(p.gridMesh, gglm.NewTrMatId().Translate(gglm.NewVec3(float32(i)+0.5, sizeY/2, 0)).Scale(gglm.NewVec3(1, sizeY, 1)), p.gridMat)
	}

	//rows
	for i := int32(0); i < p.GlyphRend.ScreenHeight; i += int32(p.GlyphRend.Atlas.LineHeight) {
		p.rend.Draw(p.gridMesh, gglm.NewTrMatId().Translate(gglm.NewVec3(sizeX/2, float32(i), 0)).Scale(gglm.NewVec3(sizeX, 1, 1)), p.gridMat)
	}
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

func (p *program) handleWindowResize() {
	w, h := p.win.SDLWin.GetSize()
	p.GlyphRend.SetScreenSize(w, h)

	projMtx := gglm.Ortho(0, float32(w), float32(h), 0, 0.1, 20)
	viewMtx := gglm.LookAt(gglm.NewVec3(0, 0, -10), gglm.NewVec3(0, 0, 0), gglm.NewVec3(0, 1, 0))
	p.gridMat.SetUnifMat4("projViewMat", &projMtx.Mul(viewMtx).Mat4)
}
