package glyphs

import (
	"errors"
	"math"
	"unicode"

	"github.com/bloeys/gglm/gglm"
	"github.com/bloeys/nmage/assets"
	"github.com/bloeys/nmage/buffers"
	"github.com/bloeys/nmage/materials"
	"github.com/bloeys/nmage/meshes"
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/golang/freetype/truetype"
)

const (
	MaxGlyphsPerBatch = 16384

	floatsPerGlyph = 11
	invalidRune    = unicode.ReplacementChar
)

var (
	RuneInfos map[rune]RuneInfo
)

type GlyphRend struct {
	Atlas    *FontAtlas
	AtlasTex *assets.Texture

	GlyphMesh    *meshes.Mesh
	InstancedBuf buffers.Buffer
	GlyphMat     *materials.Material

	GlyphCount uint32
	//NOTE: Because of the sad realities (bugs?) of CGO, passing an array in a struct
	//to C explodes (Go pointer to Go pointer error) even though passing the same array
	//allocated inside the function is fine (Go potentially can't detect what's happening properly).
	//
	//Luckily slices still work, so for now we will use our slice as an array (no appending)
	GlyphVBO []float32
	// GlyphVBO [floatsPerGlyph * maxGlyphsPerBatch]float32

	ScreenWidth  int32
	ScreenHeight int32

	SpacesPerTab uint
}

//DrawTextOpenGLAbs prepares text that will be drawn on the next GlyphRend.Draw call.
//screenPos is in the range [0,1], where (0,0) is the bottom left.
//Color is RGBA in the range [0,1].
func (gr *GlyphRend) DrawTextOpenGL01(text string, screenPos *gglm.Vec3, color *gglm.Vec4) {
	screenPos.Set(screenPos.X()*float32(gr.ScreenWidth), screenPos.Y()*float32(gr.ScreenHeight), screenPos.Z())
	gr.DrawTextOpenGLAbs(text, screenPos, color)
}

//DrawTextOpenGLAbs prepares text that will be drawn on the next GlyphRend.Draw call.
//screenPos is in the range ([0,ScreenWidth],[0,ScreenHeight]).
//Color is RGBA in the range [0,1].
func (gr *GlyphRend) DrawTextOpenGLAbs(text string, screenPos *gglm.Vec3, color *gglm.Vec4) {

	runs := gr.GetTextRuns(text)
	if runs == nil {
		return
	}

	pos := screenPos.Clone()
	advanceF32 := float32(gr.Atlas.Advance)
	lineHeightF32 := float32(gr.Atlas.LineHeight)
	scale := gglm.NewVec2(advanceF32, lineHeightF32)

	buffIndex := gr.GlyphCount * floatsPerGlyph

	for _, run := range runs {

		rs := run
		prevRune := invalidRune

		//TODO: Information on run (like bidi) should come from GetTextRuns.
		//Default is left in case we hav a run of neutrals
		bidiCat := BidiCategory_L
		for _, r := range rs {
			if !unicode.Is(unicode.Common, r) {
				bidiCat = RuneInfos[r].BidiCat
				break
			}
		}
		isLtr := !(bidiCat == BidiCategory_R || bidiCat == BidiCategory_AL || bidiCat == BidiCategory_RLE || bidiCat == BidiCategory_RLO || bidiCat == BidiCategory_RLI || bidiCat == BidiCategory_RLM)

		if isLtr {

			for i := 0; i < len(rs); i++ {

				r := rs[i]
				if r == '\n' {
					screenPos.SetY(screenPos.Y() - lineHeightF32)
					pos = screenPos.Clone()
					prevRune = r
					continue
				} else if r == ' ' {
					pos.AddX(advanceF32)
					prevRune = r
					continue
				} else if r == '\t' {
					pos.AddX(advanceF32 * float32(gr.SpacesPerTab))
					prevRune = r
					continue
				}

				var g FontAtlasGlyph
				if i < len(rs)-1 {
					//start or middle of sentence
					g = gr.glyphFromRunes(r, prevRune, rs[i+1])
				} else {
					//Last character
					g = gr.glyphFromRunes(r, prevRune, invalidRune)
				}

				//See: https://developer.apple.com/library/archive/documentation/TextFonts/Conceptual/CocoaTextArchitecture/Art/glyph_metrics_2x.png
				//The uvs coming in make it so that glyphs are sitting on top of the baseline (no descent) and with horizontal bearing applied.
				//So to position correctly we move them down by the descent amount.
				drawPos := *pos
				drawPos.SetX(drawPos.X())
				drawPos.SetY(drawPos.Y() - g.Descent)

				//Add the glyph information to the vbo
				//UV
				gr.GlyphVBO[buffIndex+0] = g.U
				gr.GlyphVBO[buffIndex+1] = g.V

				//Color
				gr.GlyphVBO[buffIndex+2] = color.R()
				gr.GlyphVBO[buffIndex+3] = color.G()
				gr.GlyphVBO[buffIndex+4] = color.B()
				gr.GlyphVBO[buffIndex+5] = color.A()

				//Model Pos
				gr.GlyphVBO[buffIndex+6] = drawPos.X()
				gr.GlyphVBO[buffIndex+7] = drawPos.Y()
				gr.GlyphVBO[buffIndex+8] = drawPos.Z()

				//Model Scale
				gr.GlyphVBO[buffIndex+9] = scale.X()
				gr.GlyphVBO[buffIndex+10] = scale.Y()

				gr.GlyphCount++
				pos.AddX(advanceF32)

				//If we fill the buffer we issue a draw call
				if gr.GlyphCount == MaxGlyphsPerBatch {
					gr.Draw()
					buffIndex = 0
				} else {
					buffIndex += floatsPerGlyph
				}

				prevRune = r
			}

		} else {

			for i := len(rs) - 1; i >= 0; i-- {

				r := rs[i]
				if r == '\n' {
					screenPos.SetY(screenPos.Y() - lineHeightF32)
					pos = screenPos.Clone()
					prevRune = r
					continue
				} else if r == ' ' {
					pos.AddX(advanceF32)
					prevRune = r
					continue
				} else if r == '\t' {
					pos.AddX(advanceF32 * float32(gr.SpacesPerTab))
					prevRune = r
					continue
				}

				var g FontAtlasGlyph
				if i > 0 {
					//start or middle of sentence
					g = gr.glyphFromRunes(r, rs[i-1], prevRune)
				} else {
					//Last character
					g = gr.glyphFromRunes(r, invalidRune, prevRune)
				}

				//See: https://developer.apple.com/library/archive/documentation/TextFonts/Conceptual/CocoaTextArchitecture/Art/glyph_metrics_2x.png
				//The uvs coming in make it so that glyphs are sitting on top of the baseline (no descent) and with horizontal bearing applied.
				//So to position correctly we move them down by the descent amount.
				drawPos := *pos
				drawPos.SetX(drawPos.X())
				drawPos.SetY(drawPos.Y() - g.Descent)

				//Add the glyph information to the vbo
				//UV
				gr.GlyphVBO[buffIndex+0] = g.U
				gr.GlyphVBO[buffIndex+1] = g.V

				//Color
				gr.GlyphVBO[buffIndex+2] = color.R()
				gr.GlyphVBO[buffIndex+3] = color.G()
				gr.GlyphVBO[buffIndex+4] = color.B()
				gr.GlyphVBO[buffIndex+5] = color.A()

				//Model Pos
				gr.GlyphVBO[buffIndex+6] = roundF32(drawPos.X())
				gr.GlyphVBO[buffIndex+7] = roundF32(drawPos.Y())
				gr.GlyphVBO[buffIndex+8] = drawPos.Z()

				//Model Scale
				gr.GlyphVBO[buffIndex+9] = scale.X()
				gr.GlyphVBO[buffIndex+10] = scale.Y()

				gr.GlyphCount++
				pos.AddX(advanceF32)

				//If we fill the buffer we issue a draw call
				if gr.GlyphCount == MaxGlyphsPerBatch {
					gr.Draw()
					buffIndex = 0
				} else {
					buffIndex += floatsPerGlyph
				}

				prevRune = r
			}

		}

	}
}

func (gr *GlyphRend) GetTextRuns(t string) [][]rune {

	rs := []rune(t)

	if len(rs) == 0 {
		return nil
	}

	runs := make([][]rune, 0, 10)
	currRunScript := RuneInfos[rs[0]].ScriptTable

	//TODO: We need to detect neutral characters through BiDi category, not being in common
	runStartIndex := 0
	for i := 1; i < len(rs); i++ {

		r := rs[i]
		ri := RuneInfos[r]
		//A run is a set of characters using the same script (and other metrics) minus leading/trailing neutral characters
		if ri.ScriptTable == currRunScript || ri.ScriptTable == unicode.Common {
			continue
		}

		//We reached a new run so count trailing neutrals to be removed from this run
		newRun := rs[runStartIndex:i]
		trailingCommonsCount := 0
		for j := len(newRun) - 1; j >= 0; j-- {
			if !unicode.Is(unicode.Common, newRun[j]) {
				break
			}
			trailingCommonsCount++
		}

		//If we have a run without trailing neutrals or had a run of just neutrals (e.g. starting sentence with spaces)
		//then the full run is added, otherwise we slice the run to put neturals in a separate run
		if trailingCommonsCount == 0 || len(newRun) == trailingCommonsCount {
			runs = append(runs, newRun)
		} else {
			runs = append(runs, newRun[:len(newRun)-trailingCommonsCount], newRun[len(newRun)-trailingCommonsCount:])
		}

		//The removed neutrals are included as the start of the new run
		runStartIndex = i
		currRunScript = ri.ScriptTable
	}

	runs = append(runs, rs[runStartIndex:])

	return runs
}

func (gr *GlyphRend) glyphFromRunes(curr, prev, next rune) FontAtlasGlyph {

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
		g := gr.Atlas.Glyphs[curr]
		return g
	}

	ri := RuneInfos[curr]

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

	case PosCtx_isolated:

		// equivRunes := RuneInfos[curr].EquivalentRunes
		// for i := 0; i < len(equivRunes); i++ {

		// 	otherRune := equivRunes[i]
		// 	otherRuneInfo := RuneInfos[otherRune]
		// 	if otherRuneInfo.DecompTag == DecompTag_isolated {
		// 		curr = otherRune
		// 		break
		// 	}
		// }
	}

	g := gr.Atlas.Glyphs[curr]
	return g
}

func (gr *GlyphRend) Draw() {

	if gr.GlyphCount == 0 {
		return
	}

	gr.InstancedBuf.SetData(gr.GlyphVBO[:gr.GlyphCount*floatsPerGlyph])
	gr.InstancedBuf.Bind()
	gr.GlyphMat.Bind()

	gl.DrawElementsInstanced(gl.TRIANGLES, gr.GlyphMesh.Buf.IndexBufCount, gl.UNSIGNED_INT, gl.PtrOffset(0), int32(gr.GlyphCount))
	gr.GlyphCount = 0
}

//SetFace updates the underlying font atlas used by the glyph renderer.
//The current atlas is unchanged if there is an error
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

//updateFontAtlasTexture uploads the texture representing the font atlas to the GPU
//and updates the GlyphRend.AtlasTex field.
//
//Any old textures are deleted
func (gr *GlyphRend) updateFontAtlasTexture() error {

	//Clean old texture and load new texture
	if gr.AtlasTex != nil {
		gl.DeleteTextures(1, &gr.AtlasTex.TexID)
		gr.AtlasTex = nil
	}

	atlasTex, err := assets.LoadTextureInMemImg(gr.Atlas.Img, nil)
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
	gr.GlyphMat.SetUnifVec2("sizeUV", &gr.Atlas.SizeUV)

	return nil
}

func (gr *GlyphRend) SetScreenSize(screenWidth, screenHeight int32) {

	gr.ScreenWidth = screenWidth
	gr.ScreenHeight = screenHeight

	//The projection matrix fits the screen size. This is needed so we can size and position characters correctly.
	projMtx := gglm.Ortho(0, float32(screenWidth), float32(screenHeight), 0, 0.1, 20)
	viewMtx := gglm.LookAt(gglm.NewVec3(0, 0, -10), gglm.NewVec3(0, 0, 0), gglm.NewVec3(0, 1, 0))
	projViewMtx := projMtx.Mul(viewMtx)

	gr.GlyphMat.SetUnifMat4("projViewMat", &projViewMtx.Mat4)
}

func NewGlyphRend(fontFile string, fontOptions *truetype.Options, screenWidth, screenHeight int32) (*GlyphRend, error) {

	gr := &GlyphRend{
		GlyphCount:   0,
		GlyphVBO:     make([]float32, floatsPerGlyph*MaxGlyphsPerBatch),
		SpacesPerTab: 4,
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
	var err error
	gr.Atlas, err = NewFontAtlasFromFile(fontFile, fontOptions)
	if err != nil {
		return nil, err
	}

	err = gr.updateFontAtlasTexture()
	if err != nil {
		return nil, err
	}

	//Create instanced buf and set its instanced attributes.
	//Multiple VBOs under one VAO, one VBO for vertex data, and one VBO for instanced data.
	gr.InstancedBuf = buffers.Buffer{
		VAOID: gr.GlyphMesh.Buf.VAOID,
	}

	gl.GenBuffers(1, &gr.InstancedBuf.BufID)
	if gr.InstancedBuf.BufID == 0 {
		return nil, errors.New("failed to create OpenGL VBO buffer")
	}

	gr.InstancedBuf.SetLayout(
		buffers.Element{ElementType: buffers.DataTypeVec2}, //UV0
		buffers.Element{ElementType: buffers.DataTypeVec4}, //Color
		buffers.Element{ElementType: buffers.DataTypeVec3}, //ModelPos
		buffers.Element{ElementType: buffers.DataTypeVec2}, //ModelScale
	)

	gr.InstancedBuf.Bind()
	gl.BindBuffer(gl.ARRAY_BUFFER, gr.InstancedBuf.BufID)
	layout := gr.InstancedBuf.GetLayout()

	//Instanced attributes
	uvEle := layout[0]
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(1, uvEle.ElementType.CompCount(), uvEle.ElementType.GLType(), false, gr.InstancedBuf.Stride, gl.PtrOffset(uvEle.Offset))
	gl.VertexAttribDivisor(1, 1)

	colorEle := layout[1]
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointer(2, colorEle.ElementType.CompCount(), colorEle.ElementType.GLType(), false, gr.InstancedBuf.Stride, gl.PtrOffset(colorEle.Offset))
	gl.VertexAttribDivisor(2, 1)

	posEle := layout[2]
	gl.EnableVertexAttribArray(3)
	gl.VertexAttribPointer(3, posEle.ElementType.CompCount(), posEle.ElementType.GLType(), false, gr.InstancedBuf.Stride, gl.PtrOffset(posEle.Offset))
	gl.VertexAttribDivisor(3, 1)

	scaleEle := layout[3]
	gl.EnableVertexAttribArray(4)
	gl.VertexAttribPointer(4, scaleEle.ElementType.CompCount(), scaleEle.ElementType.GLType(), false, gr.InstancedBuf.Stride, gl.PtrOffset(scaleEle.Offset))
	gl.VertexAttribDivisor(4, 1)

	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gr.InstancedBuf.UnBind()

	//Reset mesh layout because the instancedBuf setLayout over-wrote vertex attribute 0
	gr.GlyphMesh.Buf.SetLayout(buffers.Element{ElementType: buffers.DataTypeVec3})

	gr.SetScreenSize(screenWidth, screenHeight)

	RuneInfos, err = ParseUnicodeData("./unicode-data-13.txt", "./arabic-shaping-13.txt")
	if err != nil {
		return nil, err
	}

	return gr, nil
}

func roundF32(x float32) float32 {
	return float32(math.Round(float64(x)))
}
