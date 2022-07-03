package glyphs

import (
	"errors"
	"math"

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

	//Prepass to pre-allocate the buffer
	rs := []rune(text)
	const floatsPerGlyph = 18

	// startPos := screenPos.Clone()
	pos := screenPos.Clone()
	advanceF32 := float32(gr.Atlas.Advance)
	lineHeightF32 := float32(gr.Atlas.LineHeight)
	instancedData := make([]float32, 0, len(rs)*floatsPerGlyph) //This a larger approximation than needed because we don't count spaces etc
	for i := 0; i < len(rs); i++ {

		r := rs[i]
		g := gr.Atlas.Glyphs[r]
		if r == '\n' {
			screenPos.SetY(screenPos.Y() - lineHeightF32)
			pos = screenPos.Clone()
			continue
		} else if r == ' ' {
			pos.SetX(pos.X() + advanceF32)
			continue
		}
		gr.GlyphCount++

		scale := gglm.NewVec3(advanceF32, lineHeightF32, 1)

		//See: https://developer.apple.com/library/archive/documentation/TextFonts/Conceptual/CocoaTextArchitecture/Art/glyph_metrics_2x.png
		//The uvs coming in make it so that glyphs are sitting on top of the baseline (no descent) and with horizontal bearing applied.
		//So to position correctly we move them down by the descent amount.
		drawPos := *pos
		drawPos.SetX(drawPos.X())
		drawPos.SetY(drawPos.Y() - g.Descent)

		instancedData = append(instancedData, []float32{
			g.U, g.V,
			color.R(), color.G(), color.B(), color.A(), //Color
			roundF32(drawPos.X()), roundF32(drawPos.Y()), drawPos.Z(), //Model pos
			scale.X(), scale.Y(), scale.Z(), //Model scale
		}...)

		pos.SetX(pos.X() + advanceF32)
	}

	//Draw baselines
	// g := gr.Atlas.Glyphs['-']
	// lineData := []float32{
	// 	g.U, g.V,
	// 	g.U + g.SizeU, g.V,
	// 	g.U, g.V + g.SizeV,
	// 	g.U + g.SizeU, g.V + g.SizeV,

	// 	1, 0, 0, 1, //Color
	// 	0, startPos.Y(), 1, //Model pos
	// 	float32(gr.ScreenWidth), 5, 1, //Model scale
	// }

	// instancedData = append(instancedData, lineData...)
	// lineData[13] -= float32(gr.Atlas.LineHeight)
	// instancedData = append(instancedData, lineData...)
	// gr.GlyphCount++
	// gr.GlyphCount++

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
		GlyphCount: 0,
		GlyphVBO:   make([]float32, 0),
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
		buffers.Element{ElementType: buffers.DataTypeVec3}, //ModelScale
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
	// fmt.Printf("lineHeight=%d, glyphInfo=%+v\n", gr.Atlas.LineHeight, gr.Atlas.Glyphs['A'])
	return gr, nil
}

func roundF32(x float32) float32 {
	return float32(math.Round(float64(x)))
}
