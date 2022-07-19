package ring

import (
	"golang.org/x/exp/constraints"
)

type Buffer[T any] struct {
	Data  []T
	Start int64
	Len   int64
	Cap   int64
}

func (b *Buffer[T]) Write(x ...T) {

	inLen := int64(len(x))

	for len(x) > 0 {

		copied := copy(b.Data[b.WriteHead():], x)
		x = x[copied:]

		if b.Len == b.Cap {
			b.Start = (b.Start + int64(copied)) % (b.Cap)
		} else {
			b.Len = clamp(b.Len+inLen, 0, b.Cap)
		}
	}
}

//WriteHead is the absolute position within the buffer where new writes will happen
func (b *Buffer[T]) WriteHead() int64 {
	return (b.Start + b.Len) % b.Cap
}

//Clear resets Len and Start to zero but elements within Data aren't touched.
//This gives you empty Views and new writes/inserts will overwrite old data
func (b *Buffer[T]) Clear() {
	b.Len = 0
	b.Start = 0
}

func (b *Buffer[T]) IsFull() bool {
	return b.Len == b.Cap
}

//Insert inserts the given elements starting at the provided index.
//
//Note: Insert is a no-op if the buffer is full or doesn't have enough place for the elements
func (b *Buffer[T]) Insert(index uint64, x ...T) {

	delta := int64(len(x))
	newLen := b.Len + delta
	if newLen > b.Cap {
		return
	}

	copy(b.Data[b.Start+int64(index)+delta:], b.Data[index:])
	copy(b.Data[index:], x)
	b.Len = newLen
}

//DeleteN removes 'n' elements starting at the provided index.
//
//Note DeleteN is a no-op if Len==0 or if buffer is full with start>0
func (b *Buffer[T]) DeleteN(delStartIndex, n uint64) {

	if b.Len == 0 {
		return
	}

	if b.Len == b.Cap && b.Start > 0 {
		return
	}

	relStartIndex := b.Start + int64(delStartIndex)
	copy(b.Data[relStartIndex:], b.Data[relStartIndex+int64(n):])
	b.Len = clamp(b.Len-int64(n), 0, b.Cap)
}

func clamp[T constraints.Ordered](x, min, max T) T {

	if x < min {
		return min
	}

	if x > max {
		return max
	}

	return x
}

// Views returns two slices that have 'Len' elements in total between them.
// The first slice is from Start till min(Start+Len, Cap). If Start+Len<=Cap then the first slice contains all the data and the second is empty.
// If Start+Len>Cap then the first slice contains the data from Start till Cap, and the second slice contains data from Zero till Start+Len-Cap (basically the remaining elements to reach Len in total)
//
// This function does NOT copy. Any changes on the returned slices will reflect on the buffer Data
//
// Note: Views become invalid when a write/insert is done on the buffer
func (b *Buffer[T]) Views() (v1, v2 []T) {

	if b.Start+b.Len <= b.Cap {
		return b.Data[b.Start : b.Start+b.Len], []T{}
	}

	v1 = b.Data[b.Start:]
	v2 = b.Data[:b.Start+b.Len-b.Cap]
	return
}

func (b *Buffer[T]) Iterator() Iterator[T] {

	v1, v2 := b.Views()
	return Iterator[T]{
		V1:   v1,
		V2:   v2,
		Curr: 0,
		InV1: true,
	}
}

func NewBuffer[T any](capacity uint64) *Buffer[T] {

	return &Buffer[T]{
		Data:  make([]T, capacity),
		Start: 0,
		Len:   0,
		Cap:   int64(capacity),
	}
}

// Iterator provides a way of iterating and indexing values of a ring buffer as if it was a flat array
// without having to deal with wrapping and so on.
//
// Indices used are all relative to 'Buffer.Start'
type Iterator[T any] struct {
	V1 []T
	V2 []T

	// Curr is the index of the element that will be returned on Next()
	Curr int64
	InV1 bool
}

// Next returns the value at Iterator.Curr and done=false
//
// If there are no more values to return the default value is returned for v and done=true
func (it *Iterator[T]) Next() (v T, done bool) {

	if it.InV1 {

		v = it.V1[it.Curr]

		it.Curr++
		if it.Curr >= int64(len(it.V1)) {
			it.Curr = 0
			it.InV1 = false
		}

		return v, false
	}

	if it.Curr >= int64(len(it.V2)) {
		return v, true
	}

	v = it.V2[it.Curr]
	it.Curr++
	return v, false
}

// Next returns the value at Iterator.Curr-1 and done=false
//
// If there are no more values to return the default value is returned for v and done=true
func (it *Iterator[T]) Prev() (v T, done bool) {

	if it.InV1 {

		if it.Curr <= 0 {
			return v, true
		}

		it.Curr--
		v = it.V1[it.Curr]
		return v, false
	}

	it.Curr--
	if it.Curr < 0 {
		it.InV1 = true
		it.Curr = int64(len(it.V1))
		return it.Prev()
	}

	v = it.V2[it.Curr]

	return v, false
}

// GotoStart adjusts the iterator such that the following Next() call returns the value at index=0
// and the next Prev() call returns done=true
func (it *Iterator[T]) GotoStart() {
	it.Curr = 0
	it.InV1 = true
}

// GotoIndex goes to the index n relative to Buffer.Start
func (it *Iterator[T]) GotoIndex(n int64) {

	if n <= 0 {
		it.GotoStart()
		return
	}

	v1Len := int64(len(it.V1))
	if n < v1Len {
		it.Curr = n
		it.InV1 = true
		return
	}

	n -= v1Len
	if n < int64(len(it.V2)) {
		it.Curr = n
		it.InV1 = false
		return
	}

	it.GotoEnd()
}

// GotoEnd adjusts the iterator such that the following Prev() call returns the value at index=Len-1
// and the following Next() call returns done=true
func (it *Iterator[T]) GotoEnd() {
	it.Curr = int64(len(it.V2))
	it.InV1 = false
}
