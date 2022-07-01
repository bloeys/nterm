package main

import (
	"github.com/bloeys/gglm/gglm"
	"github.com/bloeys/nmage/engine"
	"github.com/bloeys/nmage/input"
	"github.com/bloeys/nmage/renderer/rend3dgl"
	nmageimgui "github.com/bloeys/nmage/ui/imgui"
	"github.com/bloeys/nterm/glyphs"
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
}

//nMage TODO:
//	* Assert that engine is inited
//	* Create VAO struct independent from VBO to support multi-VBO use cases (e.g. instancing)
//	* Move SetAttribute away from material struct
//	* Fix FPS counter
//	* Allow texture loading without cache
//	* Reduce/remove Game interface

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

		FontSize: 32,
	}

	p.win.EventCallbacks = append(p.win.EventCallbacks, func(e sdl.Event) {
		switch e := e.(type) {
		case *sdl.WindowEvent:
			if e.Event == sdl.WINDOWEVENT_SIZE_CHANGED {
				p.handleWindowResize()
			}
		}
	})

	engine.Run(p)
}

func (p *program) Init() {

	dpi, _, _, err := sdl.GetDisplayDPI(0)
	if err != nil {
		panic("Failed to get display DPI. Err: " + err.Error())
	}
	println("DPI:", dpi)

	w, h := p.win.SDLWin.GetSize()
	p.GlyphRend, err = glyphs.NewGlyphRend("./res/fonts/Consolas.ttf", &truetype.Options{Size: float64(p.FontSize), DPI: p.Dpi}, w, h)
	if err != nil {
		panic("Failed to create atlas from font file. Err: " + err.Error())
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

	if input.KeyClicked(sdl.K_SPACE) {
		p.handleWindowResize()
	}

	fontSizeChanged := false
	if input.KeyClicked(sdl.K_KP_PLUS) {
		p.FontSize += 2
		fontSizeChanged = true
	} else if input.KeyClicked(sdl.K_KP_MINUS) {
		p.FontSize -= 2
		fontSizeChanged = true
	}

	if fontSizeChanged {
		p.GlyphRend.SetFace(&truetype.Options{Size: float64(p.FontSize), DPI: p.Dpi})
	}
}

func (p *program) Render() {

	defer p.GlyphRend.Draw()

	textColor := gglm.NewVec4(1, 1, 1, 1)
	p.GlyphRend.DrawTextOpenGL("Hello there, friend.", gglm.NewVec3(0, 0.9, 0), textColor)
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

	println("Old size:", p.GlyphRend.ScreenWidth, ",", p.GlyphRend.ScreenHeight)
	w, h := p.win.SDLWin.GetSize()
	p.GlyphRend.SetScreenSize(w, h)
	println("New size:", w, ",", h, "\n")
}
