package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime/pprof"
	"strings"

	"github.com/bloeys/gglm/gglm"
	"github.com/bloeys/nmage/engine"
	"github.com/bloeys/nmage/input"
	"github.com/bloeys/nmage/materials"
	"github.com/bloeys/nmage/meshes"
	"github.com/bloeys/nmage/renderer/rend3dgl"
	"github.com/bloeys/nmage/timing"
	nmageimgui "github.com/bloeys/nmage/ui/imgui"
	"github.com/bloeys/nterm/assert"
	"github.com/bloeys/nterm/consts"
	"github.com/bloeys/nterm/glyphs"
	"github.com/golang/freetype/truetype"
	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/exp/constraints"
	"golang.org/x/image/font"
)

type Cmd struct {
	C      *exec.Cmd
	Stdout io.ReadCloser
	Stdin  io.WriteCloser
	Stderr io.ReadCloser
}

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

	textBuf     []rune
	textBufSize int64
	textBufLen  int64

	cmdBuf    []rune
	cmdBufLen int64

	cursorCharIndex int64
	//lastCmdCharPos is the screen pos of the last cmdBuf char drawn this frame
	lastCmdCharPos *gglm.Vec3
	scrollPos      int64
	scrollSpd      int64

	activeCmd *Cmd
}

const (
	subPixelX = 64
	subPixelY = 64
	hinting   = font.HintingNone

	defaultCmdBufSize  = 4 * 1024
	defaultTextBufSize = 4 * 1024 * 1024

	defaultScrollSpd = 5
)

var (
	// isDrawingBounds = false
	drawManyLines = false
	drawGrid      bool

	textToShow = ""
	textColor  = gglm.NewVec4(1, 1, 1, 1)

	xOff float32 = 0
	yOff float32 = 0
)

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
		FontSize:  40,

		textBuf:     make([]rune, defaultTextBufSize),
		textBufSize: defaultTextBufSize,
		textBufLen:  0,

		cursorCharIndex: 0,
		lastCmdCharPos:  gglm.NewVec3(0, 0, 0),
		cmdBuf:          make([]rune, defaultCmdBufSize),
		cmdBufLen:       0,

		scrollSpd: defaultScrollSpd,
	}

	p.win.EventCallbacks = append(p.win.EventCallbacks, p.handleSDLEvent)

	//Don't flash white
	p.win.SDLWin.GLSwap()

	if consts.Mode_Debug {
		var pf, _ = os.Create("pprof.cpu")
		defer pf.Close()
		pprof.StartCPUProfile(pf)
	}

	engine.Run(p, p.win, p.imguiInfo)

	if consts.Mode_Debug {
		pprof.StopCPUProfile()
	}
}

func (p *program) handleSDLEvent(e sdl.Event) {

	switch e := e.(type) {

	case *sdl.TextInputEvent:
		p.WriteToCmdBuf([]rune(e.GetText()))
	case *sdl.WindowEvent:
		if e.Event == sdl.WINDOWEVENT_SIZE_CHANGED {
			p.HandleWindowResize()
		}
	}
}

func (p *program) Init() {

	dpi, _, _, err := sdl.GetDisplayDPI(0)
	if err != nil {
		panic("Failed to get display DPI. Err: " + err.Error())
	}
	fmt.Printf("DPI: %f, font size: %d\n", dpi, p.FontSize)

	w, h := p.win.SDLWin.GetSize()
	// p.GlyphRend, err = glyphs.NewGlyphRend("./res/fonts/tajawal-regular-var.ttf", &truetype.Options{Size: float64(p.FontSize), DPI: p.Dpi, SubPixelsX: subPixelX, SubPixelsY: subPixelY, Hinting: hinting}, w, h)
	p.GlyphRend, err = glyphs.NewGlyphRend("./res/fonts/alm-fixed.ttf", &truetype.Options{Size: float64(p.FontSize), DPI: p.Dpi, SubPixelsX: subPixelX, SubPixelsY: subPixelY, Hinting: hinting}, w, h)
	if err != nil {
		panic("Failed to create atlas from font file. Err: " + err.Error())
	}

	if consts.Mode_Debug {
		glyphs.SaveImgToPNG(p.GlyphRend.Atlas.Img, "./debug-atlas.png")
	}

	//Load resources
	p.gridMesh, err = meshes.NewMesh("grid", "./res/models/quad.obj", 0)
	if err != nil {
		panic(err.Error())
	}

	p.gridMat = materials.NewMaterial("grid", "./res/shaders/grid.glsl")
	p.HandleWindowResize()

	//Set initial cursor pos
	p.lastCmdCharPos.SetY(p.GlyphRend.Atlas.LineHeight)
}

func (p *program) Update() {

	if input.IsQuitClicked() || input.KeyClicked(sdl.K_ESCAPE) {
		engine.Quit()
	}

	if consts.Mode_Debug {
		p.DebugUpdate()
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

	p.MainUpdate()
}

// @TODO: These probably need a mutex
func (p *program) WriteToTextBuf(text []rune) {

	newHeadPos := p.textBufLen + int64(len(text))
	if newHeadPos <= p.textBufSize {
		copy(p.textBuf[p.textBufLen:], text)
		p.textBufLen = newHeadPos
		return
	}

	assert.T(false, "Circular buffer not implemented for text buf")
}

func (p *program) WriteToCmdBuf(text []rune) {

	delta := int64(len(text))
	newHeadPos := p.cmdBufLen + delta
	if newHeadPos <= p.textBufSize {

		// fmt.Println("\nBuf before delta:", p.cmdBuf[:p.cmdBufHead])
		copy(p.cmdBuf[p.cursorCharIndex+delta:], p.cmdBuf[p.cursorCharIndex:])
		// fmt.Println("Buf after delta:", p.cmdBuf[:p.cmdBufHead+delta])
		copy(p.cmdBuf[p.cursorCharIndex:], text)
		// fmt.Println("Buf after write:", p.cmdBuf[:p.cmdBufHead+delta])

		p.cursorCharIndex += delta
		p.cmdBufLen = newHeadPos
		return
	}

	assert.T(false, "Circular buffer not implemented for cmd buf")
}

var sepLinePos = gglm.NewVec3(0, 0, 0)

func (p *program) MainUpdate() {

	// Return
	if input.KeyClicked(sdl.K_RETURN) || input.KeyClicked(sdl.K_KP_ENTER) {
		p.WriteToCmdBuf([]rune{'\n'})
		p.HandleReturn()
	}

	// Cursor movement and scroll
	if input.KeyClicked(sdl.K_LEFT) {
		p.cursorCharIndex = clamp(p.cursorCharIndex-1, 0, p.cmdBufLen)
	} else if input.KeyClicked(sdl.K_RIGHT) {
		p.cursorCharIndex = clamp(p.cursorCharIndex+1, 0, p.cmdBufLen)
	}

	if input.KeyClicked(sdl.K_HOME) {
		p.cursorCharIndex = 0
	} else if input.KeyClicked(sdl.K_END) {
		p.cursorCharIndex = p.cmdBufLen
	}

	if mouseWheelYNorm := -int64(input.GetMouseWheelYNorm()); mouseWheelYNorm != 0 {
		p.scrollPos = clamp(p.scrollPos+p.scrollSpd*mouseWheelYNorm, 0, p.textBufLen)
	}

	// Delete inputs
	// @TODO: Implement hold to delete
	if input.KeyClicked(sdl.K_BACKSPACE) {
		p.DeletePrevChar()
	}

	if input.KeyClicked(sdl.K_DELETE) {
		p.DeleteNextChar()
	}

	//Draw textBuf
	p.lastCmdCharPos.Data = p.GlyphRend.DrawTextOpenGLAbs(p.textBuf[p.scrollPos:p.textBufLen], gglm.NewVec3(0, float32(p.GlyphRend.ScreenHeight)-p.GlyphRend.Atlas.LineHeight, 0), gglm.NewVec4(1, 1, 1, 1)).Data
	sepLinePos.Data = p.lastCmdCharPos.Data

	//Draw cmd buf
	p.lastCmdCharPos.SetX(0)
	p.lastCmdCharPos.AddY(-p.GlyphRend.Atlas.LineHeight)
	p.lastCmdCharPos.Data = p.GlyphRend.DrawTextOpenGLAbs(p.cmdBuf[:p.cmdBufLen], p.lastCmdCharPos, gglm.NewVec4(1, 1, 1, 1)).Data
}

func (p *program) DeletePrevChar() {

	if p.cursorCharIndex == 0 || p.cmdBufLen == 0 {
		return
	}

	copy(p.cmdBuf[p.cursorCharIndex-1:], p.cmdBuf[p.cursorCharIndex:])

	p.cmdBufLen--
	p.cursorCharIndex--
}

func (p *program) DeleteNextChar() {

	if p.cmdBufLen == 0 || p.cursorCharIndex == p.cmdBufLen {
		return
	}

	copy(p.cmdBuf[p.cursorCharIndex:], p.cmdBuf[p.cursorCharIndex+1:])

	p.cmdBufLen--
}

func (p *program) HandleReturn() {

	cmdRunes := p.cmdBuf[:p.cmdBufLen]
	p.cmdBufLen = 0
	p.cursorCharIndex = 0

	if p.activeCmd != nil {

		_, err := p.activeCmd.Stdin.Write([]byte(string(cmdRunes)))
		if err != nil {
			p.PrintToTextBuf(fmt.Sprintf("Writing to stdin pipe of '%s' failed. Error: %s\n", p.activeCmd.C.Path, err.Error()))
			p.ClearActiveCmd()
			return
		}

		return
	}

	p.WriteToTextBuf(cmdRunes)

	cmdStr := strings.TrimSpace(string(cmdRunes))
	cmdSplit := strings.Split(cmdStr, " ")

	cmdName := cmdSplit[0]
	var args []string
	if len(cmdSplit) >= 2 {
		args = cmdSplit[1:]
	}

	cmd := exec.Command(cmdName, args...)

	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		p.PrintToTextBuf(fmt.Sprintf("Creating stdout pipe of '%s' failed. Error: %s\n", cmdName, err.Error()))
		return
	}

	inPipe, err := cmd.StdinPipe()
	if err != nil {
		p.PrintToTextBuf(fmt.Sprintf("Creating stdin pipe of '%s' failed. Error: %s\n", cmdName, err.Error()))
		return
	}

	errPipe, err := cmd.StderrPipe()
	if err != nil {
		p.PrintToTextBuf(fmt.Sprintf("Creating stderr pipe of '%s' failed. Error: %s\n", cmdName, err.Error()))
		return
	}

	err = cmd.Start()
	if err != nil {
		p.PrintToTextBuf(fmt.Sprintf("Running '%s' failed. Error: %s\n", cmdName, err.Error()))
		return
	}
	p.activeCmd = &Cmd{
		C:      cmd,
		Stdout: outPipe,
		Stdin:  inPipe,
		Stderr: errPipe,
	}

	//Stdout
	go func() {

		defer p.ClearActiveCmd()

		buf := make([]byte, 1024)
		for p.activeCmd != nil {

			readBytes, err := p.activeCmd.Stdout.Read(buf)
			if err != nil {

				if err == io.EOF {
					break
				}

				p.PrintToTextBuf("Stdout pipe failed. Error: " + err.Error())
				return
			}

			if readBytes == 0 {
				continue
			}

			p.PrintToTextBuf(string(buf[:readBytes]))
		}
	}()

	//Stderr
	go func() {

		defer p.ClearActiveCmd()

		buf := make([]byte, 1024)
		for p.activeCmd != nil {

			readBytes, err := p.activeCmd.Stderr.Read(buf)
			if err != nil {

				if err == io.EOF {
					break
				}

				p.PrintToTextBuf("Stderr pipe failed. Error: " + err.Error())
				return
			}

			if readBytes == 0 {
				continue
			}

			p.PrintToTextBuf(string(buf[:readBytes]))
		}
	}()
}

func (p *program) ClearActiveCmd() {

	if p.activeCmd == nil {
		return
	}

	p.activeCmd = nil
}

func (p *program) PrintToTextBuf(s string) {
	p.WriteToTextBuf([]rune(s))
}

func (p *program) DrawCursor() {

	//Position cursor by placing it at the end of the drawn characters then walking backwards
	pos := p.lastCmdCharPos.Clone()
	p.ScreenPosToGridPos(pos)

	pos.AddY(p.GlyphRend.Atlas.LineHeight * 0.5)
	for i := clamp(p.cmdBufLen, 0, int64(len(p.cmdBuf))); i > p.cursorCharIndex; i-- {

		if p.cmdBuf[i] == '\n' {
			pos.AddY(p.GlyphRend.Atlas.LineHeight)
			continue
		}
		pos.AddX(-p.GlyphRend.Atlas.SpaceAdvance)
	}

	p.rend.Draw(p.gridMesh, gglm.NewTrMatId().Translate(pos).Scale(gglm.NewVec3(0.1*p.GlyphRend.Atlas.SpaceAdvance, p.GlyphRend.Atlas.LineHeight, 1)), p.gridMat)

	// @Debug draw line indicating last char in line
	// pos = p.lastCmdCharPos.Clone()
	// p.ScreenPosToGridPos(pos)
	// p.gridMat.SetUnifVec4("color", gglm.NewVec4(1, 0, 0, 1))
	// p.rend.Draw(p.gridMesh, gglm.NewTrMatId().Translate(pos).Scale(gglm.NewVec3(0.1*p.GlyphRend.Atlas.SpaceAdvance, p.GlyphRend.Atlas.LineHeight, 1)), p.gridMat)
	// p.gridMat.SetUnifVec4("color", gglm.NewVec4(1, 1, 1, 1))
}

func (p *program) ScreenPosToGridPos(screenPos *gglm.Vec3) {
	screenPos.SetX(screenPos.X() / p.GlyphRend.Atlas.SpaceAdvance * p.GlyphRend.Atlas.SpaceAdvance)
	screenPos.SetY(screenPos.Y() / p.GlyphRend.Atlas.LineHeight * p.GlyphRend.Atlas.LineHeight)
}

func (p *program) DebugUpdate() {

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
	if input.KeyDown(sdl.K_LCTRL) && input.KeyClicked(sdl.K_SPACE) {
		drawGrid = !drawGrid
	}
}

func (p *program) Render() {

	defer p.GlyphRend.Draw()

	if consts.Mode_Debug {
		p.DebugRender()

		sizeX := float32(p.GlyphRend.ScreenWidth)
		p.rend.Draw(p.gridMesh, gglm.NewTrMatId().Translate(gglm.NewVec3(sizeX/2, sepLinePos.Y(), 0)).Scale(gglm.NewVec3(sizeX, 1, 1)), p.gridMat)
	}

	p.DrawCursor()
}

func (p *program) DebugRender() {

	if drawGrid {
		p.DrawGrid()
	}

	str := textToShow
	charCount := len([]rune(str))
	fps := int(timing.GetAvgFPS())
	if drawManyLines {
		const charsPerFrame = 500_000
		for i := 0; i < charsPerFrame/charCount; i++ {
			p.GlyphRend.DrawTextOpenGLAbsString(str, gglm.NewVec3(xOff, float32(p.GlyphRend.Atlas.LineHeight)*5+yOff, 0), textColor)
		}
		p.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps, " Draws/f: ", math.Ceil(charsPerFrame/glyphs.MaxGlyphsPerBatch), " chars/f: ", charsPerFrame, " chars/s: ", fps*charsPerFrame))
	} else {
		charsPerFrame := float64(charCount)
		p.GlyphRend.DrawTextOpenGLAbsString(str, gglm.NewVec3(xOff, float32(p.GlyphRend.Atlas.LineHeight)*5+yOff, 0), textColor)
		p.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps, " Draws/f: ", math.Ceil(charsPerFrame/glyphs.MaxGlyphsPerBatch), " chars/f: ", int(charsPerFrame), " chars/s: ", fps*int(charsPerFrame)))
	}
}

func (p *program) DrawGrid() {

	sizeX := float32(p.GlyphRend.ScreenWidth)
	sizeY := float32(p.GlyphRend.ScreenHeight)

	//columns
	adv := p.GlyphRend.Atlas.SpaceAdvance
	for i := 0; i < int(p.GlyphRend.ScreenWidth); i++ {
		p.rend.Draw(p.gridMesh, gglm.NewTrMatId().Translate(gglm.NewVec3(adv*float32(i), sizeY/2, 0)).Scale(gglm.NewVec3(1, sizeY, 1)), p.gridMat)
	}

	//rows
	for i := int32(0); i < p.GlyphRend.ScreenHeight; i += int32(p.GlyphRend.Atlas.LineHeight) {
		p.rend.Draw(p.gridMesh, gglm.NewTrMatId().Translate(gglm.NewVec3(sizeX/2, float32(i), 0)).Scale(gglm.NewVec3(sizeX, 1, 1)), p.gridMat)
	}
}

func (p *program) FrameEnd() {
	assert.T(p.cursorCharIndex <= p.cmdBufLen, fmt.Sprintf("Cursor char index is larger than cmdBufLen! You probablly forgot to move/reset the cursor index along with the buffer length somewhere. Cursor=%d, cmdBufLen=%d\n", p.cursorCharIndex, p.cmdBufLen))
}

func (p *program) DeInit() {
}

func (p *program) HandleWindowResize() {
	w, h := p.win.SDLWin.GetSize()
	p.GlyphRend.SetScreenSize(w, h)

	projMtx := gglm.Ortho(0, float32(w), float32(h), 0, 0.1, 20)
	viewMtx := gglm.LookAt(gglm.NewVec3(0, 0, -10), gglm.NewVec3(0, 0, 0), gglm.NewVec3(0, 1, 0))
	p.gridMat.SetUnifMat4("projViewMat", &projMtx.Mul(viewMtx).Mat4)
}

func FloorF32(x float32) float32 {
	return float32(math.Floor(float64(x)))
}

func clamp[T constraints.Ordered](x, min, max T) T {

	if x < min {
		return min
	}

	if x > max {
		return max
	}

	return x
}
