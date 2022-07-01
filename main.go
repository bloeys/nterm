package main

import (
	"fmt"
	"time"

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

	w, h := p.win.SDLWin.GetSize()

	var err error
	p.GlyphRend, err = glyphs.NewGlyphRend("./res/fonts/Consolas.ttf", &truetype.Options{Size: float64(p.FontSize), DPI: 72}, w, h)
	if err != nil {
		panic("Failed to create atlas from font file. Err: " + err.Error())
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

	fontSizeChanged := false
	if input.KeyClicked(sdl.K_KP_PLUS) {
		p.FontSize += 2
		fontSizeChanged = true
	} else if input.KeyClicked(sdl.K_KP_MINUS) {
		p.FontSize -= 2
		fontSizeChanged = true
	}

	if fontSizeChanged {
		p.GlyphRend.SetFace(&truetype.Options{Size: float64(p.FontSize), DPI: 72})
	}
}

func (p *program) Render() {

	w, h := p.win.SDLWin.GetSize()
	textColor := gglm.NewVec4(1, 1, 1, 1)

	//Draw FPS
	var fps float32
	if frameTime.Milliseconds() > 0 {
		fps = 1 / float32(frameTime.Milliseconds()) * 1000
	}
	startFromTop := float32(h) - float32(p.GlyphRend.Atlas.LineHeight)
	p.GlyphRend.DrawTextOpenGL(fmt.Sprintf("FPS=%f", fps), gglm.NewVec3(float32(w)*0.7, startFromTop, 0), textColor)

	//Draw point and texture sizes
	startFromTop -= float32(p.GlyphRend.Atlas.LineHeight)
	p.GlyphRend.DrawTextOpenGL(fmt.Sprintf("Point size=%d", p.FontSize), gglm.NewVec3(float32(w)*0.7, startFromTop, 0), textColor)

	startFromTop -= float32(p.GlyphRend.Atlas.LineHeight)
	p.GlyphRend.DrawTextOpenGL(fmt.Sprintf("Texture size=%d*%d", p.GlyphRend.Atlas.Img.Rect.Max.X, p.GlyphRend.Atlas.Img.Rect.Max.Y), gglm.NewVec3(float32(w)*0.7, startFromTop, 0), textColor)

	//Draw all other
	count := 1000
	startFromBot := float32(p.GlyphRend.Atlas.LineHeight)
	for i := 0; i < count; i++ {
		p.GlyphRend.DrawTextOpenGL("Hello friend, how are you?\n", gglm.NewVec3(0, startFromBot, 0), textColor)
		startFromBot += float32(p.GlyphRend.Atlas.LineHeight) * 2
	}

	p.GlyphRend.Draw()
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

func (p *program) handleWindowResize() {
	w, h := p.win.SDLWin.GetSize()
	p.GlyphRend.SetScreenSize(w, h)
}
