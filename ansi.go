package main

import (
	"bytes"
	"fmt"

	"github.com/bloeys/gglm/gglm"
)

// https://en.wikipedia.org/wiki/ANSI_escape_code#CSI_(Control_Sequence_Introducer)_sequences
// For Control Sequence Introducer (CSI) commands, `ESC[` is followed by:
// Zero or more "parameter bytes" in the range 0x30–0x3F.
// Zero or more "intermediate bytes" in the range 0x20–0x2F.
// One "final byte" in the range 0x40–0x7E.
const (
	AnsiCsiParamBytesStart  = 0x30
	AnsiCsiParamBytesEnd    = 0x3F
	AnsiCsiIntermBytesStart = 0x20
	AnsiCsiIntermBytesEnd   = 0x2F
	AnsiCsiFinalBytesStart  = 0x40
	AnsiCsiFinalBytesEnd    = 0x7E
)

var (
	AnsiEscBytes    = []byte{'\\', 'x', '1', 'b', '['}
	AnsiEscBytesLen = len(AnsiEscBytes)
)

func NextAnsiCode(arr []byte) (index int, code []byte) {

	// https://en.wikipedia.org/wiki/ANSI_escape_code#CSI_(Control_Sequence_Introducer)_sequences
	// For Control Sequence Introducer (CSI) commands, `ESC[` is followed by:
	// Zero or more "parameter bytes" in the range 0x30–0x3F.
	// Zero or more "intermediate bytes" in the range 0x20–0x2F.
	// One "final byte" in the range 0x40–0x7E.

	const paramBytesRegion = 0
	const intermBytesRegion = 1

	startOffset := 0
	for startOffset < len(arr)-1 {

		ansiEscIndex := bytes.Index(arr[startOffset:], AnsiEscBytes)
		if ansiEscIndex == -1 {
			return -1, nil
		}
		ansiEscIndex += startOffset
		startOffset = ansiEscIndex + AnsiEscBytesLen

		// Now that we have found an ESC[, to parse the sequence we expect bytes in a specific order
		// and a specific range. That is, a valid "paramter bytes" character in the interm bytes region
		// is considered an invalid char and will invalidate the sequence
		finalByteIndex := -1
		region := paramBytesRegion
		for i := ansiEscIndex + AnsiEscBytesLen; i < len(arr); i++ {

			b := arr[i]

			if region == paramBytesRegion {

				if b >= AnsiCsiParamBytesStart && b <= AnsiCsiParamBytesEnd {
					continue
				}

				if b >= AnsiCsiIntermBytesStart && b <= AnsiCsiIntermBytesEnd {
					region = intermBytesRegion
					continue
				}

				if b >= AnsiCsiFinalBytesStart && b <= AnsiCsiFinalBytesEnd {
					finalByteIndex = i
					break
				}

				break

			} else {

				if b >= AnsiCsiIntermBytesStart && b <= AnsiCsiIntermBytesEnd {
					continue
				}

				if b >= AnsiCsiFinalBytesStart && b <= AnsiCsiFinalBytesEnd {
					finalByteIndex = i
					break
				}

				break

			}
		}

		//If we fail to parse this sequence we continue to search ahead in the string
		if finalByteIndex == -1 {
			continue
		}

		return ansiEscIndex, arr[ansiEscIndex : finalByteIndex+1]
	}

	return -1, nil
}

func fgColorFromAnsiCode(code int) gglm.Vec4 {

	switch code {
	case Ansi_Fg_Black:
		return gglm.Vec4{}
	case Ansi_Fg_Red:
		return gglm.Vec4{Data: [4]float32{0.5, 0, 0, 1}}
	case Ansi_Fg_Green:
		return gglm.Vec4{Data: [4]float32{0, 0.5, 0, 1}}
	case Ansi_Fg_Yellow:
		return gglm.Vec4{Data: [4]float32{0.5, 0.5, 0, 1}}
	case Ansi_Fg_Blue:
		return gglm.Vec4{Data: [4]float32{0, 0, 0.5, 1}}
	case Ansi_Fg_Magenta:
		return gglm.Vec4{Data: [4]float32{0.5, 0, 0.5, 1}}
	case Ansi_Fg_Cyan:
		return gglm.Vec4{Data: [4]float32{0, 0.66, 0.66, 1}}
	case Ansi_Fg_White:
		return gglm.Vec4{Data: [4]float32{0.8, 0.8, 0.8, 1}}
	case Ansi_Fg_Gray:
		return gglm.Vec4{Data: [4]float32{0.5, 0.5, 0.5, 1}}

	case Ansi_Fg_Bright_Red:
		return gglm.Vec4{Data: [4]float32{1, 0, 0, 1}}
	case Ansi_Fg_Bright_Green:
		return gglm.Vec4{Data: [4]float32{0, 1, 0, 1}}
	case Ansi_Fg_Bright_Yellow:
		return gglm.Vec4{Data: [4]float32{1, 1, 0, 1}}
	case Ansi_Fg_Bright_Blue:
		return gglm.Vec4{Data: [4]float32{0, 0, 1, 1}}
	case Ansi_Fg_Bright_Magenta:
		return gglm.Vec4{Data: [4]float32{1, 0, 1, 1}}
	case Ansi_Fg_Bright_Cyan:
		return gglm.Vec4{Data: [4]float32{0, 1, 1, 1}}
	case Ansi_Fg_Bright_White:
		return gglm.Vec4{Data: [4]float32{1, 1, 1, 1}}

	}

	panic("Invalid ansi code: " + fmt.Sprint(code))
}
