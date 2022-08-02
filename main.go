package main

import (
	"bytes"
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
	DefaultFgColor gglm.Vec4
	DefaultBgColor gglm.Vec4
	StringColor    gglm.Vec4

	MaxFps   int
	LimitFps bool
}

type Cmd struct {
	C      *exec.Cmd
	Stdout io.ReadCloser
	Stdin  io.WriteCloser
	Stderr io.ReadCloser
}

// Line represents a series of chars between two new-lines.
// The indices are in terms of total written elements to the ring buffer
type Line struct {
	StartIndex_WriteCount uint64
	EndIndex_WriteCount   uint64
}

func (l *Line) Len() uint64 {
	size := l.EndIndex_WriteCount - l.StartIndex_WriteCount
	return size
}

var _ engine.Game = &nterm{}

type nterm struct {
	win       *engine.Window
	rend      *rend3dgl.Rend3DGL
	imguiInfo nmageimgui.ImguiInfo

	FontSize  uint32
	Dpi       float64
	GlyphRend *glyphs.GlyphRend

	gridMesh *meshes.Mesh
	gridMat  *materials.Material

	LineBeingParsed Line
	Lines           *ring.Buffer[Line]

	textBuf      *ring.Buffer[byte]
	textBufMutex sync.Mutex

	cmdBuf    []rune
	cmdBufLen int64

	cursorCharIndex int64
	// lastCmdCharPos is the screen pos of the last cmdBuf char drawn this frame
	lastCmdCharPos *gglm.Vec3
	scrollPosRel   int64
	scrollSpd      int64

	CellCountX int64
	CellCountY int64
	CellCount  int64

	activeCmd *Cmd
	Settings  *Settings

	frameStartTime time.Time

	SepLinePos gglm.Vec3

	firstValidLine *Line
}

const (
	subPixelX = 64
	subPixelY = 64
	hinting   = font.HintingNone

	defaultCmdBufSize  = 4 * 1024
	defaultLineBufSize = 10 * 1024 // Max number of lines
	defaultTextBufSize = 8 * 1024 * 1024

	// How many lines to move per scroll
	defaultScrollSpd = 1
)

var (
	drawGrid      bool
	drawManyLines = false

	textToShow = ""
	// textToShow = "Hello there, friend!"

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

	p := &nterm{
		win:       win,
		rend:      rend,
		imguiInfo: nmageimgui.NewImGUI(),
		FontSize:  40,

		Lines: ring.NewBuffer[Line](defaultLineBufSize),

		textBuf: ring.NewBuffer[byte](defaultTextBufSize),

		cursorCharIndex: 0,
		lastCmdCharPos:  gglm.NewVec3(0, 0, 0),
		cmdBuf:          make([]rune, defaultCmdBufSize),
		cmdBufLen:       0,

		scrollSpd: defaultScrollSpd,

		Settings: &Settings{
			DefaultFgColor: *gglm.NewVec4(1, 1, 1, 1),
			DefaultBgColor: *gglm.NewVec4(0, 0, 0, 0),
			StringColor:    *gglm.NewVec4(242/255.0, 244/255.0, 10/255.0, 1),
			MaxFps:         120,
			LimitFps:       true,
		},

		firstValidLine: &Line{},
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

func (p *nterm) handleSDLEvent(e sdl.Event) {

	switch e := e.(type) {

	case *sdl.TextInputEvent:
		p.WriteToCmdBuf([]rune(e.GetText()))
	case *sdl.WindowEvent:
		if e.Event == sdl.WINDOWEVENT_SIZE_CHANGED {
			p.HandleWindowResize()
		}
	}
}

func (p *nterm) Init() {

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

	p.GlyphRend.OptValues.BgColor = gglm.NewVec4(0, 0, 0, 0)
	p.GlyphRend.SetOpts(glyphs.GlyphRendOpt_BgColor)

	// if consts.Mode_Debug {
	// glyphs.SaveImgToPNG(p.GlyphRend.Atlas.Img, "./debug-atlas.png")
	// }

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

func (p *nterm) Update() {

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

func (nt *nterm) MainUpdate() {

	// Keep a reference to the first valid line
	if !IsLineValid(nt.textBuf, nt.firstValidLine) || nt.firstValidLine.Len() == 0 {

		lineIt := nt.Lines.Iterator()
		for p, done := lineIt.NextPtr(); !done; p, done = lineIt.NextPtr() {

			lineStatus := getLineStatus(nt.textBuf, p)
			if lineStatus == LineStatus_Invalid {
				continue
			}

			// If start index is invalid but end index is still valid then we push the start into a valid position
			if lineStatus == LineStatus_PartiallyInvalid {
				diff := nt.textBuf.WrittenElements - nt.firstValidLine.StartIndex_WriteCount
				deltaToValid := diff - uint64(nt.textBuf.Cap) + 1 // How much we need to move startIndex to be barely valid
				nt.firstValidLine.StartIndex_WriteCount = clamp(nt.firstValidLine.StartIndex_WriteCount+deltaToValid, 0, nt.firstValidLine.EndIndex_WriteCount-1)
			}

			nt.firstValidLine = p
			break
		}
	}

	// We might have way more chars than lines and so the first line might not start
	// at the first char, but midway in the buffer, so we ensure that scrollPosRel
	// starts at the first line
	firstValidLineStartIndexRel := int64(nt.textBuf.RelIndexFromWriteCount(nt.firstValidLine.StartIndex_WriteCount))
	if nt.scrollPosRel < firstValidLineStartIndexRel {
		nt.scrollPosRel = firstValidLineStartIndexRel
	}

	nt.ReadInputs()

	// Line separator
	nt.SepLinePos.SetY(2 * nt.GlyphRend.Atlas.LineHeight)

	// Draw textBuf
	gw, gh := nt.GridSize()
	v1, v2 := nt.textBuf.ViewsFromToRelIndex(uint64(nt.scrollPosRel), uint64(nt.scrollPosRel)+uint64(gw*gh))

	nt.lastCmdCharPos.Data = gglm.NewVec3(0, float32(nt.GlyphRend.ScreenHeight)-nt.GlyphRend.Atlas.LineHeight, 0).Data
	nt.lastCmdCharPos.Data = nt.DrawTextAnsiCodes(v1, *nt.lastCmdCharPos).Data
	nt.lastCmdCharPos.Data = nt.DrawTextAnsiCodes(v2, *nt.lastCmdCharPos).Data

	// Draw cmd buf
	nt.lastCmdCharPos.SetX(0)
	nt.lastCmdCharPos.SetY(nt.SepLinePos.Y() - nt.GlyphRend.Atlas.LineHeight)
	nt.lastCmdCharPos.Data = nt.SyntaxHighlightAndDraw(nt.cmdBuf[:nt.cmdBufLen], *nt.lastCmdCharPos).Data
}

func (nt *nterm) ReadInputs() {

	if input.KeyClicked(sdl.K_RETURN) || input.KeyClicked(sdl.K_KP_ENTER) {

		if nt.cmdBufLen > 0 {
			nt.cursorCharIndex = nt.cmdBufLen // This is so \n is written to the end of the cmdBuf
			nt.WriteToCmdBuf([]rune{'\n'})
			nt.HandleReturn()
		} else {
			nt.WriteToTextBuf([]byte{'\n'})
		}
	}

	// Cursor movement and scroll
	if input.KeyClicked(sdl.K_LEFT) {
		nt.cursorCharIndex = clamp(nt.cursorCharIndex-1, 0, nt.cmdBufLen)
	} else if input.KeyClicked(sdl.K_RIGHT) {
		nt.cursorCharIndex = clamp(nt.cursorCharIndex+1, 0, nt.cmdBufLen)
	}

	if input.KeyClicked(sdl.K_HOME) {
		nt.cursorCharIndex = 0
	} else if input.KeyClicked(sdl.K_END) {
		nt.cursorCharIndex = nt.cmdBufLen
	}

	if input.KeyDown(sdl.K_LCTRL) && input.KeyClicked(sdl.K_END) {

		charsPerLine, _ := nt.GridSize()
		nt.scrollPosRel = FindNLinesIndexIterator(nt.textBuf.Iterator(), nt.Lines.Iterator(), nt.textBuf.Len-1, -nt.scrollSpd, charsPerLine-1)
		nt.scrollPosRel = clamp(nt.scrollPosRel, int64(nt.textBuf.RelIndexFromWriteCount(nt.firstValidLine.StartIndex_WriteCount)), nt.textBuf.Len-1)

	} else if input.KeyDown(sdl.K_LCTRL) && input.KeyClicked(sdl.K_HOME) {
		nt.scrollPosRel = 0
	}

	if mouseWheelYNorm := -int64(input.GetMouseWheelYNorm()); mouseWheelYNorm != 0 {

		charsPerLine, _ := nt.GridSize()
		if mouseWheelYNorm < 0 {
			nt.scrollPosRel = FindNLinesIndexIterator(nt.textBuf.Iterator(), nt.Lines.Iterator(), nt.scrollPosRel, -nt.scrollSpd, charsPerLine-1)
		} else {
			nt.scrollPosRel = FindNLinesIndexIterator(nt.textBuf.Iterator(), nt.Lines.Iterator(), nt.scrollPosRel, nt.scrollSpd, charsPerLine-1)
		}

		nt.scrollPosRel = clamp(nt.scrollPosRel, int64(nt.textBuf.RelIndexFromWriteCount(nt.firstValidLine.StartIndex_WriteCount)), nt.textBuf.Len-1)
	}

	// Delete inputs
	// @TODO: Implement hold to delete
	if input.KeyClicked(sdl.K_BACKSPACE) {
		nt.DeletePrevChar()
	}

	if input.KeyClicked(sdl.K_DELETE) {
		nt.DeleteNextChar()
	}
}

func (p *nterm) DrawTextAnsiCodes(bs []byte, pos gglm.Vec3) gglm.Vec3 {

	currFgColor := p.Settings.DefaultFgColor
	currBgColor := p.Settings.DefaultBgColor

	draw := func(rs []rune) {

		p.GlyphRend.OptValues.BgColor.Data = currBgColor.Data

		startIndex := 0
		for i := 0; i < len(rs); i++ {

			r := rs[i]

			// @PERF We could probably use bytes.IndexByte here
			if r == '\n' {
				pos.Data = p.GlyphRend.DrawTextOpenGLAbsRectWithStartPos(rs[startIndex:i], &pos, gglm.NewVec3(0, 0, 0), gglm.NewVec2(float32(p.GlyphRend.ScreenWidth), 2*p.GlyphRend.Atlas.LineHeight), &currFgColor).Data
				pos.SetX(0)
				pos.AddY(-p.GlyphRend.Atlas.LineHeight)
				startIndex = i + 1
				continue
			}
		}

		if startIndex < len(rs) {
			pos.Data = p.GlyphRend.DrawTextOpenGLAbsRectWithStartPos(rs[startIndex:], &pos, gglm.NewVec3(0, 0, 0), gglm.NewVec2(float32(p.GlyphRend.ScreenWidth), 2*p.GlyphRend.Atlas.LineHeight), &currFgColor).Data
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
		if info.Options.HasOptions(ansi.AnsiCodeOptions_ColorFg) {

			if info.Info1.X() == -1 {
				currFgColor = p.Settings.DefaultFgColor
			} else {
				currFgColor = info.Info1
			}
		}

		if info.Options.HasOptions(ansi.AnsiCodeOptions_ColorBg) {
			if info.Info1.X() == -1 {
				currBgColor = p.Settings.DefaultBgColor
			} else {
				currBgColor = info.Info1
			}
		}

		// Advance beyond the code chars
		bs = bs[index+len(code):]
	}

	return pos
}

func (p *nterm) SyntaxHighlightAndDraw(text []rune, pos gglm.Vec3) gglm.Vec3 {

	startIndex := 0
	startPos := pos.Clone()
	currColor := &p.Settings.DefaultFgColor

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
			currColor = &p.Settings.DefaultFgColor

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
			currColor = &p.Settings.DefaultFgColor
		}
	}

	if startIndex < len(text) {
		if inDoubleString || inSingleString {
			pos.Data = p.GlyphRend.DrawTextOpenGLAbs(text[startIndex:], &pos, &p.Settings.StringColor).Data
		} else {
			pos.Data = p.GlyphRend.DrawTextOpenGLAbs(text[startIndex:], &pos, &p.Settings.DefaultFgColor).Data
		}
	}

	return pos
}

func (p *nterm) DeletePrevChar() {

	if p.cursorCharIndex == 0 || p.cmdBufLen == 0 {
		return
	}

	copy(p.cmdBuf[p.cursorCharIndex-1:], p.cmdBuf[p.cursorCharIndex:])

	p.cmdBufLen--
	p.cursorCharIndex--
}

func (p *nterm) DeleteNextChar() {

	if p.cmdBufLen == 0 || p.cursorCharIndex == p.cmdBufLen {
		return
	}

	copy(p.cmdBuf[p.cursorCharIndex:], p.cmdBuf[p.cursorCharIndex+1:])

	p.cmdBufLen--
}

func (p *nterm) HandleReturn() {

	cmdRunes := p.cmdBuf[:p.cmdBufLen]
	p.cmdBufLen = 0
	p.cursorCharIndex = 0

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
		buf := make([]byte, 4*1024)
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

			// @Todo We need to parse ansi codes as data is coming in to update the drawing settings (e.g. color)
			b := buf[:readBytes]
			p.WriteToTextBuf(b)
			// println("Read:", string(buf[:readBytes]))
		}
	}()

	//Stderr
	go func() {

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

func (p *nterm) ParseLines(bs []byte) {

	// @TODO We should virtually break lines when they are too long
	checkedBytes := uint64(0)
	for len(bs) > 0 {

		// IndexByte is assembly optimized for different platforms and is much faster than checking one byte at a time
		index := bytes.IndexByte(bs, '\n')
		if index == -1 {
			break
		}
		bs = bs[index+1:]

		checkedBytes += uint64(index + 1)
		p.LineBeingParsed.EndIndex_WriteCount = p.textBuf.WrittenElements + checkedBytes
		p.WriteLine(&p.LineBeingParsed)
		p.LineBeingParsed.StartIndex_WriteCount = p.textBuf.WrittenElements + checkedBytes
	}
}

func (p *nterm) WriteLine(l *Line) {
	assert.T(l.StartIndex_WriteCount <= l.EndIndex_WriteCount, "Invalid line: %+v\n", l)
	p.Lines.Write(*l)
}

func (p *nterm) ClearActiveCmd() {

	if p.activeCmd == nil {
		return
	}

	p.activeCmd = nil
}

func (p *nterm) DrawCursor() {

	//Position cursor by placing it at the end of the drawn characters then walking backwards
	pos := p.lastCmdCharPos.Clone()

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

// GridSize returns how many cells horizontally (aka chars per line) and how many cells vertically (aka lines)
func (p *nterm) GridSize() (w, h int64) {
	w = int64(p.GlyphRend.ScreenWidth) / int64(p.GlyphRend.Atlas.SpaceAdvance)
	h = int64(p.GlyphRend.ScreenHeight) / int64(p.GlyphRend.Atlas.LineHeight)
	return w, h
}

func (p *nterm) ScreenPosToGridPos(screenPos *gglm.Vec3) {
	screenPos.SetX(FloorF32(screenPos.X() / p.GlyphRend.Atlas.SpaceAdvance))
	screenPos.SetY(FloorF32(screenPos.Y() / p.GlyphRend.Atlas.LineHeight))
}

func (p *nterm) DebugUpdate() {

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

func (p *nterm) Render() {

	defer p.GlyphRend.Draw()

	if consts.Mode_Debug {
		p.DebugRender()

		sizeX := float32(p.GlyphRend.ScreenWidth)
		p.rend.Draw(p.gridMesh, gglm.NewTrMatId().Translate(gglm.NewVec3(sizeX/2, p.SepLinePos.Y(), 0)).Scale(gglm.NewVec3(sizeX, 1, 1)), p.gridMat)
	}

	p.DrawCursor()
}

func (p *nterm) DebugRender() {

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
				p.GlyphRend.DrawTextOpenGLAbsString(str, gglm.NewVec3(xOff, float32(p.GlyphRend.Atlas.LineHeight)*5+yOff, 0), &p.Settings.DefaultFgColor)
			}
			p.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps, " Draws/f: ", math.Ceil(charsPerFrame/glyphs.DefaultGlyphsPerBatch), " chars/f: ", charsPerFrame, " chars/s: ", fps*charsPerFrame))
		} else {
			charsPerFrame := float64(charCount)
			p.GlyphRend.DrawTextOpenGLAbsString(str, gglm.NewVec3(xOff, float32(p.GlyphRend.Atlas.LineHeight)*5+yOff, 0), &p.Settings.DefaultFgColor)
			p.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps, " Draws/f: ", math.Ceil(charsPerFrame/glyphs.DefaultGlyphsPerBatch), " chars/f: ", int(charsPerFrame), " chars/s: ", fps*int(charsPerFrame)))
		}
	} else {
		p.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps))
	}
}

func (p *nterm) DrawGrid() {

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

func (p *nterm) FrameEnd() {
	assert.T(p.cursorCharIndex <= p.cmdBufLen, "Cursor char index is larger than cmdBufLen! You probablly forgot to move/reset the cursor index along with the buffer length somewhere. Cursor=%d, cmdBufLen=%d\n", p.cursorCharIndex, p.cmdBufLen)

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

func (p *nterm) DeInit() {
}

func (p *nterm) HandleWindowResize() {
	w, h := p.win.SDLWin.GetSize()
	p.GlyphRend.SetScreenSize(w, h)

	projMtx := gglm.Ortho(0, float32(w), float32(h), 0, 0.1, 20)
	viewMtx := gglm.LookAt(gglm.NewVec3(0, 0, -10), gglm.NewVec3(0, 0, 0), gglm.NewVec3(0, 1, 0))
	p.gridMat.SetUnifMat4("projViewMat", &projMtx.Mul(viewMtx).Mat4)

	p.CellCountX, p.CellCountY = p.GridSize()
	p.CellCount = p.CellCountX * p.CellCountY
}

func (p *nterm) WriteToTextBuf(text []byte) {
	// This is locked because running cmds are potentially writing to it same time we are
	p.textBufMutex.Lock()

	p.ParseLines(text)
	p.textBuf.Write(text...)

	p.textBufMutex.Unlock()
}

func (p *nterm) WriteToCmdBuf(text []rune) {

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

// FindNLinesIndexIterator starts at startIndex and moves n lines forward/backward, depending on whether 'n' is negative or positive,
// then returns the starting index of the nth line.
//
// A line is counted when either a '\n' is seen or by seeing enough chars that a wrap is required.
func FindNLinesIndexIterator(it ring.Iterator[byte], lineIt ring.Iterator[Line], startIndex, n, charsPerLine int64) (newIndex int64) {

	done := false
	read := 0
	bytesToKeep := 0
	buf := make([]byte, 4)
	it.GotoIndex(startIndex)

	// @Todo we should ignore zero width glyphs
	// @Note is this better in glyphs package?
	bytesSeen := int64(0)
	charsSeenThisLine := int64(0)

	if n >= 0 {

		// If nothing changes (e.g. already at end of iterator) then we will stay at the same place
		newIndex = startIndex

		for !done || bytesToKeep > 0 {

			read, done = it.NextN(buf[bytesToKeep:], 4)

			r, size := utf8.DecodeRune(buf[:bytesToKeep+read])
			bytesToKeep += read - size
			copy(buf, buf[size:size+bytesToKeep])

			charsSeenThisLine++
			bytesSeen += int64(size)

			// If this is true we covered one line
			if charsSeenThisLine > charsPerLine || r == '\n' {

				newIndex = startIndex + bytesSeen

				// Don't stop at newlines, but the char right after
				if charsSeenThisLine > charsPerLine {
					charsSeenThisLine = 0
				} else {
					charsSeenThisLine = 0
				}

				n--
				if n <= 0 {
					break
				}
			}
		}

	} else {

		// If on the empty line between non-empty lines we want to know where the last char of the previous
		// line is so we can take into account position differences with wrapping
		startIndexByte := it.Buf.Get(uint64(startIndex))
		startMinusOneIndexByte := it.Buf.Get(uint64(startIndex - 1))
		if startIndexByte == '\n' {

			if startMinusOneIndexByte == '\n' {

				charsIntoLine := getCharGridPosX(it.Buf.Iterator(), lineIt, clamp(startIndex-2, 0, it.Buf.Len-1), charsPerLine)
				if charsIntoLine > 0 {
					charsSeenThisLine = charsPerLine - charsIntoLine
				}
			}
		}

		// Skip the extra new line so the decoder starts with normal characters instead of seeing a newline
		// and immediately quitting
		if startIndexByte == '\n' || startMinusOneIndexByte == '\n' {
			it.Prev()
		}

		for !done || bytesToKeep > 0 {

			read, done = it.PrevN(buf[bytesToKeep:], 4)

			r, size := utf8.DecodeRune(buf[:bytesToKeep+read])
			bytesToKeep += read - size
			copy(buf, buf[size:size+bytesToKeep])

			charsSeenThisLine++
			bytesSeen += int64(size)

			// If this is true we covered one line
			if charsSeenThisLine > charsPerLine || r == '\n' {

				newIndex = startIndex - bytesSeen

				n++
				if n >= 0 {
					break
				}

				charsSeenThisLine = 1
			}
		}

		// If we reached beginning of buffer before finding a new line then newIndex is zero
		if startIndex-bytesSeen == 0 {
			newIndex = 0
		}
	}

	return newIndex
}

// getCharGridPosX returns the dispaly grid's X position of the char at textBufStartIndexRel.
// Wrapping is respected so if the char is at the end of a long line it's position will take that into consideration
func getCharGridPosX(it ring.Iterator[byte], lineIt ring.Iterator[Line], textBufStartIndexRel, charsPerLine int64) int64 {

	// Find line that contains the start index
	line, _ := GetLineFromTextBufIndex(it, lineIt, uint64(textBufStartIndexRel))
	if line == nil {
		return 0
	}

	// This doesn't consider non-printing chars for wrapping, but should be good enough
	v1, v2 := it.Buf.ViewsFromToRelIndex(it.Buf.RelIndexFromWriteCount(line.StartIndex_WriteCount+1), uint64(textBufStartIndexRel))

	// Limit runes we count to maxLineLookBack so we don't spend too much time here.
	// All this is just so we position the last part of a wrapped line corrrectly when the full wrapped line
	// is visible. But in a super long line like this the bottom part will never be in view at the same time as the
	// start of the line, and so it doesn't matter that it's crazy accurate, the user will never see it.

	lenV1 := int64(len(v1))
	lenV2 := int64(len(v2))
	lineLen := lenV1 + lenV2
	const maxLineLookBack = 8 * 1024
	if lineLen > maxLineLookBack {

		extraLen := lineLen - maxLineLookBack
		if extraLen <= lenV1 {
			v1 = v1[extraLen:]
		} else {
			v1 = v1[lenV1:]
			v2 = v2[extraLen-lenV1:]
		}
	}

	runeCount := utf8.RuneCount(v1)
	runeCount += utf8.RuneCount(v2)
	lastCharGridPosX := runeCount % int(charsPerLine+1)
	return int64(lastCharGridPosX)
}

func PrintLine(textBuf *ring.Buffer[byte], p *Line) {

	if !IsLineValid(textBuf, p) {
		return
	}

	v1, v2 := textBuf.ViewsFromToWriteCount(p.StartIndex_WriteCount, p.EndIndex_WriteCount)
	fmt.Println(string(v1) + string(v2))
}

func GetLineFromTextBufIndex(it ring.Iterator[byte], lineIt ring.Iterator[Line], textBufStartIndexRel uint64) (outLine *Line, pIndex uint64) {

	if lineIt.Buf.Len == 0 {
		return
	}

	// Find first valid line
	lineIt.GotoStart()
	for p, done := lineIt.NextPtr(); !done; p, done = lineIt.NextPtr() {

		if !IsLineValid(it.Buf, p) {
			continue
		}

		lineIt.Prev()
		break
	}

	// Binary search for the line
	lowIndexRel := lineIt.CurrToRelIndex()
	highIndexRel := uint64(lineIt.Buf.Len)
	for lowIndexRel <= highIndexRel {

		medianIndexRel := (lowIndexRel + highIndexRel) / 2
		p := lineIt.Buf.GetPtr(medianIndexRel)

		startIndexRel := it.Buf.RelIndexFromWriteCount(p.StartIndex_WriteCount)
		endIndexRel := it.Buf.RelIndexFromWriteCount(p.EndIndex_WriteCount)

		if textBufStartIndexRel < startIndexRel {
			highIndexRel = medianIndexRel - 1
		} else if textBufStartIndexRel > endIndexRel {
			lowIndexRel = medianIndexRel + 1
		} else {
			outLine = p
			pIndex = medianIndexRel
			break
		}
	}

	if outLine == nil {
		panic(fmt.Sprintf("Could not find line for text buffer relative index %d", textBufStartIndexRel))
	}

	return outLine, pIndex
}

type LineStatus byte

const (
	LineStatus_Unknown LineStatus = iota
	LineStatus_Valid
	// LineStatus_PartiallyInvalid is when the start index is invalid but the end index is in a valid position
	LineStatus_PartiallyInvalid
	LineStatus_Invalid
)

// IsLineValid returns true only if the status is LineStatus_Valid
func IsLineValid(textBuf *ring.Buffer[byte], p *Line) bool {
	isValid := textBuf.WrittenElements-p.StartIndex_WriteCount < uint64(textBuf.Cap)
	return isValid
}

func getLineStatus(textBuf *ring.Buffer[byte], p *Line) LineStatus {

	startValid := textBuf.WrittenElements-p.StartIndex_WriteCount < uint64(textBuf.Cap)
	if startValid {
		return LineStatus_Valid
	}

	endValid := textBuf.WrittenElements-p.EndIndex_WriteCount < uint64(textBuf.Cap)
	if endValid {
		return LineStatus_PartiallyInvalid
	}

	return LineStatus_Invalid
}
