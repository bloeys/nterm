package ring

import (
	"github.com/bloeys/nterm/assert"
	"golang.org/x/exp/constraints"
)

type Buffer[T any] struct {
	Data  []T
	Start int64
	Len   int64
	Cap   int64

	// WrittenElements is the total number of elements written to the buffer over its lifetime.
	// Can be bigger than Cap
	WrittenElements uint64
}

func (b *Buffer[T]) Write(x ...T) {

	inLen := int64(len(x))
	b.WrittenElements += uint64(inLen)

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

func clamp[T constraints.Ordered](x, min, max T) T {

	if x < min {
		return min
	}

	if x > max {
		return max
	}

	return x
}

// Get returns the element at the index relative from Buffer.Start
// If there are no elements then the default value of T is returned
func (b *Buffer[T]) Get(index uint64) (val T) {

	if index >= uint64(b.Len) {
		return val
	}

	return b.Data[(b.Start+int64(index))%b.Cap]
}

// Get returns the element at the index relative from Buffer.Start
// If there are no elements then the default value of T is returned
func (b *Buffer[T]) GetPtr(index uint64) (val *T) {

	if index >= uint64(b.Len) {
		return new(T)
	}

	return &b.Data[(b.Start+int64(index))%b.Cap]
}

// AbsIndexFromRel takes an index relative to Buffer.Start and returns an absolute index into Buffer.Data
func (b *Buffer[T]) AbsIndexFromRel(relIndex uint64) uint64 {
	return uint64((b.Start + int64(relIndex)) % b.Cap)
}

// RelIndexFromAbs takes an index into Buffer.Data and returns an index relative to Buffer.Start
func (b *Buffer[T]) RelIndexFromAbs(absIndex uint64) uint64 {
	assert.T(absIndex < uint64(b.Cap), "absIndex must be between 0 and Buffer.Cap-1")
	return uint64((int64(absIndex) - b.Start + b.Cap) % b.Cap)
}

// AbsIndexFromWriteCount takes the total number of elements written and returns the index of the
// last written element after 'writeCount' writes.
//
// For example, if writeCount=1 then the index of last written element (the returned value) is zero.
// For a buffer of cap=4, after 5 writes the last updated index is absIndex=0
//
// writeCount=0 is undefined because no elements have been written to yet. In this case zero is returned.
func (b *Buffer[T]) AbsIndexFromWriteCount(writeCount uint64) uint64 {
	if writeCount == 0 {
		return 0
	}

	return (writeCount - 1) % uint64(b.Cap)
}

// RelIndexFromWriteCount takes the total number of elements written and returns the index of the
// last written element after 'writeCount' writes relative to the current Buffer.Start value.
//
// For example, if writeCount=1 then the index of last written element (the returned value) is zero.
// For a buffer of cap=4, after 5 writes the last updated index is absIndex=0
//
// writeCount=0 is undefined because no elements have been written to yet. In this case zero is returned.
func (b *Buffer[T]) RelIndexFromWriteCount(writeCount uint64) uint64 {
	if writeCount == 0 {
		return 0
	}
	return b.RelIndexFromAbs(b.AbsIndexFromWriteCount(writeCount))
}

// Views returns two slices that have 'Len' elements in total between them.
// The first slice is from Start till min(Start+Len, Cap). If Start+Len<=Cap then the first slice contains all the data and the second is empty.
// If Start+Len>Cap then the first slice contains the data from Start till Cap, and the second slice contains data from 0 till Start+Len-Cap (basically the remaining elements to reach Len in total)
//
// This function does NOT copy. Any changes on the returned slices will reflect on the buffer Data
//
// Note: Views become invalid when a write/insert is done on the buffer
func (b *Buffer[T]) Views() (v1, v2 []T) {

	if b.Start+b.Len <= b.Cap {
		return b.Data[b.Start : b.Start+b.Len], []T{}
	}

	v1 = b.Data[b.Start:]
	v2 = b.Data[:(b.Start+b.Len)%b.Cap]
	return
}

func (b *Buffer[T]) ViewsFromToWriteCount(fromIndex, toIndex uint64) (v1, v2 []T) {
	fromIndex = b.RelIndexFromWriteCount(fromIndex)
	toIndex = b.RelIndexFromWriteCount(toIndex)
	return b.ViewsFromToRelIndex(fromIndex, toIndex)
}

// ViewsFromToRelIndex takes indices relative to Buffer.Start and returns views adjusted to contain
// elements between these two indices (inclusive)
func (b *Buffer[T]) ViewsFromToRelIndex(fromIndex, toIndex uint64) (v1, v2 []T) {

	toIndex++ // We convert the index into a length (e.g. from=0, to=0 is from=0, len=1)
	if toIndex <= fromIndex || fromIndex >= uint64(b.Len) {
		return []T{}, []T{}
	}

	v1, v2 = b.Views()
	v1Len := uint64(len(v1))
	v2Len := uint64(len(v2))
	startInV1 := fromIndex < v1Len

	if startInV1 {

		if toIndex <= v1Len {
			v1 = v1[fromIndex:toIndex]
			v2 = v2[:0]
			return
		}

		toIndex -= v1Len
		if toIndex > v2Len {
			toIndex = v2Len
		}

		v1 = v1[fromIndex:v1Len]
		v2 = v2[:toIndex]
		return
	}

	fromIndex -= v1Len
	toIndex -= v1Len
	if toIndex >= v2Len {
		toIndex = v2Len
	}

	v1 = v1[:0]
	v2 = v2[fromIndex:toIndex]
	return
}

func (b *Buffer[T]) Iterator() Iterator[T] {
	return NewIterator(b)
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
	Buf *Buffer[T]
	V1  []T
	V2  []T

	// Curr is the index of the element that will be returned on Next(),
	// which means it is an index into V1 or V2 and so is relative to Buffer.Start value at the time
	// of creating this iterator instance
	Curr int64
	InV1 bool
}

func (it *Iterator[T]) Len() int64 {
	return int64(len(it.V1) + len(it.V2))
}

func (it *Iterator[T]) CurrToRelIndex() uint64 {

	if it.InV1 {
		return uint64(it.Curr)
	}

	return uint64(it.Curr) + uint64(len(it.V1))
}

func (it *Iterator[T]) NextPtr() (v *T, done bool) {

	if it.InV1 {

		v = &it.V1[it.Curr]

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

	v = &it.V2[it.Curr]
	it.Curr++
	return v, false
}

// Next returns the value at Iterator.Curr and done=false
//
// If there are no more values to return the default value is returned for v and done=true
func (it *Iterator[T]) Next() (v T, done bool) {

	var vPtr *T
	vPtr, done = it.NextPtr()
	if vPtr == nil {
		return v, done
	}

	return *vPtr, done
}

// Next returns the value at Iterator.Curr-1 and done=false
//
// If there are no more values to return the default value is returned for v and done=true
func (it *Iterator[T]) PrevPtr() (v *T, done bool) {

	if it.InV1 {

		if it.Curr <= 0 {
			return v, true
		}

		it.Curr--
		v = &it.V1[it.Curr]
		return v, false
	}

	it.Curr--
	if it.Curr < 0 {
		it.InV1 = true
		it.Curr = int64(len(it.V1))
		return it.PrevPtr()
	}

	v = &it.V2[it.Curr]

	return v, false
}

func (it *Iterator[T]) Prev() (v T, done bool) {

	var vPtr *T
	vPtr, done = it.PrevPtr()
	if vPtr == nil {
		return v, done
	}

	return *vPtr, done
}

// NextN calls Next() up to n times and places the result in the passed buffer.
// 'read' is the actual number of elements put in the buffer.
// We might not be able to put 'n' elements because the buffer is too small or because there aren't enough remaining elements
func (it *Iterator[T]) NextN(buf []T, n int) (read int, done bool) {

	if n > len(buf) {
		n = len(buf)
	}

	var v T
	for v, done = it.Next(); !done; v, done = it.Next() {

		buf[read] = v
		read++

		// We must break inside the loop not in the 'for' check because
		// if we check part of the loop and break before done=true we will waste the last
		// value
		n--
		if n == 0 {
			break
		}
	}

	return read, done
}

// PrevN calls Prev() up to n times and places the result in the passed buffer in the order they are read from Prev().
// That is, the first Prev() call is put into index 0, second Prev() call is in index 1, and so on,
// similar to if you were doing a reverse loop on an array.
//
// 'read' is the actual number of elements put in the buffer.
// We might not be able to put 'n' elements because the buffer is too small or because there aren't enough remaining elements
func (it *Iterator[T]) PrevN(buf []T, n int) (read int, done bool) {

	if n > len(buf) {
		n = len(buf)
	}

	var v T
	for v, done = it.Prev(); !done; v, done = it.Prev() {

		buf[read] = v
		read++

		// We must break inside the loop not in the 'for' check because
		// if we check part of the loop and break before done=true we will waste the last
		// value
		n--
		if n == 0 {
			break
		}
	}

	return read, done
}

func (it *Iterator[T]) HasNext() bool {
	hasNext := it.InV1 || it.Curr < int64(len(it.V2))
	return hasNext
}

func (it *Iterator[T]) HasPrev() bool {
	hasPrev := (!it.InV1 && it.Curr-1 >= 0) || (it.InV1 && it.Curr > 0)
	return hasPrev
}

// GotoStart adjusts the iterator such that the following Next() call returns the value at index=0
// and the next Prev() call returns done=true
func (it *Iterator[T]) GotoStart() {
	it.Curr = 0
	it.InV1 = len(it.V1) > 0
}

// GotoIndex goes to the index n relative to Buffer.Start.
// If n<=0 this is equivalent to Iterator.GotoStart()
// if n>=Buffer.Len this is equivalent to Iterator.GotoEnd()
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

func NewIterator[T any](b *Buffer[T]) Iterator[T] {
	v1, v2 := b.Views()
	return Iterator[T]{
		Buf:  b,
		V1:   v1,
		V2:   v2,
		Curr: 0,
		InV1: len(v1) > 0, // If buffer is empty we shouldn't be in V1
	}
}
