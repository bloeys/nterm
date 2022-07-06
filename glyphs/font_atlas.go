package glyphs

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"unicode"

	"github.com/bloeys/gglm/gglm"
	"github.com/bloeys/nterm/assert"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type FontAtlas struct {
	Font   *truetype.Font
	Img    *image.RGBA
	Glyphs map[rune]FontAtlasGlyph

	//Advance is global to the atlas because we only support monospaced fonts
	Advance    int
	LineHeight int
	SizeUV     gglm.Vec2
}

type FontAtlasGlyph struct {
	U float32
	V float32

	Ascent   float32
	Descent  float32
	BearingX float32
}

//NewFontAtlasFromFile reads a TTF or TTC file and produces a font texture atlas containing
//all its characters using the specified options. The atlas uses equally sized tiles
//such that all characters use an equal horizontal/vertical on the atlas.
//If the character is smaller than the tile then the rest of the tile is empty.
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
//all its characters using the specified options. The atlas uses equally sized tiles
//such that all characters use an equal horizontal/vertical on the atlas.
//If the character is smaller than the tile then the rest of the tile is empty.
//
//Only monospaced fonts are supported.
func NewFontAtlasFromFont(f *truetype.Font, face font.Face, pointSize uint) (*FontAtlas, error) {

	const maxAtlasSize = 8192

	glyphs := getGlyphsFromRuneRanges(getGlyphRangesFromFont(f))
	assert.T(len(glyphs) > 0, "no glyphs")

	//Find advance and line height
	const charPaddingX = 2
	const charPaddingY = 2
	charAdvFixed, _ := face.GlyphAdvance('L')
	charAdv := charAdvFixed.Ceil() + charPaddingX

	//Find largest vertical character.
	//We don't use face.Metrics().Height because its not reliable
	lineHeightFixed := fixed.Int26_6(0)
	for _, g := range glyphs {

		gBounds, _, _ := face.GlyphBounds(g)
		ascent := absFixedI26_6(gBounds.Min.Y)
		descent := absFixedI26_6(gBounds.Max.Y)

		charHeight := ascent + descent
		if charHeight > lineHeightFixed {
			lineHeightFixed = charHeight
		}
	}
	lineHeightFixed = fixed.I(lineHeightFixed.Ceil())
	lineHeight := lineHeightFixed.Ceil()

	//Calculate needed atlas size
	atlasSizeX := 128
	atlasSizeY := 128

	maxLinesInAtlas := atlasSizeY/lineHeight - 2
	charsPerLine := atlasSizeX/charAdv - 1
	linesNeeded := int(math.Ceil(float64(len(glyphs))/float64(charsPerLine))) + 1

	for linesNeeded > maxLinesInAtlas {

		atlasSizeX *= 2
		atlasSizeY *= 2

		maxLinesInAtlas = atlasSizeY/lineHeight - 2

		charsPerLine = atlasSizeX/charAdv - 1
		linesNeeded = int(math.Ceil(float64(len(glyphs))/float64(charsPerLine))) + 1
	}

	if atlasSizeX > maxAtlasSize {
		return nil, errors.New("atlas size went beyond the maximum of 8192*8192")
	}

	//Create atlas
	// atlasSizeXF32 := float32(atlasSizeX)
	atlasSizeYF32 := float32(atlasSizeY)
	atlas := &FontAtlas{
		Font:   f,
		Img:    image.NewRGBA(image.Rect(0, 0, atlasSizeX, atlasSizeY)),
		Glyphs: make(map[rune]FontAtlasGlyph, len(glyphs)),

		Advance:    charAdv - charPaddingX,
		LineHeight: lineHeight,
		SizeUV:     *gglm.NewVec2(float32(charAdv-charPaddingX), float32(lineHeight)),
		// SizeUV:     *gglm.NewVec2(float32(charAdv-charPaddingX)/atlasSizeXF32, float32(lineHeight)/atlasSizeYF32),
	}

	//Clear background to black
	draw.Draw(atlas.Img, atlas.Img.Bounds(), image.Black, image.Point{}, draw.Src)
	drawer := &font.Drawer{
		Dst:  atlas.Img,
		Src:  image.White,
		Face: face,
	}

	//Put glyphs on atlas
	charPaddingXFixed := fixed.I(charPaddingX)
	charPaddingYFixed := fixed.I(charPaddingY)

	charsOnLine := 0
	drawer.Dot = fixed.P(atlas.Advance+charPaddingX, lineHeight)

	drawHorizontalLines := true
	drawVerticalLines := true
	for _, g := range glyphs {

		gBounds, _, _ := face.GlyphBounds(g)
		ascent := absFixedI26_6(gBounds.Min.Y)
		descent := absFixedI26_6(gBounds.Max.Y)
		bearingX := absFixedI26_6(gBounds.Min.X)

		atlas.Glyphs[g] = FontAtlasGlyph{
			U: float32((drawer.Dot.X).Floor()),
			V: (atlasSizeYF32 - float32((drawer.Dot.Y).Ceil())),
			// U: float32((drawer.Dot.X).Floor()) / atlasSizeXF32,
			// V: (atlasSizeYF32 - float32((drawer.Dot.Y).Ceil())) / atlasSizeYF32,

			Ascent:   float32(ascent.Ceil()),
			Descent:  float32(descent.Ceil()),
			BearingX: float32(bearingX.Ceil()),
		}

		//Get glyph to draw but undo any applied descent so that the glyph is drawn sitting on the line exactly.
		//Bearing will be applied correctly but descent will be the responsibility of the positioning code
		imgRect, mask, maskp, _, _ := face.Glyph(drawer.Dot, g)
		if imgRect.Max.Y > drawer.Dot.Y.Ceil() {
			diff := imgRect.Max.Y - drawer.Dot.Y.Ceil()
			imgRect.Min.Y -= diff
			imgRect.Max.Y -= diff
		}

		if drawVerticalLines {
			rectCopy := imgRect
			rectCopy.Min.Y = 0
			rectCopy.Max.Y = drawer.Dst.Bounds().Max.Y
			rectCopy.Max.X = rectCopy.Min.X + 1
			oldPos := drawer.Dot
			drawer.Dot.Y = 0
			// fmt.Printf("Drawing with maskP %s\n", maskp.String())
			draw.Draw(drawer.Dst, rectCopy, image.NewUniform(color.NRGBA{G: 255, A: 255}), image.Point{}, draw.Over)
			drawer.Dot = oldPos
		}

		//Draw glyph and advance dot
		draw.DrawMask(drawer.Dst, imgRect, drawer.Src, image.Point{}, mask, maskp, draw.Over)
		drawer.Dot.X += fixed.I(atlas.Advance) + charPaddingXFixed

		charsOnLine++
		if charsOnLine == charsPerLine {

			if drawHorizontalLines {
				rectCopy := imgRect
				rectCopy.Min.X = 0
				rectCopy.Max.X = drawer.Dst.Bounds().Max.X

				// rectCopy.Min.Y += (lineHeightFixed + charPaddingYFixed).Floor() * 1
				rectCopy.Max.Y = rectCopy.Min.Y + 1

				oldPos := drawer.Dot
				drawer.Dot.X = 0
				draw.Draw(drawer.Dst, rectCopy, image.NewUniform(color.NRGBA{G: 255, A: 255}), image.Point{}, draw.Over)
				drawer.Dot = oldPos

				drawVerticalLines = false
			}

			charsOnLine = 0
			drawer.Dot.X = fixed.I(atlas.Advance) + charPaddingXFixed
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
