package ring_test

import (
	"testing"

	"github.com/bloeys/nterm/ring"
)

func TestRing(t *testing.T) {

	// Basics
	b := ring.NewBuffer[rune](4)
	b.Write('a', 'b', 'c', 'd')
	CheckArr(t, []rune{'a', 'b', 'c', 'd'}, b.Data)

	v1, v2 := b.Views()
	CheckArr(t, []rune{'a', 'b', 'c', 'd'}, v1)
	CheckArr(t, nil, v2)
	Check(t, 0, b.Start)
	Check(t, 4, b.Len)

	b.Write('e', 'f')
	Check(t, 2, b.Start)
	CheckArr(t, []rune{'e', 'f', 'c', 'd'}, b.Data)

	v1, v2 = b.Views()
	CheckArr(t, []rune{'c', 'd'}, v1)
	CheckArr(t, []rune{'e', 'f'}, v2)

	b.Write('g')
	Check(t, 3, b.Start)

	v1, v2 = b.Views()
	CheckArr(t, []rune{'e', 'f', 'g', 'd'}, b.Data)
	CheckArr(t, []rune{'d'}, v1)
	CheckArr(t, []rune{'e', 'f', 'g'}, v2)

	b = ring.NewBuffer[rune](4)
	b.Write('a', 'b', 'c', 'd', 'e')
	Check(t, 1, b.Start)

	// Input over 2x bigger than buffer
	b = ring.NewBuffer[rune](4)
	b.Write('a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i')
	CheckArr(t, []rune{'i', 'f', 'g', 'h'}, b.Data)

	// Input starting in the middle and having to loop back
	b2 := ring.NewBuffer[int](4)
	b2.Write(1, 2, 3)

	b2.Write(4, 5)
	CheckArr(t, []int{5, 2, 3, 4}, b2.Data)

	b2.Write(6)
	CheckArr(t, []int{5, 6, 3, 4}, b2.Data)

	b2.Write(7)
	CheckArr(t, []int{5, 6, 7, 4}, b2.Data)

	b2.Write(8)
	CheckArr(t, []int{5, 6, 7, 8}, b2.Data)

	// Insert
	b2 = ring.NewBuffer[int](4)
	b2.Write(1, 2)

	b2.Insert(0, 3)
	CheckArr(t, []int{3, 1, 2, 0}, b2.Data)

	b2.Insert(3, 4)
	CheckArr(t, []int{3, 1, 2, 4}, b2.Data)

	b2.Insert(2, 5, 6)
	CheckArr(t, []int{3, 1, 2, 4}, b2.Data)

	// Delete
	b2 = ring.NewBuffer[int](4)
	b2.Write(1, 2, 3, 4)

	b2.DeleteN(0, 4)
	Check(t, 0, b2.Start)
	Check(t, 0, b2.Len)
	CheckArr(t, []int{1, 2, 3, 4}, b2.Data)

	b2.Write(5, 6, 7, 8)
	Check(t, 4, b2.Len)
	b2.DeleteN(2, 1)
	Check(t, 3, b2.Len)
	CheckArr(t, []int{5, 6, 8, 8}, b2.Data)
}

func Check[T comparable](t *testing.T, expected, got T) {
	if got != expected {
		t.Fatalf("Expected %v but got %v\n", expected, got)
	}
}

func CheckArr[T comparable](t *testing.T, expected, got []T) {

	if len(expected) != len(got) {
		t.Fatalf("Expected %v but got %v\n", expected, got)
		return
	}

	for i := 0; i < len(expected); i++ {

		if expected[i] != got[i] {
			t.Fatalf("Expected %v but got %v\n", expected, got)
			return
		}
	}
}
