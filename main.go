package main

import (
	"fmt"
	"os"

	"github.com/bloeys/nmage/engine"
	"github.com/bloeys/nmage/input"
	"github.com/bloeys/nmage/renderer/rend3dgl"
	nmageimgui "github.com/bloeys/nmage/ui/imgui"
	"github.com/golang/freetype/truetype"
)

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

	face := truetype.NewFace(f, &truetype.Options{Size: 12, DPI: 72})
	fmt.Println(face.Metrics())
}

func (p *program) Start() {

}

func (p *program) FrameStart() {

}

func (p *program) Update() {

	if input.IsQuitClicked() {
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
