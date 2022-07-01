package glyphs

import (
	"errors"
	"os"
	"unicode"

	"github.com/bloeys/gglm/gglm"
	"github.com/bloeys/nmage/assets"
	"github.com/bloeys/nmage/buffers"
	"github.com/bloeys/nmage/materials"
	"github.com/bloeys/nmage/meshes"
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/golang/freetype/truetype"
)

type GlyphRend struct {
	Atlas    *FontAtlas
	AtlasTex *assets.Texture

	GlyphMesh    *meshes.Mesh
	InstancedBuf buffers.Buffer
	GlyphMat     *materials.Material

	GlyphCount int32
	GlyphVBO   []float32

	ScreenWidth  int32
	ScreenHeight int32
}

//getGlyphRanges returns a list of ranges, each range is: [i][0]<=range<[i][1]
func getGlyphRanges(f *truetype.Font) (ret [][2]rune) {
	rr := [2]rune{-1, -1}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if privateUseArea(r) {
			continue
		}
		if f.Index(r) == 0 {
			continue
		}
		if rr[1] == r {
			rr[1] = r + 1
			continue
		}
		if rr[0] != -1 {
			ret = append(ret, rr)
		}
		rr = [2]rune{r, r + 1}
	}
	if rr[0] != -1 {
		ret = append(ret, rr)
	}
	return ret
}

func privateUseArea(r rune) bool {
	return 0xe000 <= r && r <= 0xf8ff ||
		0xf0000 <= r && r <= 0xffffd ||
		0x100000 <= r && r <= 0x10fffd
}

//getGlyphsFromRanges takes ranges of runes and produces an array of all the runes in these ranges
func getGlyphsFromRanges(ranges [][2]rune) []rune {

	out := make([]rune, 0)
	for _, rr := range ranges {

		temp := make([]rune, 0, rr[1]-rr[0])
		for r := rr[0]; r < rr[1]; r++ {
			temp = append(temp, r)
		}

		out = append(out, temp...)
	}

	return out
}

//DrawTextOpenGL prepares text that will be drawn on the next GlyphRend.Draw call.
//screenPos is in the range [0,1], where (0,0) is the bottom left.
//Color is RGBA in the range [0,1].
func (gr *GlyphRend) DrawTextOpenGL(text string, screenPos *gglm.Vec3, color *gglm.Vec4) {

	screenWidthF32 := float32(gr.ScreenWidth)
	screenHeightF32 := float32(gr.ScreenHeight)
	screenPos.Set(screenPos.X()*screenWidthF32, screenPos.Y()*screenHeightF32, screenPos.Z())

	//Prepass to pre-allocate the buffer
	rs := []rune(text)
	const floatsPerGlyph = 18

	pos := screenPos.Clone()
	instancedData := make([]float32, 0, len(rs)*floatsPerGlyph) //This a larger approximation than needed because we don't count spaces etc
	for i := 0; i < len(rs); i++ {

		r := rs[i]
		g := gr.Atlas.Glyphs[r]
		if r == '\n' {
			screenPos.SetY(screenPos.Y() - float32(gr.Atlas.LineHeight))
			pos = screenPos.Clone()
			continue
		}
		gr.GlyphCount++

		height := float32(g.Ascent + g.Descent)
		scale := gglm.NewVec3(g.Advance, height, 1)

		//See: https://developer.apple.com/library/archive/documentation/TextFonts/Conceptual/CocoaTextArchitecture/Art/glyph_metrics_2x.png
		//Quads are drawn from the center and so that's our baseline. But chars shouldn't be centered, they should follow ascent/decent/advance.
		//To make them do that vertically, we raise them above the baseline (y+height/2), then since they are sitting on top of the baseline we can simply
		//move them down by the decent amount to put them in the correct vertical position.
		//
		//Horizontally the character should be drawn from the left edge not the center, so we just move it forward by advance/2
		drawPos := *pos
		drawPos.SetX(drawPos.X() + g.Advance*0.5)
		drawPos.SetY(drawPos.Y() + height*0.5 - g.Descent)

		instancedData = append(instancedData, []float32{
			g.U, g.V,
			g.U + g.SizeU, g.V,
			g.U, g.V + g.SizeV,
			g.U + g.SizeU, g.V + g.SizeV,

			color.R(), color.G(), color.B(), color.A(), //Color
			drawPos.X(), drawPos.Y(), drawPos.Z(), //Model pos
			scale.X(), scale.Y(), scale.Z(), //Model scale
		}...)

		pos.SetX(pos.X() + g.Advance)
	}

	gr.GlyphVBO = append(gr.GlyphVBO, instancedData...)
}

func (gr *GlyphRend) Draw() {

	if gr.GlyphCount == 0 {
		return
	}

	gr.InstancedBuf.SetData(gr.GlyphVBO)
	gr.InstancedBuf.Bind()
	gr.GlyphMat.Bind()

	gl.DrawElementsInstanced(gl.TRIANGLES, gr.GlyphMesh.Buf.IndexBufCount, gl.UNSIGNED_INT, gl.PtrOffset(0), gr.GlyphCount)

	gr.GlyphCount = 0
	gr.GlyphVBO = []float32{}
}

func (gr *GlyphRend) SetFace(fontOptions *truetype.Options) {
	face := truetype.NewFace(gr.Atlas.Font, fontOptions)
	gr.Atlas = NewFontAtlasFromFont(gr.Atlas.Font, face, uint(fontOptions.Size))
	gr.updateFontAtlasTexture("temp-atlas")
}

func (gr *GlyphRend) SetFontFromFile(fontFile string, fontOptions *truetype.Options) error {

	atlas, err := NewFontAtlasFromFile(fontFile, fontOptions)
	if err != nil {
		return err
	}

	gr.Atlas = atlas
	gr.updateFontAtlasTexture(fontFile)
	return nil
}

//updateFontAtlasTexture uploads the texture representing the font atlas to the GPU
//and updates the GlyphRend.AtlasTex field.
//
//Any old textures are deleted
func (gr *GlyphRend) updateFontAtlasTexture(fontFile string) error {

	if gr.AtlasTex != nil {
		gl.DeleteTextures(1, &gr.AtlasTex.TexID)
	}

	pngFileName := fontFile + "-atlas.png"
	err := SaveImgToPNG(gr.Atlas.Img, pngFileName)
	if err != nil {
		return err
	}
	defer os.Remove(pngFileName)

	atlasTex, err := assets.LoadPNGTexture(pngFileName)
	if err != nil {
		return err
	}
	gr.AtlasTex = &atlasTex

	//TODO: We want a function to load without caching. For now we clear manually
	delete(assets.Textures, atlasTex.TexID)
	delete(assets.TexturePaths, pngFileName)

	gl.BindTexture(gl.TEXTURE_2D, atlasTex.TexID)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.BindTexture(gl.TEXTURE_2D, 0)

	return nil
}

func (gr *GlyphRend) SetScreenSize(screenWidth, screenHeight int32) {

	gr.ScreenWidth = screenWidth
	gr.ScreenHeight = screenHeight

	//The projection matrix fits the screen size. This is needed so we can size and position characters correctly.
	projMtx := gglm.Ortho(0, float32(screenWidth), float32(screenHeight), 0, 0.1, 10)
	viewMtx := gglm.LookAt(gglm.NewVec3(0, 0, -1), gglm.NewVec3(0, 0, 0), gglm.NewVec3(0, 1, 0))
	projViewMtx := projMtx.Clone().Mul(viewMtx)

	gr.GlyphMat.DiffuseTex = gr.AtlasTex.TexID
	gr.GlyphMat.SetUnifMat4("projViewMat", &projViewMtx.Mat4)
}

func NewGlyphRend(fontFile string, fontOptions *truetype.Options, screenWidth, screenHeight int32) (*GlyphRend, error) {

	gr := &GlyphRend{
		GlyphCount: 0,
		GlyphVBO:   make([]float32, 0),
	}

	atlas, err := NewFontAtlasFromFile(fontFile, fontOptions)
	if err != nil {
		return nil, err
	}
	gr.Atlas = atlas

	err = gr.updateFontAtlasTexture(fontFile)
	if err != nil {
		return nil, err
	}

	//Create glyph mesh
	gr.GlyphMesh = &meshes.Mesh{
		Name: "glypQuad",

		//VertPos, UV, Color; Instanced attributes are stored separately
		Buf: buffers.NewBuffer(
			buffers.Element{ElementType: buffers.DataTypeVec3},
		),
	}

	gr.GlyphMesh.Buf.SetData([]float32{
		-0.5, -0.5, 0,
		0.5, -0.5, 0,
		-0.5, 0.5, 0,
		0.5, 0.5, 0,
	})

	gr.GlyphMesh.Buf.SetIndexBufData([]uint32{
		0, 1, 2,
		1, 3, 2,
	})

	//Setup material
	gr.GlyphMat = materials.NewMaterial("glyphMat", "./res/shaders/glyph")
	gr.GlyphMat.SetAttribute(gr.GlyphMesh.Buf)
	gr.GlyphMat.DiffuseTex = gr.AtlasTex.TexID

	//Create instanced buf and set its instanced attributes.
	//Multiple VBOs under one VAO, one VBO for vertex data, and one VBO for instanced data.
	gr.InstancedBuf = buffers.Buffer{
		VAOID: gr.GlyphMesh.Buf.VAOID,
	}
	gr.InstancedBuf.SetLayout(
		buffers.Element{ElementType: buffers.DataTypeVec2}, //UVST0
		buffers.Element{ElementType: buffers.DataTypeVec2}, //UVST1
		buffers.Element{ElementType: buffers.DataTypeVec2}, //UVST2
		buffers.Element{ElementType: buffers.DataTypeVec2}, //UVST3

		buffers.Element{ElementType: buffers.DataTypeVec4}, //Color
		buffers.Element{ElementType: buffers.DataTypeVec3}, //ModelPos
		buffers.Element{ElementType: buffers.DataTypeVec3}, //ModelScale
	)

	gl.GenBuffers(1, &gr.InstancedBuf.BufID)
	if gr.InstancedBuf.BufID == 0 {
		return nil, errors.New("failed to create OpenGL VBO buffer")
	}

	gr.InstancedBuf.Bind()
	gl.BindBuffer(gl.ARRAY_BUFFER, gr.InstancedBuf.BufID)
	layout := gr.InstancedBuf.GetLayout()

	//4 UV values
	uvEle := layout[0]
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(1, uvEle.ElementType.CompCount(), uvEle.ElementType.GLType(), false, gr.InstancedBuf.Stride, gl.PtrOffset(uvEle.Offset))
	gl.VertexAttribDivisor(1, 1)

	uvEle = layout[1]
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointer(2, uvEle.ElementType.CompCount(), uvEle.ElementType.GLType(), false, gr.InstancedBuf.Stride, gl.PtrOffset(uvEle.Offset))
	gl.VertexAttribDivisor(2, 1)

	uvEle = layout[2]
	gl.EnableVertexAttribArray(3)
	gl.VertexAttribPointer(3, uvEle.ElementType.CompCount(), uvEle.ElementType.GLType(), false, gr.InstancedBuf.Stride, gl.PtrOffset(uvEle.Offset))
	gl.VertexAttribDivisor(3, 1)

	uvEle = layout[3]
	gl.EnableVertexAttribArray(4)
	gl.VertexAttribPointer(4, uvEle.ElementType.CompCount(), uvEle.ElementType.GLType(), false, gr.InstancedBuf.Stride, gl.PtrOffset(uvEle.Offset))
	gl.VertexAttribDivisor(4, 1)

	//Rest of instanced attributes
	colorEle := layout[4]
	gl.EnableVertexAttribArray(5)
	gl.VertexAttribPointer(5, colorEle.ElementType.CompCount(), colorEle.ElementType.GLType(), false, gr.InstancedBuf.Stride, gl.PtrOffset(colorEle.Offset))
	gl.VertexAttribDivisor(5, 1)

	posEle := layout[5]
	gl.EnableVertexAttribArray(6)
	gl.VertexAttribPointer(6, posEle.ElementType.CompCount(), posEle.ElementType.GLType(), false, gr.InstancedBuf.Stride, gl.PtrOffset(posEle.Offset))
	gl.VertexAttribDivisor(6, 1)

	scaleEle := layout[6]
	gl.EnableVertexAttribArray(7)
	gl.VertexAttribPointer(7, scaleEle.ElementType.CompCount(), scaleEle.ElementType.GLType(), false, gr.InstancedBuf.Stride, gl.PtrOffset(scaleEle.Offset))
	gl.VertexAttribDivisor(7, 1)

	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gr.InstancedBuf.UnBind()

	gr.SetScreenSize(screenWidth, screenHeight)
	return gr, nil
}
