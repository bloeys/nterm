package glyphs

import (
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"

	"github.com/bloeys/nterm/assert"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type FontAtlas struct {
	Font       *truetype.Font
	Img        *image.RGBA
	Glyphs     map[rune]FontAtlasGlyph
	LineHeight int
}

type FontAtlasGlyph struct {
	U     float32
	V     float32
	SizeU float32
	SizeV float32

	Ascent  float32
	Descent float32
	Advance float32
}

func NewFontAtlasFromFile(fontFile string, fontOptions *truetype.Options) (*FontAtlas, error) {

	fBytes, err := os.ReadFile(fontFile)
	if err != nil {
		return nil, err
	}

	f, err := truetype.Parse(fBytes)
	if err != nil {
		return nil, err
	}

	face := truetype.NewFace(f, fontOptions)
	atlas := NewFontAtlasFromFont(f, face, uint(fontOptions.Size))
	return atlas, nil
}

func NewFontAtlasFromFont(f *truetype.Font, face font.Face, pointSize uint) *FontAtlas {

	const maxAtlasSize = 8192

	glyphs := getGlyphsFromRanges(getGlyphRanges(f))

	assert.T(len(glyphs) > 0, "no glyphs")

	//Choose atlas size
	atlasSizeX := 512
	atlasSizeY := 512

	_, charWidthFixed, _ := face.GlyphBounds(glyphs[0])
	charWidth := charWidthFixed.Floor()
	lineHeight := face.Metrics().Height.Floor()

	maxLinesInAtlas := atlasSizeY/lineHeight - 1
	charsPerLine := atlasSizeX / charWidth
	linesNeeded := int(math.Ceil(float64(len(glyphs)) / float64(charsPerLine)))

	for linesNeeded > maxLinesInAtlas {

		atlasSizeX *= 2
		atlasSizeY *= 2

		maxLinesInAtlas = atlasSizeY/lineHeight - 1

		charsPerLine = atlasSizeX / charWidth
		linesNeeded = int(math.Ceil(float64(len(glyphs)) / float64(charsPerLine)))
	}
	assert.T(atlasSizeX <= maxAtlasSize, "Atlas size went beyond maximum")

	//Create atlas
	atlas := &FontAtlas{
		Font:       f,
		Img:        image.NewRGBA(image.Rect(0, 0, atlasSizeX, atlasSizeY)),
		Glyphs:     make(map[rune]FontAtlasGlyph, len(glyphs)),
		LineHeight: lineHeight,
	}

	//Clear background to black
	draw.Draw(atlas.Img, atlas.Img.Bounds(), image.Black, image.Point{}, draw.Src)

	drawer := &font.Drawer{
		Dst:  atlas.Img,
		Src:  image.White,
		Face: face,
	}

	//Put glyphs on atlas
	atlasSizeXF32 := float32(atlasSizeX)
	atlasSizeYF32 := float32(atlasSizeY)

	charsOnLine := 0
	lineDx := fixed.P(0, lineHeight)
	drawer.Dot = fixed.P(0, lineHeight)
	for _, g := range glyphs {

		gBounds, gAdvanceFixed, _ := face.GlyphBounds(g)

		descent := gBounds.Max.Y
		advanceRoundedF32 := float32(gAdvanceFixed.Floor())
		ascent := -gBounds.Min.Y

		heightRounded := (ascent + descent).Floor()

		atlas.Glyphs[g] = FontAtlasGlyph{
			U: float32(drawer.Dot.X.Floor()) / atlasSizeXF32,
			V: (atlasSizeYF32 - float32((drawer.Dot.Y + descent).Floor())) / atlasSizeYF32,

			SizeU: advanceRoundedF32 / atlasSizeXF32,
			SizeV: float32(heightRounded) / atlasSizeYF32,

			Ascent:  float32(ascent.Floor()),
			Descent: float32(descent.Floor()),
			Advance: float32(advanceRoundedF32),
		}
		drawer.DrawString(string(g))

		charsOnLine++
		if charsOnLine == charsPerLine {

			charsOnLine = 0
			drawer.Dot.X = 0
			drawer.Dot = drawer.Dot.Add(lineDx)
		}
	}

	return atlas
}

func SaveImgToPNG(img image.Image, file string) error {

	outFile, err := os.Create(file)
	if err != nil {
		return err
	}
	defer outFile.Close()

	err = png.Encode(outFile, img)
	if err != nil {
		return err
	}

	return nil
}
