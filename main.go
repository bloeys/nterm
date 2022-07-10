package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime/pprof"

	"github.com/bloeys/gglm/gglm"
	"github.com/bloeys/nmage/engine"
	"github.com/bloeys/nmage/input"
	"github.com/bloeys/nmage/materials"
	"github.com/bloeys/nmage/meshes"
	"github.com/bloeys/nmage/renderer/rend3dgl"
	"github.com/bloeys/nmage/timing"
	nmageimgui "github.com/bloeys/nmage/ui/imgui"
	"github.com/bloeys/nterm/glyphs"
	"github.com/golang/freetype/truetype"
	"github.com/inkyblackness/imgui-go/v4"
	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/image/font"
)

var _ engine.Game = &program{}

type program struct {
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

const subPixelX = 64
const subPixelY = 64
const hinting = font.HintingNone

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
		win:       win,
		rend:      rend,
		imguiInfo: nmageimgui.NewImGUI(),

		FontSize: 36,
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
	p.win.SDLWin.GLSwap()

	var pf, _ = os.Create("pprof.cpu")
	defer pf.Close()
	pprof.StartCPUProfile(pf)
	engine.Run(p, p.win, p.imguiInfo)
	pprof.StopCPUProfile()
}

func (p *program) Init() {

	dpi, _, _, err := sdl.GetDisplayDPI(0)
	if err != nil {
		panic("Failed to get display DPI. Err: " + err.Error())
	}
	fmt.Printf("DPI: %f, font size: %d\n", dpi, p.FontSize)

	w, h := p.win.SDLWin.GetSize()
	p.GlyphRend, err = glyphs.NewGlyphRend("./res/fonts/alm-fixed.ttf", &truetype.Options{Size: float64(p.FontSize), DPI: p.Dpi, SubPixelsX: subPixelX, SubPixelsY: subPixelY, Hinting: hinting}, w, h)
	if err != nil {
		panic("Failed to create atlas from font file. Err: " + err.Error())
	}

	glyphs.SaveImgToPNG(p.GlyphRend.Atlas.Img, "./debug-atlas.png")

	//Load resources
	p.gridMesh, err = meshes.NewMesh("grid", "./res/models/quad.obj", 0)
	if err != nil {
		panic(err.Error())
	}

	p.gridMat = materials.NewMaterial("grid", "./res/shaders/grid.glsl")
	p.handleWindowResize()
}

func (p *program) Update() {

	if input.IsQuitClicked() || input.KeyClicked(sdl.K_ESCAPE) {
		engine.Quit()
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

		err := p.GlyphRend.SetFace(&truetype.Options{Size: float64(p.FontSize), DPI: p.Dpi, SubPixelsX: subPixelX, SubPixelsY: subPixelY, Hinting: hinting})
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

	//UI
	imgui.InputText("", &textToShow)

	if imgui.Button("Print Runs") {
		runs := make([]glyphs.TextRun, 0, 20)
		p.GlyphRend.GetTextRuns(textToShow, &runs)
		for _, run := range runs {
			fmt.Printf("%s; runes: %#x\n\n", string(run.Runes), run.Runes)
		}
		fmt.Printf("----------------\n")
	}

	if imgui.Checkbox("Draw Bounds", &isDrawingBounds) {

		if isDrawingBounds {
			p.GlyphRend.GlyphMat.SetUnifInt32("drawBounds", 1)
		} else {
			p.GlyphRend.GlyphMat.SetUnifInt32("drawBounds", 0)
		}
	}

	imgui.Checkbox("Draw many", &drawManyLines)
	glyphs.PrintPositions = imgui.Button("Print positions")
}

var isDrawingBounds = false
var drawManyLines = false
var textToShow = " Hello there يا friend. اسمي عمر wow!"

var xOff float32 = 0
var yOff float32 = 0

var r = rand.Float32()
var g = rand.Float32()
var b = rand.Float32()

func (p *program) Render() {

	defer p.GlyphRend.Draw()

	if p.shouldDrawGrid {
		p.drawGrid()
	}

	const colorSpd = 0.005
	r += colorSpd
	if r > 1 {
		r = 0
	}

	g += colorSpd
	if g > 1 {
		g = 0
	}

	b += colorSpd
	if b > 1 {
		b = 0
	}

	str := textToShow
	charCount := len([]rune(str))
	fps := int(timing.GetAvgFPS())
	textColor := gglm.NewVec4(r, g, b, 1)
	if drawManyLines {

		const charsPerFrame = 500_000
		for i := 0; i < charsPerFrame/charCount; i++ {
			p.GlyphRend.DrawTextOpenGLAbs(str, gglm.NewVec3(xOff, float32(p.GlyphRend.Atlas.LineHeight)*5+yOff, 0), textColor)
		}
		p.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps, " Draws/f: ", math.Ceil(charsPerFrame/glyphs.MaxGlyphsPerBatch), " chars/f: ", charsPerFrame, " chars/s: ", fps*charsPerFrame))
	} else {
		charsPerFrame := float64(charCount)
		p.GlyphRend.DrawTextOpenGLAbs(str, gglm.NewVec3(xOff, float32(p.GlyphRend.Atlas.LineHeight)*5+yOff, 0), textColor)
		p.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps, " Draws/f: ", math.Ceil(charsPerFrame/glyphs.MaxGlyphsPerBatch), " chars/f: ", int(charsPerFrame), " chars/s: ", fps*int(charsPerFrame)))
	}

}

func (p *program) drawGrid() {

	sizeX := float32(p.GlyphRend.ScreenWidth)
	sizeY := float32(p.GlyphRend.ScreenHeight)

	//columns
	adv := p.GlyphRend.Atlas.Advance
	for i := int32(0); i < p.GlyphRend.ScreenWidth; i += int32(adv) {
		p.rend.Draw(p.gridMesh, gglm.NewTrMatId().Translate(gglm.NewVec3(float32(i)+0.5, sizeY/2, 0)).Scale(gglm.NewVec3(1, sizeY, 1)), p.gridMat)
	}

	//rows
	for i := int32(0); i < p.GlyphRend.ScreenHeight; i += int32(p.GlyphRend.Atlas.LineHeight) {
		p.rend.Draw(p.gridMesh, gglm.NewTrMatId().Translate(gglm.NewVec3(sizeX/2, float32(i), 0)).Scale(gglm.NewVec3(sizeX, 1, 1)), p.gridMat)
	}
}

func (p *program) FrameEnd() {
	// engine.Quit()
}

func (p *program) DeInit() {

}

func (p *program) handleWindowResize() {
	w, h := p.win.SDLWin.GetSize()
	p.GlyphRend.SetScreenSize(w, h)

	projMtx := gglm.Ortho(0, float32(w), float32(h), 0, 0.1, 20)
	viewMtx := gglm.LookAt(gglm.NewVec3(0, 0, -10), gglm.NewVec3(0, 0, 0), gglm.NewVec3(0, 1, 0))
	p.gridMat.SetUnifMat4("projViewMat", &projMtx.Mul(viewMtx).Mat4)
}
