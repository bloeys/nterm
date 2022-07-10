package glyphs

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"unicode"

	"github.com/bloeys/nterm/assert"
	"github.com/bloeys/nterm/consts"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type FontAtlas struct {
	Font   *truetype.Font
	Face   font.Face
	Img    *image.RGBA
	Glyphs map[rune]FontAtlasGlyph

	//SpaceAdvance is global to the atlas because we only support monospaced fonts
	SpaceAdvance float32
	LineHeight   float32
}

type FontAtlasGlyph struct {
	Rune  rune
	U     float32
	V     float32
	SizeU float32
	SizeV float32

	Ascent   float32
	Descent  float32
	BearingX float32
	Advance  float32
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

func calcNeededAtlasSize(glyphs []rune, face font.Face, charPaddingXFixed, charPaddingYFixed fixed.Int26_6) (atlasSizeX, atlasSizeY int) {

	//Calculate needed atlas size
	atlasSizeX = 512
	atlasSizeY = 512
	lineHeight := face.Metrics().Height
	foundAtlasSize := false
	for !foundAtlasSize {

		foundAtlasSize = true
		dotX := charPaddingXFixed
		dotY := lineHeight
		atlasSizeXFixed := fixed.I(atlasSizeX)
		atlasSizeYFixed := fixed.I(atlasSizeY)
		for i := 0; i < len(glyphs); i++ {

			//Prepare all glyph metrics
			g := glyphs[i]
			gBounds, _, _ := face.GlyphBounds(g)
			bearingXFixed := gBounds.Min.X
			gWidthFixed := gBounds.Max.X - gBounds.Min.X
			// descent := gBounds.Max.Y

			// Calculate distance dot will move after drawing. Advance normally if line has space,
			// otherwise go to next line and reset X position.
			distToMoveX := bearingXFixed + gWidthFixed + charPaddingXFixed

			//If bearing is negative this char might overlap with the previous one.
			//So we need to move the dot so the drawer won't overlap even after a negative offset
			if bearingXFixed < 0 {
				distToMoveX += absI26_6(bearingXFixed)
			}

			//If we hav eno more space go to next line
			if dotX+distToMoveX >= atlasSizeXFixed {

				dotX = distToMoveX
				dotY += lineHeight + charPaddingYFixed

				//If we have only one more empty line then resize to be safe against descents being clipped
				if dotY+lineHeight >= atlasSizeYFixed {
					atlasSizeX *= 2
					atlasSizeY *= 2
					foundAtlasSize = false
					break
				}
			} else {
				dotX += distToMoveX
			}
		}
	}

	return atlasSizeX, atlasSizeY
}

//NewFontAtlasFromFile uses the passed font to produce a font texture atlas containing
//all its characters using the specified options. The atlas uses equally sized tiles
//such that all characters use an equal horizontal/vertical on the atlas.
//If the character is smaller than the tile then the rest of the tile is empty.
//
//Only monospaced fonts are supported.
func NewFontAtlasFromFont(f *truetype.Font, face font.Face, pointSize uint) (*FontAtlas, error) {

	// Vertical padding must be a bit larger because low descent on one line
	// and high ascent on the next might cause overlapping chars
	const charPaddingXFixed = 4 << 6
	const charPaddingYFixed = 4 << 6
	const maxAtlasSize = 8192

	glyphs := getGlyphsFromRuneRanges(getGlyphRangesFromFont(f))
	assert.T(len(glyphs) > 0, "no glyphs")

	atlasSizeX, atlasSizeY := calcNeededAtlasSize(glyphs, face, charPaddingXFixed, charPaddingYFixed)
	if atlasSizeX > maxAtlasSize {
		return nil, errors.New("atlas size went beyond the maximum of 8192*8192")
	}

	//Create atlas
	lineHeight := face.Metrics().Height
	spaceAdv, _ := face.GlyphAdvance(' ')
	atlas := &FontAtlas{
		Font:   f,
		Face:   face,
		Img:    image.NewRGBA(image.Rect(0, 0, atlasSizeX, atlasSizeY)),
		Glyphs: make(map[rune]FontAtlasGlyph, len(glyphs)),

		SpaceAdvance: float32(spaceAdv.Ceil()),
		LineHeight:   float32(lineHeight.Ceil()),
	}

	//Clear background to black
	draw.Draw(atlas.Img, atlas.Img.Bounds(), image.Black, image.Point{}, draw.Src)
	drawer := &font.Drawer{
		Dst:  atlas.Img,
		Src:  image.White,
		Face: face,
	}

	//Put glyphs on atlas
	drawer.Dot = fixed.P(int(atlas.SpaceAdvance), 0)
	drawer.Dot.X += charPaddingXFixed
	drawer.Dot.Y = lineHeight

	const drawBoundingBoxes bool = false
	atlasSizeXFixed := fixed.I(atlasSizeX)
	atlasSizeYFixed := fixed.I(atlasSizeY)
	for _, g := range glyphs {

		//Glyph metrics
		gBounds, gAdvanceFixed, _ := face.GlyphBounds(g)
		bearingXFixed := gBounds.Min.X
		ascentAbsFixed := absI26_6(gBounds.Min.Y)
		descentAbsFixed := absI26_6(gBounds.Max.Y)
		gWidthFixed := gBounds.Max.X - gBounds.Min.X

		//If bearing is negative this char might overlap with the previous one.
		//So we need to move the dot so the drawer won't overlap even after a negative offset
		if bearingXFixed < 0 {
			drawer.Dot.X += absI26_6(bearingXFixed)
		}

		// Position dot by calculating how much it will move after drawing, and if there isn't enough space
		// move to next line then draw
		nextDotPosDeltaX := bearingXFixed + gWidthFixed + charPaddingXFixed
		if drawer.Dot.X+nextDotPosDeltaX >= atlasSizeXFixed {

			assert.T(drawer.Dot.Y+lineHeight < atlasSizeYFixed, "Failed to create atlas because it did not fit")

			drawer.Dot.X = charPaddingXFixed
			if bearingXFixed < 0 {
				drawer.Dot.X += absI26_6(bearingXFixed)
			}

			drawer.Dot.Y += lineHeight + charPaddingYFixed
		}

		drawer.Dot = fixed.P(drawer.Dot.X.Round(), drawer.Dot.Y.Round())

		//Build and insert glyph struct
		gTopLeft := image.Point{
			X: (drawer.Dot.X + bearingXFixed).Floor(),
			Y: (drawer.Dot.Y - ascentAbsFixed).Floor(),
		}

		gBotRight := image.Point{
			X: (drawer.Dot.X + bearingXFixed + gWidthFixed).Ceil(),
			Y: (drawer.Dot.Y + descentAbsFixed).Ceil(),
		}

		atlas.Glyphs[g] = FontAtlasGlyph{
			Rune:  g,
			U:     float32(gTopLeft.X),
			V:     float32(atlasSizeY - gBotRight.Y),
			SizeU: float32(gBotRight.X - gTopLeft.X),
			SizeV: float32(gBotRight.Y - gTopLeft.Y),

			Ascent:   I26_6ToF32(ascentAbsFixed),
			Descent:  I26_6ToF32(descentAbsFixed),
			BearingX: I26_6ToF32(bearingXFixed),
			Advance:  I26_6ToF32(gAdvanceFixed),
		}

		if consts.Mode_Debug && drawBoundingBoxes {
			rect := image.Rectangle{
				Min: gTopLeft,
				Max: gBotRight,
			}
			drawRectOutline(atlas.Img, rect, color.NRGBA{B: 255, A: 128})
		}

		//Draw glyph
		imgRect, mask, maskp, _, _ := face.Glyph(drawer.Dot, g)
		draw.DrawMask(drawer.Dst, imgRect, drawer.Src, image.Point{}, mask, maskp, draw.Over)
		drawer.Dot.X += nextDotPosDeltaX
	}

	// // This is a test section that uses the drawer to draw an Arabic
	// // string at the bottom of the atlas. Useful to compare glyph renderer against a 'correct' implementation
	// str := "السلام عليكم"
	// rs := []rune(str)
	// finalR := make([]rune, 0)
	// prevRune := invalidRune
	// for i := len(rs) - 1; i >= 0; i-- {
	// 	var g FontAtlasGlyph
	// 	if i > 0 {
	// 		//start or middle of sentence
	// 		g = GlyphFromRunes(atlas.Glyphs, rs[i], rs[i-1], prevRune)
	// 	} else {
	// 		//Last character
	// 		g = GlyphFromRunes(atlas.Glyphs, rs[i], invalidRune, prevRune)
	// 	}
	// 	prevRune = rs[i]
	// 	finalR = append(finalR, g.R)
	// }
	// drawer.Dot.Y += lineHeightFixed + charPaddingYFixed
	// drawer.DrawString(string(finalR))

	return atlas, nil
}

func drawRectOutline(img *image.RGBA, rect image.Rectangle, color color.NRGBA) {

	rowPixCount := img.Stride / 4

	topLeft := img.PixOffset(rect.Min.X, rect.Min.Y)
	botRight := img.PixOffset(rect.Max.X, rect.Max.Y)

	for i := topLeft; i <= botRight; i += 4 {

		pixel := i / 4
		y := pixel / rowPixCount
		x := pixel - y*rowPixCount

		if x >= rect.Min.X && x <= rect.Max.X && y >= rect.Min.Y && y <= rect.Max.Y {
			if x == rect.Min.X || x == rect.Max.X || y == rect.Min.Y || y == rect.Max.Y {
				img.Pix[i+3] = color.A
				img.Pix[i+2] = color.B
				img.Pix[i+1] = color.G
				img.Pix[i+0] = color.R
			}
		}
	}
}

func DrawRect(img *image.RGBA, rect image.Rectangle, color color.NRGBA) {

	rowPixCount := img.Stride / 4

	topLeft := img.PixOffset(rect.Min.X, rect.Min.Y)
	botRight := img.PixOffset(rect.Max.X, rect.Max.Y)

	//Draw top line
	for i := topLeft; i <= botRight; i += 4 {

		pixel := i / 4
		y := pixel / rowPixCount
		x := pixel - y*rowPixCount

		if x >= rect.Min.X && x <= rect.Max.X && y >= rect.Min.Y && y <= rect.Max.Y {
			img.Pix[i+3] = color.A
			img.Pix[i+2] = color.B
			img.Pix[i+1] = color.G
			img.Pix[i+0] = color.R
		}
	}
}

func I26_6ToF32(x fixed.Int26_6) float32 {
	const lower6BitMask = 1<<6 - 1

	if x > 0 {
		return float32(x.Floor()) + float32(x&lower6BitMask)/64
	} else {
		return float32(x.Floor()) - float32(x&lower6BitMask)/64
	}
}

func DrawVerticalLine(img *image.RGBA, posX int, color color.NRGBA) {

	rowLength := img.Stride
	start := img.PixOffset(posX, 0)

	for i := start; i < len(img.Pix); i += rowLength {

		img.Pix[i+3] = color.A
		img.Pix[i+2] = color.B
		img.Pix[i+1] = color.G
		img.Pix[i+0] = color.R
	}
}

func DrawHorizontalLine(img *image.RGBA, posY int, color color.NRGBA) {

	rowLength := img.Stride
	start := img.PixOffset(0, posY)

	//Horizontal line
	for i := start; i < start+rowLength; i += 4 {

		img.Pix[i+3] = color.A
		img.Pix[i+2] = color.B
		img.Pix[i+1] = color.G
		img.Pix[i+0] = color.R
	}
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

func absI26_6(x fixed.Int26_6) fixed.Int26_6 {
	if x < 0 {
		return -x
	}

	return x
}
