package glyphs

import (
	"errors"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"unicode"

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

	Ascent   float32
	Descent  float32
	Advance  float32
	BearingX float32
	Width    float32
}

//NewFontAtlasFromFile reads a TTF or TTC file and produces a font texture atlas containing
//all its characters using the specified options.
//
//Only monospaced fonts are supported
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
	return NewFontAtlasFromFont(f, face, uint(fontOptions.Size))
}

//NewFontAtlasFromFile uses the passed font to produce a font texture atlas containing
//all its characters using the specified options.
//
//Only monospaced fonts are supported
func NewFontAtlasFromFont(f *truetype.Font, face font.Face, pointSize uint) (*FontAtlas, error) {

	const maxAtlasSize = 8192

	glyphs := getGlyphsFromRuneRanges(getGlyphRangesFromFont(f))
	assert.T(len(glyphs) > 0, "no glyphs")

	//Choose atlas size
	atlasSizeX := 512
	atlasSizeY := 512

	const charPaddingX = 2
	const charPaddingY = 2
	charAdvFixed, _ := face.GlyphAdvance('L')
	charAdv := charAdvFixed.Ceil() + charPaddingX

	lineHeight := face.Metrics().Height.Ceil()

	maxLinesInAtlas := atlasSizeY/lineHeight - 1
	charsPerLine := atlasSizeX / charAdv
	linesNeeded := int(math.Ceil(float64(len(glyphs)) / float64(charsPerLine)))

	for linesNeeded > maxLinesInAtlas {

		atlasSizeX *= 2
		atlasSizeY *= 2

		maxLinesInAtlas = atlasSizeY/lineHeight - 1

		charsPerLine = atlasSizeX / charAdv
		linesNeeded = int(math.Ceil(float64(len(glyphs)) / float64(charsPerLine)))
	}

	if atlasSizeX > maxAtlasSize {
		return nil, errors.New("atlas size went beyond the maximum of 8192*8192")
	}

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
	charPaddingXFixed := fixed.I(charPaddingX)
	charPaddingYFixed := fixed.I(charPaddingY)

	charsOnLine := 0
	lineHeightFixed := fixed.I(lineHeight)
	drawer.Dot = fixed.P(0, lineHeight)
	for _, g := range glyphs {

		gBounds, gAdvanceFixed, _ := face.GlyphBounds(g)
		advanceCeilF32 := float32(gAdvanceFixed.Ceil())

		ascent := absFixedI26_6(gBounds.Min.Y)
		descent := absFixedI26_6(gBounds.Max.Y)
		bearingX := absFixedI26_6(gBounds.Min.X)

		glyphWidth := float32((absFixedI26_6(gBounds.Max.X) - absFixedI26_6(gBounds.Min.X)).Ceil())
		heightRounded := (ascent + descent).Ceil()

		atlas.Glyphs[g] = FontAtlasGlyph{
			U: float32((drawer.Dot.X + bearingX).Floor()) / atlasSizeXF32,
			V: (atlasSizeYF32 - float32((drawer.Dot.Y + descent).Ceil())) / atlasSizeYF32,

			SizeU: glyphWidth / atlasSizeXF32,
			SizeV: float32(heightRounded) / atlasSizeYF32,

			Ascent:  float32(ascent.Ceil()),
			Descent: float32(descent.Ceil()),
			Advance: float32(advanceCeilF32),

			BearingX: float32(bearingX.Ceil()),
			Width:    glyphWidth,
		}

		// z := atlas.Glyphs[g]
		// fmt.Printf("c=%s; u=%f, v=%f, sizeU=%f, sizeV=%f; x=%d, y=%d, w=%f, h=%f\n", string(g), z.U, z.V, z.SizeU, z.SizeV, int(z.U*atlasSizeXF32), int(z.V*atlasSizeYF32), z.SizeU*atlasSizeXF32, z.SizeV*atlasSizeYF32)

		drawer.DrawString(string(g))
		drawer.Dot.X += charPaddingXFixed

		charsOnLine++
		if charsOnLine == charsPerLine {

			charsOnLine = 0
			drawer.Dot.X = 0
			drawer.Dot.Y += lineHeightFixed + charPaddingYFixed
		}
	}

	return atlas, nil
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

//getGlyphRangesFromFont returns a list of ranges, each range is: [i][0]<=range<[i][1]
func getGlyphRangesFromFont(f *truetype.Font) (ret [][2]rune) {

	isRuneInPrivateUseArea := func(r rune) bool {
		return 0xe000 <= r && r <= 0xf8ff ||
			0xf0000 <= r && r <= 0xffffd ||
			0x100000 <= r && r <= 0x10fffd
	}

	rr := [2]rune{-1, -1}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if isRuneInPrivateUseArea(r) {
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

//getGlyphsFromRuneRanges takes ranges of runes and produces an array of all the runes in these ranges
func getGlyphsFromRuneRanges(ranges [][2]rune) []rune {

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

func absFixedI26_6(x fixed.Int26_6) fixed.Int26_6 {
	if x < 0 {
		return -x
	}

	return x
}
