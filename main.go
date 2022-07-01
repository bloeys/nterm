package main

import (
	"fmt"
	"sync"
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
}

//nMage TODO:
//	* Assert that engine is inited
//	* Create VAO struct independent from VBO to support multi-VBO use cases (e.g. instancing)
//	* Move SetAttribute away from material struct
//	* Fix FPS counter
//	* Allow texture loading without cache

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

var fontPointSize uint = 32
var glyphRend *glyphs.GlyphRend

func (p *program) Init() {

	var err error
	glyphRend, err = glyphs.NewGlyphRend("./res/fonts/Consolas.ttf", &truetype.Options{Size: float64(fontPointSize), DPI: 72})
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
		fontPointSize += 2
		fontSizeChanged = true
	} else if input.KeyClicked(sdl.K_KP_MINUS) {
		fontPointSize -= 2
		fontSizeChanged = true
	}

	if fontSizeChanged {
		glyphRend.SetFace(&truetype.Options{Size: float64(fontPointSize), DPI: 72})
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
	startFromTop := float32(h) - float32(glyphRend.Atlas.LineHeight)
	glyphRend.DrawTextOpenGL(fmt.Sprintf("FPS=%f", fps), gglm.NewVec3(float32(w)*0.7, startFromTop, 0), w, h)

	//Draw point and texture sizes
	startFromTop -= float32(glyphRend.Atlas.LineHeight)
	glyphRend.DrawTextOpenGL(fmt.Sprintf("Point size=%d", fontPointSize), gglm.NewVec3(float32(w)*0.7, startFromTop, 0), w, h)

	startFromTop -= float32(glyphRend.Atlas.LineHeight)
	glyphRend.DrawTextOpenGL(fmt.Sprintf("Texture size=%d*%d", glyphRend.Atlas.Img.Rect.Max.X, glyphRend.Atlas.Img.Rect.Max.Y), gglm.NewVec3(float32(w)*0.7, startFromTop, 0), w, h)

	//Draw all other
	count := 1000
	startFromBot := float32(glyphRend.Atlas.LineHeight)
	for i := 0; i < count; i++ {
		glyphRend.DrawTextOpenGL("Hello friend, how are you?\n", gglm.NewVec3(0, startFromBot, 0), w, h)
		startFromBot += float32(glyphRend.Atlas.LineHeight) * 2
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
