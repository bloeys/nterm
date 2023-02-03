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
	"github.com/bloeys/nmage/camera"
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

	glyphGrid *GlyphGrid

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

// @TODO: We should 'draw' and apply ansi operations on an in-mem grid and send the final grid for rendering

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
		FontSize:  24,

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
		var pf, _ = os.Create("cpu.pprof")
		defer pf.Close()
		pprof.StartCPUProfile(pf)
	}

	engine.Run(p, p.win, p.imguiInfo)

	if consts.Mode_Debug {
		pprof.StopCPUProfile()

		var heapProfile, _ = os.Create("heap.pprof")
		defer heapProfile.Close()
		pprof.WriteHeapProfile(heapProfile)
	}
}

func (nt *nterm) handleSDLEvent(e sdl.Event) {

	switch e := e.(type) {

	case *sdl.TextInputEvent:
		nt.WriteToCmdBuf([]rune(e.GetText()))
	case *sdl.WindowEvent:
		if e.Event == sdl.WINDOWEVENT_SIZE_CHANGED {
			nt.HandleWindowResize()
		}
	}
}

func (nt *nterm) Init() {

	dpi, _, _, err := sdl.GetDisplayDPI(0)
	if err != nil {
		panic("Failed to get display DPI. Err: " + err.Error())
	}
	fmt.Printf("DPI: %f, font size: %d\n", dpi, nt.FontSize)

	w, h := nt.win.SDLWin.GetSize()
	// p.GlyphRend, err = glyphs.NewGlyphRend("./res/fonts/tajawal-regular-var.ttf", &truetype.Options{Size: float64(p.FontSize), DPI: p.Dpi, SubPixelsX: subPixelX, SubPixelsY: subPixelY, Hinting: hinting}, w, h)
	// nt.GlyphRend, err = glyphs.NewGlyphRend("./res/fonts/alm-fixed.ttf", &truetype.Options{Size: float64(nt.FontSize), DPI: nt.Dpi, SubPixelsX: subPixelX, SubPixelsY: subPixelY, Hinting: hinting}, w, h)
	nt.GlyphRend, err = glyphs.NewGlyphRend("./res/fonts/CascadiaMono-Regular.ttf", &truetype.Options{Size: float64(nt.FontSize), DPI: nt.Dpi, SubPixelsX: subPixelX, SubPixelsY: subPixelY, Hinting: hinting}, w, h)
	if err != nil {
		panic("Failed to create atlas from font file. Err: " + err.Error())
	}

	nt.GlyphRend.OptValues.BgColor = &nt.Settings.DefaultBgColor
	nt.GlyphRend.SetOpts(glyphs.GlyphRendOpt_BgColor)

	// if consts.Mode_Debug {
	// glyphs.SaveImgToPNG(p.GlyphRend.Atlas.Img, "./debug-atlas.png")
	// }

	//Load resources
	nt.gridMesh, err = meshes.NewMesh("grid", "./res/models/quad.obj", 0)
	if err != nil {
		panic(err.Error())
	}

	nt.gridMat = materials.NewMaterial("grid", "./res/shaders/grid.glsl")
	nt.HandleWindowResize()

	// Set initial cursor pos
	nt.lastCmdCharPos.SetY(nt.GlyphRend.Atlas.LineHeight)

	// Init glyph grid
	gridWidth, gridHeight := nt.GridSize()
	nt.glyphGrid = NewGlyphGrid(uint(gridWidth), uint(gridHeight))
}

func (nt *nterm) Update() {

	nt.frameStartTime = time.Now()

	if input.IsQuitClicked() || input.KeyClicked(sdl.K_ESCAPE) {
		engine.Quit()
	}

	if consts.Mode_Debug {
		nt.DebugUpdate()
	}

	//Font sizing
	oldFontSize := nt.FontSize
	fontSizeChanged := false
	if input.KeyClicked(sdl.K_KP_PLUS) {
		nt.FontSize += 2
		fontSizeChanged = true
	} else if input.KeyClicked(sdl.K_KP_MINUS) {
		nt.FontSize -= 2
		fontSizeChanged = true
	}

	if fontSizeChanged {

		err := nt.GlyphRend.SetFace(&truetype.Options{Size: float64(nt.FontSize), DPI: nt.Dpi, SubPixelsX: subPixelX, SubPixelsY: subPixelY, Hinting: hinting})
		if err != nil {
			nt.FontSize = oldFontSize
			fmt.Println("Failed to update font face. Err: " + err.Error())
		} else {
			glyphs.SaveImgToPNG(nt.GlyphRend.Atlas.Img, "./debug-atlas.png")
			gridWidth, gridHeight := nt.GridSize()
			nt.glyphGrid = NewGlyphGrid(uint(gridWidth), uint(gridHeight))
			fmt.Println("New font size:", nt.FontSize, "; New texture size:", nt.GlyphRend.Atlas.Img.Rect.Max.X)
		}
	}

	nt.MainUpdate()
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

	// Since we have more chars than lines the first line might not start
	// at the first char but midway in the buffer, so we ensure that scrollPosRel
	// starts at the first line
	firstValidLineStartIndexRel := int64(nt.textBuf.RelIndexFromWriteCount(nt.firstValidLine.StartIndex_WriteCount))
	if nt.scrollPosRel < firstValidLineStartIndexRel {
		nt.scrollPosRel = firstValidLineStartIndexRel
	}

	nt.ReadInputs()

	// Line separator
	nt.SepLinePos.SetY(2 * nt.GlyphRend.Atlas.LineHeight)

	// Draw textBuf
	nt.glyphGrid.ClearAll()
	nt.glyphGrid.SetCursor(0, 0)

	gw, gh := nt.GridSize()
	v1, v2 := nt.textBuf.ViewsFromToRelIndex(uint64(nt.scrollPosRel), uint64(nt.scrollPosRel)+uint64(gw*gh))

	nt.DrawTextAnsiCodesOnGlyphGrid(v1)
	nt.DrawTextAnsiCodesOnGlyphGrid(v2)
	nt.glyphGrid.Write(nt.cmdBuf[:nt.cmdBufLen], &nt.Settings.DefaultFgColor, &nt.Settings.DefaultBgColor)

	nt.DrawGlyphGrid()

	if input.KeyClicked(sdl.K_F4) {
		nt.glyphGrid.Print()
		println(nt.glyphGrid.SizeX, nt.glyphGrid.SizeY)
	}
}

func (nt *nterm) DrawGlyphGrid() {

	top := float32(nt.GlyphRend.ScreenHeight) - nt.GlyphRend.Atlas.LineHeight
	nt.lastCmdCharPos.Data = gglm.NewVec3(0, top, 0).Data

	for y := 0; y < len(nt.glyphGrid.Tiles); y++ {

		row := nt.glyphGrid.Tiles[y]

		for x := 0; x < len(row); x++ {

			g := &row[x]
			if g.Glyph == utf8.RuneError {
				continue
			}

			nt.GlyphRend.OptValues.BgColor.Data = g.BgColor.Data
			nt.lastCmdCharPos.Data = nt.GlyphRend.DrawTextOpenGLAbsRectWithStartPos([]rune{g.Glyph}, nt.lastCmdCharPos, gglm.NewVec3(0, top, 0), gglm.NewVec2(float32(nt.GlyphRend.ScreenWidth), nt.GlyphRend.Atlas.LineHeight), &g.FgColor).Data
		}
	}
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

func (nt *nterm) DrawTextAnsiCodesOnGlyphGrid(bs []byte) {

	// @TODO: We should remember color state even if the ansi codes are out of view
	currFgColor := nt.Settings.DefaultFgColor
	currBgColor := nt.Settings.DefaultBgColor

	draw := func(rs []rune) {
		nt.glyphGrid.Write(rs, &currFgColor, &currBgColor)
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

		//Apply codes
		ansiCodeInfo := ansi.InfoFromAnsiCode(code)
		// fmt.Printf("Info: %+v\n", ansiCodeInfo)
		for i := 0; i < len(ansiCodeInfo.Payload); i++ {

			payload := &ansiCodeInfo.Payload[i]

			if payload.Type.HasOption(ansi.AnsiCodePayloadType_Reset) {
				currFgColor = nt.Settings.DefaultFgColor
				currBgColor = nt.Settings.DefaultBgColor
				break
			}

			if payload.Type.HasOption(ansi.AnsiCodePayloadType_ColorFg) {
				currFgColor = payload.Info
			} else if payload.Type.HasOption(ansi.AnsiCodePayloadType_ColorBg) {
				currBgColor = payload.Info
			}
		}

		// Advance beyond the code chars
		bs = bs[index+len(code):]
	}
}

// @TODO: Rewrite to draw on glyph grid
func (nt *nterm) SyntaxHighlightAndDraw(text []rune, pos gglm.Vec3) gglm.Vec3 {

	startIndex := 0
	startPos := pos.Clone()
	currColor := &nt.Settings.DefaultFgColor

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
			pos.Data = nt.GlyphRend.DrawTextOpenGLAbs(text[startIndex:i], &pos, currColor).Data
			pos.SetX(startPos.X())
			pos.AddY(-nt.GlyphRend.Atlas.LineHeight)
			startIndex = i + 1
			continue

		case '"':

			if inSingleString {
				continue
			}

			if !inDoubleString {
				pos.Data = nt.GlyphRend.DrawTextOpenGLAbs(text[startIndex:i], &pos, currColor).Data

				startIndex = i
				inDoubleString = true
				currColor = &nt.Settings.StringColor
				continue
			}

			pos.Data = nt.GlyphRend.DrawTextOpenGLAbs(text[startIndex:i+1], &pos, currColor).Data
			startIndex = i + 1
			inDoubleString = false
			currColor = &nt.Settings.DefaultFgColor

		case '\'':
			if inDoubleString {
				continue
			}

			if !inSingleString {
				pos.Data = nt.GlyphRend.DrawTextOpenGLAbs(text[startIndex:i], &pos, currColor).Data

				startIndex = i
				inSingleString = true
				currColor = &nt.Settings.StringColor
				continue
			}

			pos.Data = nt.GlyphRend.DrawTextOpenGLAbs(text[startIndex:i+1], &pos, &nt.Settings.StringColor).Data
			startIndex = i + 1
			inSingleString = false
			currColor = &nt.Settings.DefaultFgColor
		}
	}

	if startIndex < len(text) {
		if inDoubleString || inSingleString {
			pos.Data = nt.GlyphRend.DrawTextOpenGLAbs(text[startIndex:], &pos, &nt.Settings.StringColor).Data
		} else {
			pos.Data = nt.GlyphRend.DrawTextOpenGLAbs(text[startIndex:], &pos, &nt.Settings.DefaultFgColor).Data
		}
	}

	return pos
}

func (nt *nterm) DeletePrevChar() {

	if nt.cursorCharIndex == 0 || nt.cmdBufLen == 0 {
		return
	}

	copy(nt.cmdBuf[nt.cursorCharIndex-1:], nt.cmdBuf[nt.cursorCharIndex:])

	nt.cmdBufLen--
	nt.cursorCharIndex--
}

func (nt *nterm) DeleteNextChar() {

	if nt.cmdBufLen == 0 || nt.cursorCharIndex == nt.cmdBufLen {
		return
	}

	copy(nt.cmdBuf[nt.cursorCharIndex:], nt.cmdBuf[nt.cursorCharIndex+1:])

	nt.cmdBufLen--
}

func (nt *nterm) HandleReturn() {

	cmdRunes := nt.cmdBuf[:nt.cmdBufLen]
	nt.cmdBufLen = 0
	nt.cursorCharIndex = 0

	cmdStr := string(cmdRunes)
	cmdBytes := []byte(cmdStr)
	nt.WriteToTextBuf(cmdBytes)

	if nt.activeCmd != nil {

		// println("Wrote:", string(cmdBytes))
		_, err := nt.activeCmd.Stdin.Write(cmdBytes)
		if err != nil {
			nt.WriteToTextBuf([]byte(fmt.Sprintf("Writing to stdin pipe of '%s' failed. Error: %s\n", nt.activeCmd.C.Path, err.Error())))
			nt.ClearActiveCmd()
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
		nt.WriteToTextBuf([]byte(fmt.Sprintf("Creating stdout pipe of '%s' failed. Error: %s\n", cmdName, err.Error())))
		return
	}

	inPipe, err := cmd.StdinPipe()
	if err != nil {
		nt.WriteToTextBuf([]byte(fmt.Sprintf("Creating stdin pipe of '%s' failed. Error: %s\n", cmdName, err.Error())))
		return
	}

	errPipe, err := cmd.StderrPipe()
	if err != nil {
		nt.WriteToTextBuf([]byte(fmt.Sprintf("Creating stderr pipe of '%s' failed. Error: %s\n", cmdName, err.Error())))
		return
	}

	startTime := time.Now()
	err = cmd.Start()
	if err != nil {
		nt.WriteToTextBuf([]byte(fmt.Sprintf("Running '%s' failed. Error: %s\n", cmdName, err.Error())))
		return
	}
	nt.activeCmd = &Cmd{
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

		defer nt.ClearActiveCmd()
		buf := make([]byte, 4*1024)
		for nt.activeCmd != nil {

			readBytes, err := nt.activeCmd.Stdout.Read(buf)
			if err != nil {

				if err == io.EOF {
					break
				}

				nt.WriteToTextBuf([]byte("Stdout pipe failed. Error: " + err.Error()))
				return
			}

			if readBytes == 0 {
				continue
			}

			// @Todo We need to parse ansi codes as data is coming in to update the drawing settings (e.g. color)
			b := buf[:readBytes]
			nt.WriteToTextBuf(b)
			// println("Read:", string(buf[:readBytes]))
		}
	}()

	//Stderr
	go func() {

		buf := make([]byte, 1024)
		for nt.activeCmd != nil {

			readBytes, err := nt.activeCmd.Stderr.Read(buf)
			if err != nil {

				if err == io.EOF {
					break
				}

				nt.WriteToTextBuf([]byte("Stderr pipe failed. Error: " + err.Error()))
				return
			}

			if readBytes == 0 {
				continue
			}

			nt.WriteToTextBuf(buf[:readBytes])
		}
	}()
}

func (nt *nterm) ParseLines(bs []byte) {

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
		nt.LineBeingParsed.EndIndex_WriteCount = nt.textBuf.WrittenElements + checkedBytes
		nt.WriteLine(&nt.LineBeingParsed)
		nt.LineBeingParsed.StartIndex_WriteCount = nt.textBuf.WrittenElements + checkedBytes
	}
}

func (nt *nterm) WriteLine(l *Line) {
	assert.T(l.StartIndex_WriteCount <= l.EndIndex_WriteCount, "Invalid line: %+v\n", l)
	nt.Lines.Write(*l)
}

func (nt *nterm) ClearActiveCmd() {

	if nt.activeCmd == nil {
		return
	}

	nt.activeCmd = nil
}

func (nt *nterm) DrawCursor() {

	//Position cursor by placing it at the end of the drawn characters then walking backwards
	pos := nt.lastCmdCharPos.Clone()

	pos.AddY(nt.GlyphRend.Atlas.LineHeight * 0.5)
	for i := clamp(nt.cmdBufLen, 0, int64(len(nt.cmdBuf))); i > nt.cursorCharIndex; i-- {

		if nt.cmdBuf[i] == '\n' {
			pos.AddY(nt.GlyphRend.Atlas.LineHeight)
			continue
		}
		pos.AddX(-nt.GlyphRend.Atlas.SpaceAdvance)
	}

	nt.rend.Draw(nt.gridMesh, gglm.NewTrMatId().Translate(pos).Scale(gglm.NewVec3(0.1*nt.GlyphRend.Atlas.SpaceAdvance, nt.GlyphRend.Atlas.LineHeight, 1)), nt.gridMat)
}

// GridSize returns how many cells horizontally (aka chars per line) and how many cells vertically (aka lines)
func (nt *nterm) GridSize() (w, h int64) {
	w = int64(nt.GlyphRend.ScreenWidth) / int64(nt.GlyphRend.Atlas.SpaceAdvance)
	h = int64(nt.GlyphRend.ScreenHeight) / int64(nt.GlyphRend.Atlas.LineHeight)
	return w, h
}

func (nt *nterm) ScreenPosToGridPos(screenPos *gglm.Vec3) {
	screenPos.SetX(FloorF32(screenPos.X() / nt.GlyphRend.Atlas.SpaceAdvance))
	screenPos.SetY(FloorF32(screenPos.Y() / nt.GlyphRend.Atlas.LineHeight))
}

func (nt *nterm) DebugUpdate() {

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

func (nt *nterm) Render() {

	defer nt.GlyphRend.Draw()

	if consts.Mode_Debug {
		nt.DebugRender()

		sizeX := float32(nt.GlyphRend.ScreenWidth)
		nt.rend.Draw(nt.gridMesh, gglm.NewTrMatId().Translate(gglm.NewVec3(sizeX/2, nt.SepLinePos.Y(), 0)).Scale(gglm.NewVec3(sizeX, 1, 1)), nt.gridMat)
	}

	nt.DrawCursor()
}

func (nt *nterm) DebugRender() {

	if drawGrid {
		nt.DrawGrid()
	}

	fps := int(timing.GetAvgFPS())
	if len(textToShow) > 0 {
		str := textToShow
		charCount := len([]rune(str))
		if drawManyLines {
			const charsPerFrame = 500_000
			for i := 0; i < charsPerFrame/charCount; i++ {
				nt.GlyphRend.DrawTextOpenGLAbsString(str, gglm.NewVec3(xOff, float32(nt.GlyphRend.Atlas.LineHeight)*5+yOff, 0), &nt.Settings.DefaultFgColor)
			}
			nt.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps, " Draws/f: ", math.Ceil(charsPerFrame/glyphs.DefaultGlyphsPerBatch), " chars/f: ", charsPerFrame, " chars/s: ", fps*charsPerFrame))
		} else {
			charsPerFrame := float64(charCount)
			nt.GlyphRend.DrawTextOpenGLAbsString(str, gglm.NewVec3(xOff, float32(nt.GlyphRend.Atlas.LineHeight)*5+yOff, 0), &nt.Settings.DefaultFgColor)
			nt.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps, " Draws/f: ", math.Ceil(charsPerFrame/glyphs.DefaultGlyphsPerBatch), " chars/f: ", int(charsPerFrame), " chars/s: ", fps*int(charsPerFrame)))
		}
	} else {
		nt.win.SDLWin.SetTitle(fmt.Sprint("FPS: ", fps))
	}
}

func (nt *nterm) DrawGrid() {

	sizeX := float32(nt.GlyphRend.ScreenWidth)
	sizeY := float32(nt.GlyphRend.ScreenHeight)

	//columns
	adv := nt.GlyphRend.Atlas.SpaceAdvance
	for i := 0; i < int(nt.GlyphRend.ScreenWidth); i++ {
		nt.rend.Draw(nt.gridMesh, gglm.NewTrMatId().Translate(gglm.NewVec3(adv*float32(i), sizeY/2, 0)).Scale(gglm.NewVec3(1, sizeY, 1)), nt.gridMat)
	}

	//rows
	for i := int32(0); i < nt.GlyphRend.ScreenHeight; i += int32(nt.GlyphRend.Atlas.LineHeight) {
		nt.rend.Draw(nt.gridMesh, gglm.NewTrMatId().Translate(gglm.NewVec3(sizeX/2, float32(i), 0)).Scale(gglm.NewVec3(sizeX, 1, 1)), nt.gridMat)
	}
}

func (nt *nterm) FrameEnd() {
	assert.T(nt.cursorCharIndex <= nt.cmdBufLen, "Cursor char index is larger than cmdBufLen! You probablly forgot to move/reset the cursor index along with the buffer length somewhere. Cursor=%d, cmdBufLen=%d\n", nt.cursorCharIndex, nt.cmdBufLen)

	if nt.Settings.LimitFps {

		elapsed := time.Since(nt.frameStartTime)
		microSecondsPerFrame := int64(1 / float32(nt.Settings.MaxFps) * 1000_000)

		// Sleep time is reduced by a millisecond to compensate for the (nearly) inevitable over-sleeping that will happen.
		timeToSleep := time.Duration((microSecondsPerFrame - elapsed.Microseconds()) * 1000)
		timeToSleep -= 1000 * time.Microsecond

		if timeToSleep.Milliseconds() > 0 {
			time.Sleep(timeToSleep)
		}
	}
}

func (nt *nterm) DeInit() {
}

func (nt *nterm) HandleWindowResize() {
	w, h := nt.win.SDLWin.GetSize()
	nt.GlyphRend.SetScreenSize(w, h)

	cam := camera.NewOrthographic(gglm.NewVec3(0, 0, 10), gglm.NewVec3(0, 0, -1), gglm.NewVec3(0, 1, 0), 0.1, 20, 0, float32(w), float32(h), 0)
	projViewMtx := cam.ProjMat.Mul(&cam.ViewMat)
	nt.gridMat.SetUnifMat4("projViewMat", projViewMtx)
}

func (nt *nterm) WriteToTextBuf(text []byte) {
	// This is locked because running cmds are potentially writing to it same time we are
	nt.textBufMutex.Lock()

	nt.ParseLines(text)
	nt.textBuf.Write(text...)

	nt.textBufMutex.Unlock()
}

func (nt *nterm) WriteToCmdBuf(text []rune) {

	delta := int64(len(text))
	newHeadPos := nt.cmdBufLen + delta
	if newHeadPos <= defaultCmdBufSize {

		copy(nt.cmdBuf[nt.cursorCharIndex+delta:], nt.cmdBuf[nt.cursorCharIndex:])
		copy(nt.cmdBuf[nt.cursorCharIndex:], text)

		nt.cursorCharIndex += delta
		nt.cmdBufLen = newHeadPos
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
