package main

import (
	"fmt"
	"unicode/utf8"

	"github.com/bloeys/gglm/gglm"
)

type GridTile struct {
	Glyph   rune
	FgColor gglm.Vec4
	BgColor gglm.Vec4
}

type GlyphGrid struct {
	CursorX uint
	CursorY uint
	SizeX   uint
	SizeY   uint
	Tiles   [][]GridTile
}

func (gg *GlyphGrid) Write(rs []rune, fgColor *gglm.Vec4, bgColor *gglm.Vec4) {

	for i := 0; i < len(rs); i++ {

		r := rs[i]
		gg.Tiles[gg.CursorY][gg.CursorX] = GridTile{
			Glyph:   r,
			FgColor: *fgColor,
			BgColor: *bgColor,
		}

		if !gg.TickCursor(r == '\n') {
			break
		}
	}
}

func (gg *GlyphGrid) ClearRow(rowIndex uint) {

	if rowIndex >= gg.SizeY {
		panic(fmt.Sprintf("passed row index of %d is larger or equal than grid Y size of %d\n", rowIndex, gg.SizeY))
	}

	row := gg.Tiles[rowIndex]
	for x := 0; x < len(row); x++ {
		row[x].Glyph = utf8.RuneError
	}
}

func (gg *GlyphGrid) ClearAll() {

	for y := 0; y < len(gg.Tiles); y++ {
		row := gg.Tiles[y]
		for x := 0; x < len(row); x++ {
			row[x].Glyph = utf8.RuneError
		}
	}
}

func (gg *GlyphGrid) SetCursor(x, y uint) {

	if x > gg.SizeX || y > gg.SizeY {
		panic("cursor position can not be larger than grid size")
	}

	gg.CursorX = x
	gg.CursorY = y
}

func (gg *GlyphGrid) TickCursor(forceDown bool) (success bool) {

	if gg.CursorX == gg.SizeX-1 && gg.CursorY == gg.SizeY-1 {
		// fmt.Println("trying to advance cursor beyond grid which is not allowed. Keeping cursor at position")
		return false
	}

	if forceDown {

		if gg.CursorY == gg.SizeY-1 {
			// fmt.Println("trying to move cursor to next line but cursor already at last line. Keeping cursor at position")
			return false
		}

		gg.CursorX = 0
		gg.CursorY++
		return true
	}

	gg.CursorX++
	if gg.CursorX >= gg.SizeX {
		gg.CursorX = 0
		gg.CursorY++
	}

	return true
}

func (gg *GlyphGrid) Print() {

	fmt.Println("\n---")
	for y := 0; y < len(gg.Tiles); y++ {

		row := gg.Tiles[y]
		for x := 0; x < len(row); x++ {

			if row[x].Glyph == utf8.RuneError {
				break
			}
			fmt.Print(string(row[x].Glyph))
		}

		fmt.Print("\n")
	}
	fmt.Println("---")
}

func NewGlyphGrid(width, height uint) *GlyphGrid {

	if width == 0 || height == 0 {
		panic("glyph grid width and height must be larger than zero")
	}

	tiles := make([][]GridTile, height)
	for i := 0; i < len(tiles); i++ {
		tiles[i] = make([]GridTile, width)
	}

	return &GlyphGrid{
		CursorX: 0,
		CursorY: 0,
		SizeX:   width,
		SizeY:   height,
		Tiles:   tiles,
	}
}
