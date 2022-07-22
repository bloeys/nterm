package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/bloeys/gglm/gglm"
	"github.com/bloeys/nmage/engine"
	"github.com/bloeys/nmage/input"
	"github.com/bloeys/nmage/materials"
	"github.com/bloeys/nmage/meshes"
	"github.com/bloeys/nmage/renderer/rend3dgl"
	"github.com/bloeys/nmage/timing"
	nmageimgui "github.com/bloeys/nmage/ui/imgui"
	"github.com/bloeys/nterm/ansi"
	"github.com/bloeys/nterm/assert"
	"github.com/bloeys/nterm/consts"
	"github.com/bloeys/nterm/glyphs"
	"github.com/bloeys/nterm/ring"
	"github.com/golang/freetype/truetype"
	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/exp/constraints"
	"golang.org/x/image/font"
)

type Settings struct {
	DefaultColor gglm.Vec4
	StringColor  gglm.Vec4

	MaxFps   int
	LimitFps bool
}

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

	textBuf      *ring.Buffer[byte]
	textBufMutex sync.Mutex

	cmdBuf    []rune
	cmdBufLen int64

	cursorCharIndex int64
	// lastCmdCharPos is the screen pos of the last cmdBuf char drawn this frame
	lastCmdCharPos *gglm.Vec3
	scrollPos      int64
	scrollSpd      int64
	maxCharsToShow int64
	maxLinesToShow int64

	activeCmd *Cmd
	Settings  *Settings

	frameStartTime time.Time
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

	// We do our own fps limiting because (at least some) drivers vsync by doing a busy loop and spiking
	// CPU to 100% doing nothing instead of a sleep
	engine.SetVSync(false)

	p := &program{
		win:       win,
		rend:      rend,
		imguiInfo: nmageimgui.NewImGUI(),
		FontSize:  40,

		textBuf: ring.NewBuffer[byte](defaultTextBufSize),

		cursorCharIndex: 0,
		lastCmdCharPos:  gglm.NewVec3(0, 0, 0),
		cmdBuf:          make([]rune, defaultCmdBufSize),
		cmdBufLen:       0,

		scrollSpd: defaultScrollSpd,

		Settings: &Settings{
			DefaultColor: *gglm.NewVec4(1, 1, 1, 1),
			StringColor:  *gglm.NewVec4(242/255.0, 244/255.0, 10/255.0, 1),
			MaxFps:       120,
			LimitFps:     true,
		},
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

	p.frameStartTime = time.Now()

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
			fmt.Println("Failed to update font face. Err: " + err.Error())
		} else {
			glyphs.SaveImgToPNG(p.GlyphRend.Atlas.Img, "./debug-atlas.png")
			fmt.Println("New font size:", p.FontSize, "; New texture size:", p.GlyphRend.Atlas.Img.Rect.Max.X)
		}
	}

	p.MainUpdate()
}

func (p *program) WriteToTextBuf(text []byte) {
	// This is locked because running cmds are potentially writing to it same time we are
	p.textBufMutex.Lock()
	p.textBuf.Write(text...)
	p.scrollPos = clamp(p.textBuf.Len-p.maxCharsToShow, 0, p.textBuf.Len-1)
	p.textBufMutex.Unlock()
}

func (p *program) WriteToCmdBuf(text []rune) {

	delta := int64(len(text))
	newHeadPos := p.cmdBufLen + delta
	if newHeadPos <= defaultCmdBufSize {

		copy(p.cmdBuf[p.cursorCharIndex+delta:], p.cmdBuf[p.cursorCharIndex:])
		copy(p.cmdBuf[p.cursorCharIndex:], text)

		p.cursorCharIndex += delta
		p.cmdBufLen = newHeadPos
		return
	}

	assert.T(false, "Circular buffer not implemented for cmd buf")
}

var sepLinePos = gglm.NewVec3(0, 0, 0)

func (p *program) MainUpdate() {

	if input.KeyClicked(sdl.K_RETURN) || input.KeyClicked(sdl.K_KP_ENTER) {
		p.cursorCharIndex = p.cmdBufLen // This is so \n is written to the end of the cmdBuf
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

		var newPosNewLines int64
		w, _ := p.GridSize()
		if mouseWheelYNorm < 0 {
			newPosNewLines, _ = find_n_lines_index_iterator(p.textBuf.Iterator(), p.scrollPos, p.scrollSpd*mouseWheelYNorm-1, int64(w))
		} else {
			newPosNewLines, _ = find_n_lines_index_iterator(p.textBuf.Iterator(), p.scrollPos, p.scrollSpd*mouseWheelYNorm, int64(w))
		}

		p.scrollPos = clamp(newPosNewLines, 0, p.textBuf.Len)
	}

	// Delete inputs
	// @TODO: Implement hold to delete
	if input.KeyClicked(sdl.K_BACKSPACE) {
		p.DeletePrevChar()
	}

	if input.KeyClicked(sdl.K_DELETE) {
		p.DeleteNextChar()
	}

	// Draw textBuf
	v1, v2 := p.textBuf.Views()

	from := clamp(p.scrollPos, 0, int64(len(v1)-1))
	to := clamp(p.scrollPos+p.maxCharsToShow, 0, int64(len(v1)-1))
	p.lastCmdCharPos.Data = p.DrawTextAnsiCodes(v1[from:to], *gglm.NewVec3(0, float32(p.GlyphRend.ScreenHeight)-p.GlyphRend.Atlas.LineHeight, 0)).Data

	if p.scrollPos >= int64(len(v1)) {

		from := clamp(p.scrollPos-int64(len(v1)), 0, int64(len(v2)-1))
		to := clamp(p.scrollPos+p.maxCharsToShow, 0, int64(len(v2)-1))
		p.lastCmdCharPos.Data = p.DrawTextAnsiCodes(v2[from:to], *p.lastCmdCharPos).Data
	}

	sepLinePos.Data = p.lastCmdCharPos.Data

	// Draw cmd buf
	p.lastCmdCharPos.SetX(0)
	p.lastCmdCharPos.AddY(-p.GlyphRend.Atlas.LineHeight)
	p.lastCmdCharPos.Data = p.SyntaxHighlightAndDraw(p.cmdBuf[:p.cmdBufLen], *p.lastCmdCharPos).Data
}

func bytesToRunes(b []byte) []rune {

	runeCount := utf8.RuneCount(b)
	if runeCount == 0 {
		return []rune{}
	}

	// @PERF We should use a pre-allocated buffer here
	out := make([]rune, 0, runeCount)
	for {

		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError {
			break
		}

		out = append(out, r)
		b = b[size:]
	}

	return out
}

func (p *program) DrawTextAnsiCodes(bs []byte, pos gglm.Vec3) gglm.Vec3 {

	startPos := pos.Clone()
	currColor := p.Settings.DefaultColor

	draw := func(rs []rune) {

		startIndex := 0
		for i := 0; i < len(rs); i++ {

			r := rs[i]

			// @PERF We could probably use bytes.IndexByte here
			if r == '\n' {
				pos.Data = p.GlyphRend.DrawTextOpenGLAbs(rs[startIndex:i], &pos, &currColor).Data
				pos.SetX(startPos.X())
				pos.AddY(-p.GlyphRend.Atlas.LineHeight)
				startIndex = i + 1
				continue
			}
		}

		if startIndex < len(rs) {
			pos.Data = p.GlyphRend.DrawTextOpenGLAbs(rs[startIndex:], &pos, &currColor).Data
		}
	}

	for {

		index, code := ansi.NextAnsiCode(bs)
		if index == -1 {
			draw(bytesToRunes(bs))
			break
		}

		// Draw text before the code
		before := bytesToRunes(bs[:index])
		draw(before)

		//Apply code
		info := ansi.InfoFromAnsiCode(code)
		if info.Options&ansi.AnsiCodeOptions_ColorFg != 0 {

			if info.Info1.X() == -1 {
				currColor = p.Settings.DefaultColor
			} else {
				currColor = info.Info1
			}
		}

		// Advance beyond the code chars
		bs = bs[index+len(code):]
	}

	return pos
}

func (p *program) SyntaxHighlightAndDraw(text []rune, pos gglm.Vec3) gglm.Vec3 {

	startIndex := 0
	startPos := pos.Clone()
	currColor := &p.Settings.DefaultColor

	inSingleString := false
	inDoubleString := false
	for i := 0; i < len(text); i++ {

		r := text[i]
		switch r {

		// Text might be drawn in multiple calls, once per color for example. If the first half
		// of the text gets drawn and the second half has a newline, the renderer will reset the X pos
		// to the middle of the text not the start as it uses the start X position of the second half.
		// So to get correct new line handling we handle newlines here
		case '\n':
			pos.Data = p.GlyphRend.DrawTextOpenGLAbs(text[startIndex:i], &pos, currColor).Data
			pos.SetX(startPos.X())
			pos.AddY(-p.GlyphRend.Atlas.LineHeight)
			startIndex = i + 1
			continue

		case '"':

			if inSingleString {
				continue
			}

			if !inDoubleString {
				pos.Data = p.GlyphRend.DrawTextOpenGLAbs(text[startIndex:i], &pos, currColor).Data

				startIndex = i
				inDoubleString = true
				currColor = &p.Settings.StringColor
				continue
			}

			pos.Data = p.GlyphRend.DrawTextOpenGLAbs(text[startIndex:i+1], &pos, currColor).Data
			startIndex = i + 1
			inDoubleString = false
			currColor = &p.Settings.DefaultColor

		case '\'':
			if inDoubleString {
				continue
			}

			if !inSingleString {
				pos.Data = p.GlyphRend.DrawTextOpenGLAbs(text[startIndex:i], &pos, currColor).Data

				startIndex = i
				inSingleString = true
				currColor = &p.Settings.StringColor
				continue
			}

			pos.Data = p.GlyphRend.DrawTextOpenGLAbs(text[startIndex:i+1], &pos, &p.Settings.StringColor).Data
			startIndex = i + 1
			inSingleString = false
			currColor = &p.Settings.DefaultColor
		}
	}

	if startIndex < len(text) {
		if inDoubleString || inSingleString {
			pos.Data = p.GlyphRend.DrawTextOpenGLAbs(text[startIndex:], &pos, &p.Settings.StringColor).Data
		} else {
			pos.Data = p.GlyphRend.DrawTextOpenGLAbs(text[startIndex:], &pos, &p.Settings.DefaultColor).Data
		}
	}

	return pos
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

	// @PERF
	cmdStr := string(cmdRunes)
	cmdBytes := []byte(cmdStr)
	p.WriteToTextBuf(cmdBytes)

	if p.activeCmd != nil {

		// println("Wrote:", string(cmdBytes))
		_, err := p.activeCmd.Stdin.Write(cmdBytes)
		if err != nil {
			p.WriteToTextBuf([]byte(fmt.Sprintf("Writing to stdin pipe of '%s' failed. Error: %s\n", p.activeCmd.C.Path, err.Error())))
			p.ClearActiveCmd()
			return
		}

		return
	}

	cmdSplit := strings.Split(strings.TrimSpace(cmdStr), " ")
	cmdName := cmdSplit[0]
	var args []string
	if len(cmdSplit) >= 2 {
		args = cmdSplit[1:]
	}

	cmd := exec.Command(cmdName, args...)
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CmdLine: strings.TrimSpace(cmdStr),
		}
	}

	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		p.WriteToTextBuf([]byte(fmt.Sprintf("Creating stdout pipe of '%s' failed. Error: %s\n", cmdName, err.Error())))
		return
	}

	inPipe, err := cmd.StdinPipe()
	if err != nil {
		p.WriteToTextBuf([]byte(fmt.Sprintf("Creating stdin pipe of '%s' failed. Error: %s\n", cmdName, err.Error())))
		return
	}

	errPipe, err := cmd.StderrPipe()
	if err != nil {
		p.WriteToTextBuf([]byte(fmt.Sprintf("Creating stderr pipe of '%s' failed. Error: %s\n", cmdName, err.Error())))
		return
	}

	startTime := time.Now()
	err = cmd.Start()
	if err != nil {
		p.WriteToTextBuf([]byte(fmt.Sprintf("Running '%s' failed. Error: %s\n", cmdName, err.Error())))
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

		defer func() {
			fmt.Printf("Cmd '%s' took %0.2fs\n", cmdName, time.Since(startTime).Seconds())
		}()

		defer p.ClearActiveCmd()
		buf := make([]byte, 1024)
		for p.activeCmd != nil {

			readBytes, err := p.activeCmd.Stdout.Read(buf)
			if err != nil {

				if err == io.EOF {
					break
				}

				p.WriteToTextBuf([]byte("Stdout pipe failed. Error: " + err.Error()))
				return
			}

			if readBytes == 0 {
				continue
			}

			p.WriteToTextBuf(buf[:readBytes])
			// println("Read:", string(buf[:readBytes]))
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

				p.WriteToTextBuf([]byte("Stderr pipe failed. Error: " + err.Error()))
				return
			}

			if readBytes == 0 {
				continue
			}

			p.WriteToTextBuf(buf[:readBytes])
		}
	}()
}

func (p *program) ClearActiveCmd() {

	if p.activeCmd == nil {
		return
	}

	p.activeCmd = nil
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
}

func (p *program) GridSize() (w, h int32) {
	return p.GlyphRend.ScreenWidth / int32(p.GlyphRend.Atlas.SpaceAdvance), p.GlyphRend.ScreenHeight / int32(p.GlyphRend.Atlas.LineHeight)
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

	fps := int(timing.GetAvgFPS())
	if len(textToShow) > 0 {
		str := textToShow
		charCount := len([]rune(str))
		if drawManyLines {
			const charsPerFrame = 500_000
			for i := 0; i < charsPerFrame/charCount; i++ {
				p.GlyphRend.DrawTextOpenGLAbsString(str, gglm.NewVec3(xOff, float32(p.GlyphRend.Atlas.LineHeight)*5+yOff, 0), &p.Settings.DefaultColor)
			}
			p.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps, " Draws/f: ", math.Ceil(charsPerFrame/glyphs.DefaultGlyphsPerBatch), " chars/f: ", charsPerFrame, " chars/s: ", fps*charsPerFrame))
		} else {
			charsPerFrame := float64(charCount)
			p.GlyphRend.DrawTextOpenGLAbsString(str, gglm.NewVec3(xOff, float32(p.GlyphRend.Atlas.LineHeight)*5+yOff, 0), &p.Settings.DefaultColor)
			p.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps, " Draws/f: ", math.Ceil(charsPerFrame/glyphs.DefaultGlyphsPerBatch), " chars/f: ", int(charsPerFrame), " chars/s: ", fps*int(charsPerFrame)))
		}
	} else {
		p.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps))
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

	if p.Settings.LimitFps {

		elapsed := time.Since(p.frameStartTime)
		microSecondsPerFrame := int64(1 / float32(p.Settings.MaxFps) * 1000_000)

		// Sleep time is reduced by a millisecond to compensate for the (nearly) inevitable over-sleeping that will happen.
		timeToSleep := time.Duration((microSecondsPerFrame - elapsed.Microseconds()) * 1000)
		timeToSleep -= 1000 * time.Microsecond

		if timeToSleep.Milliseconds() > 0 {
			time.Sleep(timeToSleep)
		}
	}
}

func (p *program) DeInit() {
}

func (p *program) HandleWindowResize() {
	w, h := p.win.SDLWin.GetSize()
	p.GlyphRend.SetScreenSize(w, h)

	projMtx := gglm.Ortho(0, float32(w), float32(h), 0, 0.1, 20)
	viewMtx := gglm.LookAt(gglm.NewVec3(0, 0, -10), gglm.NewVec3(0, 0, 0), gglm.NewVec3(0, 1, 0))
	p.gridMat.SetUnifMat4("projViewMat", &projMtx.Mul(viewMtx).Mat4)

	// We show a bit more than calculated to be safe against showing empty space
	p.maxLinesToShow = int64(CeilF32(float32(h)/float32(p.GlyphRend.Atlas.LineHeight)) * 1.25)
	p.maxCharsToShow = int64(CeilF32(float32(w)/float32(p.GlyphRend.Atlas.SpaceAdvance))*1.25) * p.maxLinesToShow
}

func FloorF32(x float32) float32 {
	return float32(math.Floor(float64(x)))
}

func CeilF32(x float32) float32 {
	return float32(math.Ceil(float64(x)))
}

func clamp[T constraints.Ordered](x, min, max T) T {

	if max < min {
		min, max = max, min
	}

	if x < min {
		return min
	}

	if x > max {
		return max
	}

	return x
}

func FindNthOrLastIndex[T comparable](arr []T, x T, startIndex, n int64) (lastIndex int64) {

	lastIndex = -1
	if n >= 0 {

		for i := startIndex; i < int64(len(arr)); i++ {

			if arr[i] != x {
				continue
			}
			lastIndex = i

			n--
			if n <= 0 {
				return i
			}
		}

	} else {

		for i := startIndex; i >= 0; i-- {

			if arr[i] != x {
				continue
			}
			lastIndex = i

			n++
			if n >= 0 {
				return i
			}
		}
	}

	return lastIndex
}

// find_n_lines_index starts at startIndex and moves n lines forward/backward, depending on whether 'n' is negative or positive,
// then returns the index of the nth line and the size of char in bytes that preceeds the line.
//
// A line is counted when either a '\n' is seen or by seeing enough chars that a wrap is required.
//
// Note: When moving backwards from the start of the line, the first char will be a new line (e.g. \n), so the first counted line is not a full line
// but only a single rune. So in most cases to get '-n' lines backwards you should request '-n-1' lines.
func find_n_lines_index_iterator(it ring.Iterator[byte], startIndex, n, charsPerLine int64) (newIndex, newSize int64) {

	// If nothing changes (e.g. already at end of iterator) then we will stay at the same place

	done := false
	read := 0
	bytesToKeep := 0
	buf := make([]byte, 4)
	it.GotoIndex(startIndex)

	// @Note we should ignore zero width glyphs
	// @Note is this better in glyphs package?
	bytesSeen := int64(0)
	charsSeenThisLine := int64(0)

	if n >= 0 {

		// If nothing changes (e.g. already at end of iterator) then we will stay at the same place
		newIndex = startIndex

		for !done || bytesToKeep > 0 {

			read, done = it.NextN(buf[bytesToKeep:], 4)

			r, size := utf8.DecodeRune(buf[:bytesToKeep+read])
			if r == utf8.RuneError {
				break
			}
			bytesToKeep += read - size
			copy(buf, buf[size:size+bytesToKeep])

			charsSeenThisLine++
			bytesSeen += int64(size)

			// If this is true we covered one line
			if charsSeenThisLine == charsPerLine || r == '\n' {

				charsSeenThisLine = 0
				newSize = int64(size)
				newIndex = startIndex + bytesSeen

				n--
				if n <= 0 {
					break
				}
			}
		}

	} else {

		for !done || bytesToKeep > 0 {

			read, done = it.PrevN(buf[bytesToKeep:], 4)

			r, size := utf8.DecodeRune(buf[:bytesToKeep+read])
			if r == utf8.RuneError {
				break
			}
			bytesToKeep += read - size
			copy(buf, buf[size:size+bytesToKeep])

			charsSeenThisLine++
			bytesSeen += int64(size)

			// If this is true we covered one line
			if charsSeenThisLine == charsPerLine || r == '\n' {

				charsSeenThisLine = 0
				newSize = int64(size)
				newIndex = startIndex - bytesSeen + newSize

				n++
				if n >= 0 {
					break
				}
			}
		}

		// If we reached beginning of buffer before finding a new line then newIndex is zero
		if startIndex-bytesSeen == 0 {
			newIndex = 0
			newSize = 0
		}
	}

	return newIndex, newSize
}
