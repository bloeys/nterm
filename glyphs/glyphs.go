package glyphs

import (
	"fmt"
	"math"
	"unicode"

	"github.com/bloeys/gglm/gglm"
	"github.com/bloeys/nmage/assets"
	"github.com/bloeys/nmage/buffers"
	"github.com/bloeys/nmage/camera"
	"github.com/bloeys/nmage/materials"
	"github.com/bloeys/nmage/meshes"
	"github.com/bloeys/nterm/assert"
	"github.com/bloeys/nterm/consts"
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/golang/freetype/truetype"
)

const (
	DefaultGlyphsPerBatch = 4 * 1024

	floatsPerGlyph = 13
	invalidRune    = unicode.ReplacementChar
)

var (
	RuneInfos map[rune]RuneInfo
)

type GlyphRendOpt uint64

const (
	GlyphRendOpt_None    GlyphRendOpt = 0
	GlyphRendOpt_BgColor GlyphRendOpt = 1 << (iota - 1)
	GlyphRendOpt_Underline
	GlyphRendOpt_COUNT GlyphRendOpt = iota
)

type GlyphRendOptValues struct {
	BgColor *gglm.Vec4
}

type GlyphRend struct {
	Atlas    *FontAtlas
	AtlasTex *assets.Texture

	GlyphMesh           *meshes.Mesh
	GlyphFgInstancedBuf buffers.Buffer
	GlyphBgInstancedBuf buffers.Buffer
	GlyphMat            *materials.Material
	TextRunsBuf         []TextRun

	//Luckily slices still work with go-opengl, so for now we will use our slice as an array (no appending)
	GlyphFgCount uint32
	GlyphFgVBO   []float32

	GlyphBgCount uint32
	GlyphBgVBO   []float32

	ScreenWidth  int32
	ScreenHeight int32

	SpacesPerTab uint

	Opts      GlyphRendOpt
	OptValues GlyphRendOptValues
}

func (gr *GlyphRend) SetOpts(opts ...GlyphRendOpt) {

	for _, v := range opts {
		assert.T(v <= (1<<(GlyphRendOpt_COUNT-2)), "Invalid opts of value %d", opts)

		if v == GlyphRendOpt_None {
			gr.Opts = GlyphRendOpt_None
		} else {
			gr.Opts |= v
		}
	}

	gl.ProgramUniform1ui(gr.GlyphMat.ShaderProg.ID, gr.GlyphMat.GetUnifLoc("opts1"), uint32(gr.Opts))
}

func (gr *GlyphRend) HasOpt(opt GlyphRendOpt) bool {
	return gr.Opts&opt != 0
}

// DrawTextOpenGLAbs prepares text that will be drawn on the next GlyphRend.Draw call.
// screenPos is in the range [0,1], where (0,0) is the bottom left.
// Color is RGBA in the range [0,1].
func (gr *GlyphRend) DrawTextOpenGL01String(text string, screenPos *gglm.Vec3, color *gglm.Vec4) gglm.Vec3 {
	screenPos.Set(screenPos.X()*float32(gr.ScreenWidth), screenPos.Y()*float32(gr.ScreenHeight), screenPos.Z())
	return gr.DrawTextOpenGLAbsString(text, screenPos, color)
}

// DrawTextOpenGLAbsString prepares text that will be drawn on the next GlyphRend.Draw call.
// screenPos is in the range ([0,ScreenWidth],[0,ScreenHeight]) where (0,0) is bottom left.
// Color is RGBA in the range [0,1].
func (gr *GlyphRend) DrawTextOpenGLAbsString(text string, screenPos *gglm.Vec3, color *gglm.Vec4) gglm.Vec3 {
	return gr.DrawTextOpenGLAbs([]rune(text), screenPos, color)
}

func (gr *GlyphRend) DrawTextOpenGL01(text []rune, screenPos *gglm.Vec3, color *gglm.Vec4) gglm.Vec3 {
	screenPos.Set(screenPos.X()*float32(gr.ScreenWidth), screenPos.Y()*float32(gr.ScreenHeight), screenPos.Z())
	return gr.DrawTextOpenGLAbs(text, screenPos, color)
}

// DrawTextOpenGLAbsString prepares text that will be drawn on the next GlyphRend.Draw call.
// screenPos is in the range ([0,ScreenWidth],[0,ScreenHeight]) where (0,0) is bottom left.
// Color is RGBA in the range [0,1].
func (gr *GlyphRend) DrawTextOpenGLAbs(text []rune, startPos *gglm.Vec3, color *gglm.Vec4) gglm.Vec3 {

	runs := gr.TextRunsBuf[:]
	gr.GetTextRuns(text, &runs)
	if runs == nil {
		return *startPos
	}

	drawPos := startPos.Clone()
	lineHeightF32 := float32(gr.Atlas.LineHeight)
	fgBufIndex, bgBufIndex := gr.getFgAndBgBufIndices()
	for runIndex := 0; runIndex < len(runs); runIndex++ {

		run := &runs[runIndex]
		prevRune := invalidRune

		screenWidthF32 := float32(gr.ScreenWidth)
		if run.IsLtr {

			for i := 0; i < len(run.Runes); i++ {

				if run.Runes[i] == '\n' {
					drawPos.SetXYZ(startPos.X(), drawPos.Y()-lineHeightF32, startPos.Z())
				}

				gr.drawRune(run, i, prevRune, drawPos, color, lineHeightF32, &fgBufIndex, &bgBufIndex)
				prevRune = run.Runes[i]

				//Wrap
				if drawPos.X()+gr.Atlas.SpaceAdvance >= screenWidthF32 {

					drawPos.SetXYZ(startPos.X(), drawPos.Y()-lineHeightF32, startPos.Z())
					// startPos.SetY(startPos.Y() - lineHeightF32)
					// *drawPos = *startPos.Clone()
				}
			}

		} else {

			for i := len(run.Runes) - 1; i >= 0; i-- {

				if run.Runes[i] == '\n' {
					drawPos.SetXYZ(startPos.X(), drawPos.Y()-lineHeightF32, startPos.Z())
				}

				gr.drawRune(run, i, prevRune, drawPos, color, lineHeightF32, &fgBufIndex, &bgBufIndex)
				prevRune = run.Runes[i]

				//Wrap
				if drawPos.X()+gr.Atlas.SpaceAdvance >= screenWidthF32 {
					drawPos.SetXYZ(startPos.X(), drawPos.Y()-lineHeightF32, startPos.Z())
					// startPos.SetY(startPos.Y() - lineHeightF32)
					// *drawPos = *startPos.Clone()
				}
			}

		}

		if consts.Mode_Debug && PrintPositions {
			println("")
		}
	}

	return *drawPos
}

func (gr *GlyphRend) DrawTextOpenGLAbsRect(text []rune, rectTopLeft *gglm.Vec3, rectBotRight *gglm.Vec2, color *gglm.Vec4) gglm.Vec3 {

	runs := gr.TextRunsBuf[:]
	gr.GetTextRuns(text, &runs)
	if runs == nil {
		return *rectTopLeft
	}

	drawPos := rectTopLeft.Clone()
	lineHeightF32 := float32(gr.Atlas.LineHeight)
	fgBufIndex, bgBufIndex := gr.getFgAndBgBufIndices()
	for runIndex := 0; runIndex < len(runs); runIndex++ {

		run := &runs[runIndex]
		prevRune := invalidRune

		if run.IsLtr {

			for i := 0; i < len(run.Runes); i++ {

				if run.Runes[i] == '\n' {
					drawPos.SetXYZ(rectTopLeft.X(), drawPos.Y()-lineHeightF32, rectTopLeft.Z())
				}

				gr.drawRune(run, i, prevRune, drawPos, color, lineHeightF32, &fgBufIndex, &bgBufIndex)
				prevRune = run.Runes[i]

				//Wrap
				if drawPos.X()+gr.Atlas.SpaceAdvance >= rectBotRight.X() {

					drawPos.SetXYZ(rectTopLeft.X(), drawPos.Y()-lineHeightF32, rectTopLeft.Z())
					// rectTopLeft.SetY(rectTopLeft.Y() - lineHeightF32)
					// *drawPos = *rectTopLeft.Clone()
				}
			}

		} else {

			for i := len(run.Runes) - 1; i >= 0; i-- {

				if run.Runes[i] == '\n' {
					drawPos.SetXYZ(rectTopLeft.X(), drawPos.Y()-lineHeightF32, rectTopLeft.Z())
				}

				gr.drawRune(run, i, prevRune, drawPos, color, lineHeightF32, &fgBufIndex, &bgBufIndex)
				prevRune = run.Runes[i]

				//Wrap
				if drawPos.X()+gr.Atlas.SpaceAdvance >= rectBotRight.X() {
					drawPos.SetXYZ(rectTopLeft.X(), drawPos.Y()-lineHeightF32, rectTopLeft.Z())
					// rectTopLeft.SetY(rectTopLeft.Y() - lineHeightF32)
					// *drawPos = *rectTopLeft.Clone()
				}
			}

		}

		if consts.Mode_Debug && PrintPositions {
			println("")
		}
	}

	return *drawPos
}

func (gr *GlyphRend) DrawTextOpenGLAbsRectWithStartPos(text []rune, startPos, rectTopLeft *gglm.Vec3, rectBotRight *gglm.Vec2, color *gglm.Vec4) gglm.Vec3 {

	runs := gr.TextRunsBuf[:]
	gr.GetTextRuns(text, &runs)
	if runs == nil {
		return *startPos
	}

	drawPos := startPos.Clone()
	lineHeightF32 := float32(gr.Atlas.LineHeight)
	fgBufIndex, bgBufIndex := gr.getFgAndBgBufIndices()
	for runIndex := 0; runIndex < len(runs); runIndex++ {

		run := &runs[runIndex]
		prevRune := invalidRune

		if run.IsLtr {

			for i := 0; i < len(run.Runes); i++ {

				if run.Runes[i] == '\n' {
					drawPos.SetXYZ(rectTopLeft.X(), drawPos.Y()-lineHeightF32, rectTopLeft.Z())
				}

				gr.drawRune(run, i, prevRune, drawPos, color, lineHeightF32, &fgBufIndex, &bgBufIndex)
				prevRune = run.Runes[i]

				//Wrap
				if drawPos.X()+gr.Atlas.SpaceAdvance >= rectBotRight.X() {

					drawPos.SetXYZ(rectTopLeft.X(), drawPos.Y()-lineHeightF32, rectTopLeft.Z())
					// rectTopLeft.SetY(rectTopLeft.Y() - lineHeightF32)
					// *drawPos = *rectTopLeft.Clone()
				}
			}

		} else {

			for i := len(run.Runes) - 1; i >= 0; i-- {

				if run.Runes[i] == '\n' {
					drawPos.SetXYZ(rectTopLeft.X(), drawPos.Y()-lineHeightF32, rectTopLeft.Z())
				}

				gr.drawRune(run, i, prevRune, drawPos, color, lineHeightF32, &fgBufIndex, &bgBufIndex)
				prevRune = run.Runes[i]

				//Wrap
				if drawPos.X()+gr.Atlas.SpaceAdvance >= rectBotRight.X() {
					drawPos.SetXYZ(rectTopLeft.X(), drawPos.Y()-lineHeightF32, rectTopLeft.Z())
					// rectTopLeft.SetY(rectTopLeft.Y() - lineHeightF32)
					// *drawPos = *rectTopLeft.Clone()
				}
			}

		}

		if consts.Mode_Debug && PrintPositions {
			println("")
		}
	}

	return *drawPos
}

func (gr *GlyphRend) getFgAndBgBufIndices() (fgBufIndex, bgBufIndex uint32) {
	return gr.GlyphFgCount * floatsPerGlyph, gr.GlyphBgCount * floatsPerGlyph
}

// @Debug
var PrintPositions bool

func (gr *GlyphRend) GridSize() (w, h int64) {
	w = int64(gr.ScreenWidth) / int64(gr.Atlas.SpaceAdvance)
	h = int64(gr.ScreenHeight) / int64(gr.Atlas.LineHeight)
	return w, h
}

func (gr *GlyphRend) ScreenPosToGridPos(x, y float32) (gridX, gridY float32) {
	gridX = floorF32(x / gr.Atlas.SpaceAdvance)
	gridY = floorF32(y / gr.Atlas.LineHeight)
	return gridX, gridY
}

func (gr *GlyphRend) drawRune(run *TextRun, i int, prevRune rune, pos *gglm.Vec3, color *gglm.Vec4, lineHeightF32 float32, glyphFgBufIndex, glyphBgBufIndex *uint32) {

	r := run.Runes[i]
	if r == '\t' {
		pos.AddX(gr.Atlas.SpaceAdvance * float32(gr.SpacesPerTab))
		return
	}

	var g FontAtlasGlyph
	if run.IsLtr {
		if i < len(run.Runes)-1 {
			//start or middle of sentence
			g = GlyphFromRunes(gr.Atlas.Glyphs, r, prevRune, run.Runes[i+1])
		} else {
			//Last character
			g = GlyphFromRunes(gr.Atlas.Glyphs, r, prevRune, invalidRune)
		}
	} else {
		if i > 0 {
			//start or middle of sentence
			g = GlyphFromRunes(gr.Atlas.Glyphs, r, run.Runes[i-1], prevRune)
		} else {
			//Last character
			g = GlyphFromRunes(gr.Atlas.Glyphs, r, invalidRune, prevRune)
		}
	}

	//We must adjust char positioning according to: https://developer.apple.com/library/archive/documentation/TextFonts/Conceptual/CocoaTextArchitecture/Art/glyph_metrics_2x.png
	drawPos := *pos
	//The flooring to an integer pixel must happen AFTER the (potentially) fractional adjustments have been made.
	//This is what the truetype face.Rasterizer does and seems to give good results. Do NOT floor bearing/descent first.
	drawPos.SetX(floorF32(drawPos.X() + g.BearingX))
	drawPos.SetY(floorF32(drawPos.Y() - g.Descent))

	if consts.Mode_Debug && PrintPositions {
		oldXY := gglm.NewVec2(pos.X(), pos.Y())
		newXY := gglm.NewVec2(drawPos.X(), drawPos.Y())
		fmt.Printf("char=%s; PosBefore=%s, PosAfter=%s; Bearing/Decent=(%f, %f)\n", string(r), oldXY.String(), newXY.String(), g.BearingX, g.Descent)
	}

	//Add the glyph information to the vbo
	if gr.HasOpt(GlyphRendOpt_BgColor) {
		// UV
		gr.GlyphBgVBO[*glyphBgBufIndex+0] = -1
		gr.GlyphBgVBO[*glyphBgBufIndex+1] = -1
		*glyphBgBufIndex += 2

		//UVSize
		gr.GlyphBgVBO[*glyphBgBufIndex+0] = 0
		gr.GlyphBgVBO[*glyphBgBufIndex+1] = 0
		*glyphBgBufIndex += 2

		//Color
		gr.GlyphBgVBO[*glyphBgBufIndex+0] = gr.OptValues.BgColor.R()
		gr.GlyphBgVBO[*glyphBgBufIndex+1] = gr.OptValues.BgColor.G()
		gr.GlyphBgVBO[*glyphBgBufIndex+2] = gr.OptValues.BgColor.B()
		gr.GlyphBgVBO[*glyphBgBufIndex+3] = gr.OptValues.BgColor.A()
		*glyphBgBufIndex += 4

		//Model Pos
		gr.GlyphBgVBO[*glyphBgBufIndex+0] = pos.X()
		gr.GlyphBgVBO[*glyphBgBufIndex+1] = pos.Y()
		gr.GlyphBgVBO[*glyphBgBufIndex+2] = pos.Z()
		*glyphBgBufIndex += 3

		//Model Scale
		gr.GlyphBgVBO[*glyphBgBufIndex+0] = gr.Atlas.SpaceAdvance
		gr.GlyphBgVBO[*glyphBgBufIndex+1] = lineHeightF32
		*glyphBgBufIndex += 2

		gr.GlyphBgCount++
		if gr.GlyphBgCount == DefaultGlyphsPerBatch {
			gr.Draw()
			*glyphBgBufIndex = 0
		}
	}

	//UV
	gr.GlyphFgVBO[*glyphFgBufIndex+0] = g.U
	gr.GlyphFgVBO[*glyphFgBufIndex+1] = g.V
	*glyphFgBufIndex += 2

	//UVSize
	gr.GlyphFgVBO[*glyphFgBufIndex+0] = g.SizeU
	gr.GlyphFgVBO[*glyphFgBufIndex+1] = g.SizeV
	*glyphFgBufIndex += 2

	//Color
	gr.GlyphFgVBO[*glyphFgBufIndex+0] = color.R()
	gr.GlyphFgVBO[*glyphFgBufIndex+1] = color.G()
	gr.GlyphFgVBO[*glyphFgBufIndex+2] = color.B()
	gr.GlyphFgVBO[*glyphFgBufIndex+3] = color.A()
	*glyphFgBufIndex += 4

	//Model Pos
	gr.GlyphFgVBO[*glyphFgBufIndex+0] = drawPos.X()
	gr.GlyphFgVBO[*glyphFgBufIndex+1] = drawPos.Y()
	gr.GlyphFgVBO[*glyphFgBufIndex+2] = drawPos.Z()
	*glyphFgBufIndex += 3

	//Model Scale
	gr.GlyphFgVBO[*glyphFgBufIndex+0] = g.SizeU
	gr.GlyphFgVBO[*glyphFgBufIndex+1] = g.SizeV
	*glyphFgBufIndex += 2

	pos.AddX(g.Advance)

	//If we fill the buffer we issue a draw call
	gr.GlyphFgCount++
	if gr.GlyphFgCount == DefaultGlyphsPerBatch {
		gr.Draw()
		*glyphFgBufIndex = 0
	}
}

// func roundF32(x float32) float32 {
// 	return float32(math.Round(float64(x)))
// }

// func ceilF32(x float32) float32 {
// 	return float32(math.Ceil(float64(x)))
// }

func floorF32(x float32) float32 {
	return float32(math.Floor(float64(x)))
}

type TextRun struct {
	Runes []rune
	IsLtr bool
}

func (gr *GlyphRend) GetTextRuns(rs []rune, textRunsBuf *[]TextRun) {

	if len(rs) == 0 {
		return
	}

	runs := textRunsBuf
	currRunScript := RuneInfos[rs[0]].ScriptTable

	//TODO: We need to detect neutral characters through BiDi category, not being in common
	//TODO: Diacritics go into things like 'Category_Mn' and don't necessairly follow the parent script (e.g. Arabic diacritics are NOT in unicode.Arabic).
	//They should be part of the same run but right now we split them into their own run.
	runStartIndex := 0
	for i := 1; i < len(rs); i++ {

		r := rs[i]
		ri := RuneInfos[r]
		//A run is a set of characters using the same script (and other metrics) minus leading/trailing neutral characters
		if ri.ScriptTable == currRunScript || ri.ScriptTable == unicode.Common {
			continue
		}

		//We reached a new run so count trailing neutrals to be removed from this run
		newRunRunes := rs[runStartIndex:i]
		trailingCommonsCount := 0
		for j := len(newRunRunes) - 1; j >= 0; j-- {
			if !unicode.Is(unicode.Common, newRunRunes[j]) {
				break
			}
			trailingCommonsCount++
		}

		//If we have a run without trailing neutrals or had a run of just neutrals (e.g. starting sentence with spaces)
		//then the full run is added, otherwise we slice the run to put neturals in a separate run
		if trailingCommonsCount == 0 || len(newRunRunes) == trailingCommonsCount {
			*runs = append(*runs, TextRun{Runes: newRunRunes})
		} else {
			*runs = append(*runs,
				TextRun{Runes: newRunRunes[:len(newRunRunes)-trailingCommonsCount]}, TextRun{Runes: newRunRunes[len(newRunRunes)-trailingCommonsCount:]})
		}

		//The removed neutrals are included as the start of the new run
		runStartIndex = i
		currRunScript = ri.ScriptTable
	}

	*runs = append(*runs, TextRun{Runes: rs[runStartIndex:]})

	//Detect directionality of each run
	for i := 0; i < len(*runs); i++ {

		run := &(*runs)[i]
		bidiCat := BidiCategory_L
		for _, r := range run.Runes {
			if !unicode.Is(unicode.Common, r) {
				bidiCat = RuneInfos[r].BidiCat
				break
			}
		}
		run.IsLtr = !(bidiCat == BidiCategory_R || bidiCat == BidiCategory_AL || bidiCat == BidiCategory_RLE || bidiCat == BidiCategory_RLO || bidiCat == BidiCategory_RLI || bidiCat == BidiCategory_RLM)
	}
}

// GlyphFromRunes does shaping where it selects the proper rune based (e.g. end Alef) on the surrounding runes
func GlyphFromRunes(glyphTable map[rune]FontAtlasGlyph, curr, prev, next rune) FontAtlasGlyph {

	//PERF: Map access times are absolute garbage to the point that ~85%+ of the runtime of this func
	//is spent reading from maps :). Using nSet or fMap or similar would be a lot better.
	type PosCtx int
	const (
		PosCtx_start PosCtx = iota
		PosCtx_mid
		PosCtx_end
		PosCtx_isolated
	)

	prevIsValid := prev != invalidRune
	nextIsValid := next != invalidRune

	//Isolated case
	if !prevIsValid && !nextIsValid {
		return glyphTable[curr]
	}

	ri := RuneInfos[curr]
	if ri.JoinType == JoiningType_None || ri.JoinType == JoiningType_Transparent {
		return glyphTable[curr]
	}

	prevJoinType := RuneInfos[prev].JoinType
	joinWithRight := prevIsValid &&
		(prevJoinType == JoiningType_Dual || prevJoinType == JoiningType_Left || prevJoinType == JoiningType_Causing) &&
		(ri.JoinType == JoiningType_Dual || ri.JoinType == JoiningType_Right)

	nextJoinType := RuneInfos[next].JoinType
	joinWithLeft := nextIsValid &&
		(nextJoinType == JoiningType_Dual || nextJoinType == JoiningType_Right || nextJoinType == JoiningType_Causing) &&
		(ri.JoinType == JoiningType_Dual || ri.JoinType == JoiningType_Left)

	var ctx PosCtx
	if joinWithRight && joinWithLeft {
		ctx = PosCtx_mid
	} else if joinWithLeft {
		ctx = PosCtx_start
	} else if joinWithRight {
		ctx = PosCtx_end
	} else {
		ctx = PosCtx_isolated
	}

	//This is only needed for Arabic (I think)
	switch ctx {
	case PosCtx_start:

		for i := 0; i < len(ri.EquivalentRunes); i++ {

			otherRune := ri.EquivalentRunes[i]
			otherDecompTag := RuneInfos[otherRune].DecompTag
			if otherDecompTag == DecompTag_initial {
				curr = otherRune
				break
			}
		}

	case PosCtx_mid:

		for i := 0; i < len(ri.EquivalentRunes); i++ {

			otherRune := ri.EquivalentRunes[i]
			otherDecompTag := RuneInfos[otherRune].DecompTag
			if otherDecompTag == DecompTag_medial {
				curr = otherRune
				break
			}
		}

	case PosCtx_end:

		for i := 0; i < len(ri.EquivalentRunes); i++ {

			otherRune := ri.EquivalentRunes[i]
			otherDecompTag := RuneInfos[otherRune].DecompTag
			if otherDecompTag == DecompTag_final {
				curr = otherRune
				break
			}
		}
	}

	return glyphTable[curr]
}

func (gr *GlyphRend) Draw() {

	if gr.GlyphFgCount == 0 && gr.GlyphBgCount == 0 {
		return
	}

	// Set common GPU settings for both Fg and Bg
	gr.GlyphMat.Bind()
	gl.Disable(gl.DEPTH_TEST) //We need to disable depth testing so that nearby characters don't occlude each other

	// We must draw Bg BEFORE foreground because they write to the same pixels.
	// Drawing everything in one instance buffer/batch doesn't work because the order in which instances are drawn in a batch is not guaranteed,
	// which means a background might be drawn on top of a foreground glyph.
	if gr.GlyphBgCount > 0 {

		gl.BindVertexArray(gr.GlyphBgInstancedBuf.VAOID)
		gl.BindBuffer(gl.ARRAY_BUFFER, gr.GlyphBgInstancedBuf.BufID)
		gl.BufferSubData(gl.ARRAY_BUFFER, 0, int(gr.GlyphBgCount*floatsPerGlyph)*4, gl.Ptr(&gr.GlyphBgVBO[:gr.GlyphBgCount*floatsPerGlyph][0]))

		gl.DrawElementsInstanced(gl.TRIANGLES, gr.GlyphBgInstancedBuf.IndexBufCount, gl.UNSIGNED_INT, gl.PtrOffset(0), int32(gr.GlyphBgCount))
		gr.GlyphBgCount = 0
	}

	if gr.GlyphFgCount > 0 {
		gl.BindVertexArray(gr.GlyphFgInstancedBuf.VAOID)
		gl.BindBuffer(gl.ARRAY_BUFFER, gr.GlyphFgInstancedBuf.BufID)
		gl.BufferSubData(gl.ARRAY_BUFFER, 0, int(gr.GlyphFgCount*floatsPerGlyph)*4, gl.Ptr(&gr.GlyphFgVBO[:gr.GlyphFgCount*floatsPerGlyph][0]))

		gl.DrawElementsInstanced(gl.TRIANGLES, gr.GlyphFgInstancedBuf.IndexBufCount, gl.UNSIGNED_INT, gl.PtrOffset(0), int32(gr.GlyphFgCount))
		gr.GlyphFgCount = 0
	}

	gl.Enable(gl.DEPTH_TEST)
}

// SetFace updates the underlying font atlas used by the glyph renderer.
// The current atlas is unchanged if there is an error
func (gr *GlyphRend) SetFace(fontOptions *truetype.Options) error {

	face := truetype.NewFace(gr.Atlas.Font, fontOptions)
	newAtlas, err := NewFontAtlasFromFont(gr.Atlas.Font, face, uint(fontOptions.Size))
	if err != nil {
		return err
	}

	gr.Atlas = newAtlas
	gr.updateFontAtlasTexture()
	return nil
}

func (gr *GlyphRend) SetFontFromFile(fontFile string, fontOptions *truetype.Options) error {

	atlas, err := NewFontAtlasFromFile(fontFile, fontOptions)
	if err != nil {
		return err
	}

	gr.Atlas = atlas
	gr.updateFontAtlasTexture()
	return nil
}

// updateFontAtlasTexture uploads the texture representing the font atlas to the GPU
// and updates the GlyphRend.AtlasTex field.
//
// Any old textures are deleted
func (gr *GlyphRend) updateFontAtlasTexture() error {

	//Clean old texture and load new texture
	if gr.AtlasTex != nil {
		gl.DeleteTextures(1, &gr.AtlasTex.TexID)
		gr.AtlasTex = nil
	}

	atlasTex, err := assets.LoadTextureInMemPngImg(gr.Atlas.Img, nil)
	if err != nil {
		return err
	}
	gr.AtlasTex = &atlasTex

	gl.BindTexture(gl.TEXTURE_2D, atlasTex.TexID)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.BindTexture(gl.TEXTURE_2D, 0)

	//Update material
	gr.GlyphMat.DiffuseTex = gr.AtlasTex.TexID

	return nil
}

func (gr *GlyphRend) SetScreenSize(screenWidth, screenHeight int32) {

	gr.ScreenWidth = screenWidth
	gr.ScreenHeight = screenHeight

	//The camera is orthographic and fits the screen size exactly. This is needed so we can size and position characters correctly.
	cam := camera.NewOrthographic(gglm.NewVec3(0, 0, 10), gglm.NewVec3(0, 0, -1), gglm.NewVec3(0, 1, 0), 0.1, 20, 0, float32(screenWidth), float32(screenHeight), 0)
	projViewMtx := cam.ProjMat.Mul(&cam.ViewMat)

	gr.GlyphMat.SetUnifMat4("projViewMat", projViewMtx)
}

func NewGlyphRend(fontFile string, fontOptions *truetype.Options, screenWidth, screenHeight int32) (*GlyphRend, error) {

	var err error
	if RuneInfos == nil {
		RuneInfos, err = ParseUnicodeData("./unicode-data-13.txt", "./arabic-shaping-13.txt")
		if err != nil {
			return nil, err
		}
	}

	gr := &GlyphRend{
		GlyphFgCount: 0,
		GlyphFgVBO:   make([]float32, floatsPerGlyph*DefaultGlyphsPerBatch),

		GlyphBgCount: 0,
		GlyphBgVBO:   make([]float32, floatsPerGlyph*DefaultGlyphsPerBatch),
		TextRunsBuf:  make([]TextRun, 0, 20),
		SpacesPerTab: 4,

		Opts: GlyphRendOpt_None,
		OptValues: GlyphRendOptValues{
			BgColor: gglm.NewVec4(0, 0, 0, 0),
		},
	}

	//Create glyph mesh
	gr.GlyphMesh = &meshes.Mesh{
		Name: "glypQuad",

		//VertPos only. Instanced attributes are stored separately
		Buf: buffers.NewBuffer(
			buffers.Element{ElementType: buffers.DataTypeVec3},
		),
	}

	//The quad must be anchored at the bottom-left, not it's center (i.e. bottom-left vertex must be at 0,0)
	gr.GlyphMesh.Buf.SetData([]float32{
		0, 0, 0,
		1, 0, 0,
		0, 1, 0,
		1, 1, 0,
	})

	gr.GlyphMesh.Buf.SetIndexBufData([]uint32{
		0, 1, 2,
		1, 3, 2,
	})

	//Setup material
	gr.GlyphMat = materials.NewMaterial("glyphMat", "./res/shaders/glyph.glsl")

	//With the material ready we can generate the atlas
	gr.Atlas, err = NewFontAtlasFromFile(fontFile, fontOptions)
	if err != nil {
		return nil, err
	}

	err = gr.updateFontAtlasTexture()
	if err != nil {
		return nil, err
	}

	// In each of these buffers we use one the mesh VBO for vertex position data (vertex attrib 0), and the other VBO created here for the rest.
	// We split bg/fg into separate buffers and draw calls to ensure bg is drawn first, because order of drawing instanced items sent in the same
	// call is not guaranteed
	gr.GlyphFgInstancedBuf = buffers.NewBuffer()
	gr.GlyphBgInstancedBuf = buffers.NewBuffer()

	// Create instanced buffers and set their vertex attributes.
	glyphMeshVertPosLayoutEle := gr.GlyphMesh.Buf.GetLayout()[0]
	setupGlyphInstanceBuf := func(instanceBuf *buffers.Buffer, vbo *[]float32) {

		instanceBuf.SetLayout(
			buffers.Element{ElementType: buffers.DataTypeVec2}, //UV0
			buffers.Element{ElementType: buffers.DataTypeVec2}, //UVSize
			buffers.Element{ElementType: buffers.DataTypeVec4}, //Color
			buffers.Element{ElementType: buffers.DataTypeVec3}, //ModelPos
			buffers.Element{ElementType: buffers.DataTypeVec2}, //ModelScale
		)

		// Use the index buffer based on the mesh data
		instanceBuf.IndexBufID = gr.GlyphMesh.Buf.IndexBufID
		instanceBuf.SetIndexBufData([]uint32{
			0, 1, 2,
			1, 3, 2,
		})

		// Set index buf data unbinds at the end so we re-bind here
		instanceBuf.Bind()

		// Set vertex attribute zero to use the buffer of the glyph mesh, which contains onlt vertex positions.
		// Since all instances have the same vertex positions we only send this data in the first vertex and all instances refer
		// to the position information of the first vertex. This is done by setting an attrib divisor of zero for this attribute
		gl.BindBuffer(gl.ARRAY_BUFFER, gr.GlyphMesh.Buf.BufID)
		gl.EnableVertexAttribArray(0)
		gl.VertexAttribPointer(0, glyphMeshVertPosLayoutEle.ElementType.CompCount(), glyphMeshVertPosLayoutEle.ElementType.GLType(), false, gr.GlyphMesh.Buf.Stride, gl.PtrOffset(glyphMeshVertPosLayoutEle.Offset))
		gl.VertexAttribDivisor(0, 0)

		// Set the rest of the vertex attributes to use the instance buffer
		gl.BindBuffer(gl.ARRAY_BUFFER, instanceBuf.BufID)
		layout := instanceBuf.GetLayout()

		uvEle := layout[0]
		gl.EnableVertexAttribArray(1)
		gl.VertexAttribPointer(1, uvEle.ElementType.CompCount(), uvEle.ElementType.GLType(), false, instanceBuf.Stride, gl.PtrOffset(uvEle.Offset))
		gl.VertexAttribDivisor(1, 1)

		uvSize := layout[1]
		gl.EnableVertexAttribArray(2)
		gl.VertexAttribPointer(2, uvSize.ElementType.CompCount(), uvSize.ElementType.GLType(), false, instanceBuf.Stride, gl.PtrOffset(uvSize.Offset))
		gl.VertexAttribDivisor(2, 1)

		colorEle := layout[2]
		gl.EnableVertexAttribArray(3)
		gl.VertexAttribPointer(3, colorEle.ElementType.CompCount(), colorEle.ElementType.GLType(), false, instanceBuf.Stride, gl.PtrOffset(colorEle.Offset))
		gl.VertexAttribDivisor(3, 1)

		posEle := layout[3]
		gl.EnableVertexAttribArray(4)
		gl.VertexAttribPointer(4, posEle.ElementType.CompCount(), posEle.ElementType.GLType(), false, instanceBuf.Stride, gl.PtrOffset(posEle.Offset))
		gl.VertexAttribDivisor(4, 1)

		scaleEle := layout[4]
		gl.EnableVertexAttribArray(5)
		gl.VertexAttribPointer(5, scaleEle.ElementType.CompCount(), scaleEle.ElementType.GLType(), false, instanceBuf.Stride, gl.PtrOffset(scaleEle.Offset))
		gl.VertexAttribDivisor(5, 1)

		//Fill buffer with zeros and set to dynamic so in the actual draw calls we use bufferSubData which makes things a lot faster
		gl.BufferData(gl.ARRAY_BUFFER, len(*vbo)*4, gl.Ptr(&(*vbo)[0]), buffers.BufUsage_Dynamic.ToGL())

		//Reset mesh layout because the instancedBuf setLayout over-wrote vertex attribute 0
		gr.GlyphMesh.Buf.SetLayout(buffers.Element{ElementType: buffers.DataTypeVec3})
	}

	setupGlyphInstanceBuf(&gr.GlyphFgInstancedBuf, &gr.GlyphFgVBO)
	setupGlyphInstanceBuf(&gr.GlyphBgInstancedBuf, &gr.GlyphBgVBO)

	gr.SetScreenSize(screenWidth, screenHeight)

	return gr, nil
}
