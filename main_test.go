package main_test

import (
	"testing"

	"github.com/bloeys/nterm/glyphs"
	"golang.org/x/image/math/fixed"
)

func TestI26_6ToF32(t *testing.T) {

	x := fixed.I(55)
	var ans float32 = 55
	Check(t, ans, glyphs.I26_6ToF32(x))

	x = fixed.I(-10)
	ans = -10
	Check(t, ans, glyphs.I26_6ToF32(x))

	x = fixed.Int26_6(0<<6 + 1<<0)
	ans = 1 / 64.0
	Check(t, ans, glyphs.I26_6ToF32(x))

	x = fixed.Int26_6(12<<6 + 0<<0)
	ans = 12
	Check(t, ans, glyphs.I26_6ToF32(x))

	x = fixed.Int26_6(-3<<6 + 1<<2)
	ans = -(3.0 + 4/64.0)
	Check(t, ans, glyphs.I26_6ToF32(x))

	//Test min/max values
	x = fixed.I(33554431)
	ans = 33554431
	Check(t, ans, glyphs.I26_6ToF32(x))

	x = fixed.I(-33554432)
	ans = -33554432
	Check(t, ans, glyphs.I26_6ToF32(x))
}

func Check[T comparable](t *testing.T, expected, got T) {
	if got != expected {
		t.Fatalf("Expected %v but got %v\n", expected, got)
	}
}
