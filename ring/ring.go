package ring

import "golang.org/x/exp/constraints"

type Buffer[T any] struct {
	Data  []T
	Start int64
	Len   int64
	Cap   int64
}

func (b *Buffer[T]) Append(x ...T) {

	inLen := int64(len(x))

	for len(x) > 0 {

		copied := copy(b.Data[b.Head():], x)
		x = x[copied:]

		if b.Len == b.Cap {
			b.Start = (b.Start + int64(copied)) % (b.Cap)
		} else {
			b.Len = clamp(b.Len+inLen, 0, b.Cap)
		}
	}
}

func (b *Buffer[T]) Head() int64 {
	return (b.Start + b.Len) % b.Cap
}

func (b *Buffer[T]) Insert(index uint64, x ...T) {

	delta := int64(len(x))
	newLen := b.Len + delta
	if newLen <= b.Cap {

		copy(b.Data[b.Start+int64(index)+delta:], b.Data[index:])
		copy(b.Data[index:], x)

		b.Len = newLen
		return
	}
}

func (b *Buffer[T]) Delete(delStartIndex, elementsToDel uint64, x ...T) {

	if b.Len == 0 {
		return
	}

	copy(b.Data[uint64(b.Start)+delStartIndex:], b.Data[uint64(b.Start)+delStartIndex+elementsToDel:])

	// p.cmdBufLen--
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
// The first slice is from Start till min(Start+Len, Cap). If Start+Len<=Cap then the first slice contains all the data and the second is nil.
// If Start+Len>Cap then the first slice contains the data from Start till Cap, and the second slice contains data from Zero till Start+Len-Cap (basically the remaining elements to reach Len in total)
//
// This function does NOT copy. Any changes on the returned slices will reflect on the buffer Data
func (b *Buffer[T]) Views() (v1, v2 []T) {

	if b.Start+b.Len <= b.Cap {
		return b.Data[b.Start : b.Start+b.Len], nil
	}

	v1 = b.Data[b.Start:b.Cap]
	v2 = b.Data[:b.Start+b.Len-b.Cap]
	return
}

func NewBuffer[T any](capacity uint64) *Buffer[T] {

	return &Buffer[T]{
		Data:  make([]T, capacity),
		Start: 0,
		Len:   0,
		Cap:   int64(capacity),
	}
}
