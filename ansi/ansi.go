package ansi

import (
	"bytes"
	"fmt"

	"github.com/bloeys/gglm/gglm"
)

type CSIType int

const (
	CSIType_Unknown CSIType = iota

	// Moves the cursor n (default 1) cells in the given direction. This has no effect if the cursor is at the edge of the screen
	CSIType_CUU // Cursor Up
	CSIType_CUD // Cursor Down
	CSIType_CUF // Cursor Forward
	CSIType_CUB // Cursor Back

	// Moves cursor to beginning of the line n (default 1) lines down/up
	CSIType_CNL // Cursor Next Line
	CSIType_CPL // Cursor Previous Line

	CSIType_CHA // Cursor Horizontal Absolute. Moves the cursor to column n (default 1)
	CSIType_CUP // Cursor Position. Moves the cursor to row n, column m. The values are 1-based, and default to 1 (top left corner) if omitted

	// Erase in Display. Clears part of the screen.
	// If n is 0 (or missing) clear from cursor to end of screen.
	// If n is 1, clear from cursor to beginning of the screen.
	// If n is 2, clear entire screen (and moves cursor to upper left on DOS ANSI.SYS).
	// If n is 3, clear entire screen and delete all lines saved in the scrollback buffer.
	CSIType_ED

	// Erase in Line. Erases part of the line.
	// If n is 0 (or missing), clear from cursor to the end of the line.
	// If n is 1, clear from cursor to beginning of the line.
	// If n is 2, clear entire line. Cursor position does not change.
	CSIType_EL

	// Scroll Up. Scroll whole page up by n (default 1) lines. New lines are added at the bottom
	CSIType_SU

	// Scroll Down. Scroll whole page down by n (default 1) lines. New lines are added at the top
	CSIType_SD

	// Horizontal Vertical Position. Same as CUP, but counts as a format effector function (like CR or LF) rather than an editor function (like CUD or CNL)
	CSIType_HVP

	// Select Graphic Rendition. Sets colors and style of the characters following this code
	CSIType_SGR

	// Device Status Report. Reports the cursor position (CPR) by transmitting ESC[n;mR, where n is the row and m is the column
	CSIType_DSR
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

const (
	Ansi_Fg_Black          = 30
	Ansi_Fg_Red            = 31
	Ansi_Fg_Green          = 32
	Ansi_Fg_Yellow         = 33
	Ansi_Fg_Blue           = 34
	Ansi_Fg_Magenta        = 35
	Ansi_Fg_Cyan           = 36
	Ansi_Fg_White          = 37
	Ansi_Fg_Gray           = 90
	Ansi_Fg_Bright_Red     = 91
	Ansi_Fg_Bright_Green   = 92
	Ansi_Fg_Bright_Yellow  = 93
	Ansi_Fg_Bright_Blue    = 94
	Ansi_Fg_Bright_Magenta = 95
	Ansi_Fg_Bright_Cyan    = 96
	Ansi_Fg_Bright_White   = 97

	Ansi_Bg_Black          = 40
	Ansi_Bg_Red            = 41
	Ansi_Bg_Green          = 42
	Ansi_Bg_Yellow         = 43
	Ansi_Bg_Blue           = 44
	Ansi_Bg_Magenta        = 45
	Ansi_Bg_Cyan           = 46
	Ansi_Bg_White          = 47
	Ansi_Bg_Gray           = 100
	Ansi_Bg_Bright_Red     = 101
	Ansi_Bg_Bright_Green   = 102
	Ansi_Bg_Bright_Yellow  = 103
	Ansi_Bg_Bright_Blue    = 104
	Ansi_Bg_Bright_Magenta = 105
	Ansi_Bg_Bright_Cyan    = 106
	Ansi_Bg_Bright_White   = 107
)

var (
	AnsiEscByte = byte('\x1b')
	// AnsiEscStringBytes = []byte{'\\', 'x', '1', 'b'} // represents the string: \x1b

	AnsiCSIBytes    = []byte{'\x1b', '['}
	AnsiCSIBytesLen = len(AnsiCSIBytes)
	// AnsiCSIStringBytes    = []byte{'\\', 'x', '1', 'b', '['} // represents the string: \x1b[
	// AnsiCSIStringBytesLen = len(AnsiCSIStringBytes)
)

type AnsiCodePayloadType int32

const (
	AnsiCodePayloadType_Unknown AnsiCodePayloadType = iota
	AnsiCodePayloadType_ColorFg
	AnsiCodePayloadType_ColorBg
	AnsiCodePayloadType_Reset

	AnsiCodePayloadType_CursorOffset
	AnsiCodePayloadType_CursorAbs
	AnsiCodePayloadType_LineOffset
	AnsiCodePayloadType_LineAbs
	AnsiCodePayloadType_ScrollOffset
)

func (a AnsiCodePayloadType) HasOption(opt AnsiCodePayloadType) bool {
	return a == opt
}

type AnsiCodeInfoPayload struct {
	Info gglm.Vec4
	Type AnsiCodePayloadType
}

type AnsiCodeInfo struct {
	Type    CSIType
	Payload []AnsiCodeInfoPayload
}

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

		ansiEscIndex := bytes.Index(arr[startOffset:], AnsiCSIBytes)
		if ansiEscIndex == -1 {
			return -1, nil
		}
		ansiEscIndex += startOffset
		startOffset = ansiEscIndex + AnsiCSIBytesLen

		// Now that we have found an ESC[, to parse the sequence we expect bytes in a specific order
		// and a specific range. That is, a valid "paramter bytes" character in the interm bytes region
		// is considered an invalid char and will invalidate the sequence
		finalByteIndex := -1
		region := paramBytesRegion
		for i := ansiEscIndex + AnsiCSIBytesLen; i < len(arr); i++ {

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

func InfoFromAnsiCode(code []byte) (info AnsiCodeInfo) {

	codeLen := len(code)
	if codeLen < AnsiCSIBytesLen+1 {
		return info
	}

	finalByte := code[codeLen-1]
	args := code[AnsiCSIBytesLen : codeLen-1]

	// @TODO finish parsing and filling struct
	switch finalByte {

	case 'm':
		info.Type = CSIType_SGR
		info.Payload = ParseSGRArgs(args)
	case 'A':
		info.Type = CSIType_CUU
	case 'B':
		info.Type = CSIType_CUD
	case 'C':
		info.Type = CSIType_CUF
	case 'D':
		info.Type = CSIType_CUB
	case 'E':
		info.Type = CSIType_CNL
	case 'F':
		info.Type = CSIType_CPL
	case 'G':
		info.Type = CSIType_CHA
	case 'H':
		info.Type = CSIType_CUP
	case 'J':
		info.Type = CSIType_ED
	case 'K':
		info.Type = CSIType_EL
	case 'S':
		info.Type = CSIType_SU
	case 'T':
		info.Type = CSIType_SD
	case 'f':
		info.Type = CSIType_HVP

		// case 'n':
		// 	if code[codeLen-2] == '6' {
		// 		info.Type = CSIType_DSR
		// 		args = code[AnsiCSIBytesLen : codeLen-2]
		// 	}
	}

	return info
}

func ParseSGRArgs(args []byte) (payload []AnsiCodeInfoPayload) {

	payload = make([]AnsiCodeInfoPayload, 0, 1)

	// @TODO should we trim spaces?
	splitArgs := bytes.Split(args, []byte{';'})
	for _, a := range splitArgs {

		if len(a) == 0 || a[0] == byte('0') {
			payload = append(payload, AnsiCodeInfoPayload{
				Type: AnsiCodePayloadType_Reset,
			})
			break
		}

		// @TODO We can't use this setup of one info field because one ansi code can have many settings.
		// For example, it can set Fg+Bg at once. So we need info per option.
		intCode := getSgrIntCodeFromBytes(a)
		if intCode >= 30 && intCode <= 37 || intCode >= 90 && intCode <= 97 {
			payload = append(payload, AnsiCodeInfoPayload{
				Info: ColorFromSgrCode(intCode),
				Type: AnsiCodePayloadType_ColorFg,
			})
			continue
		}

		if intCode >= 40 && intCode <= 47 || intCode >= 100 && intCode <= 107 {
			payload = append(payload, AnsiCodeInfoPayload{
				Info: ColorFromSgrCode(intCode),
				Type: AnsiCodePayloadType_ColorBg,
			})
			continue
		}

		// @TODO Support bold/underline etc
		// @TODO Support 256 and RGB colors
		println("Code not supported yet: " + fmt.Sprint(intCode))
	}

	return payload
}

func getSgrIntCodeFromBytes(bs []byte) (code int) {

	mul := 1
	for i := 1; i < len(bs); i++ {
		mul *= 10
	}

	for i := 0; i < len(bs); i++ {

		b := bs[i]
		switch b {
		case '1':
			code += 1 * mul
		case '2':
			code += 2 * mul
		case '3':
			code += 3 * mul
		case '4':
			code += 4 * mul
		case '5':
			code += 5 * mul
		case '6':
			code += 6 * mul
		case '7':
			code += 7 * mul
		case '8':
			code += 8 * mul
		case '9':
			code += 9 * mul
		}

		mul /= 10
	}

	return code
}

func ColorFromSgrCode(code int) gglm.Vec4 {

	switch code {

	//Foreground and background
	case Ansi_Bg_Black:
		fallthrough
	case Ansi_Fg_Black:
		return gglm.Vec4{Data: [4]float32{0, 0, 0, 1}}

	case Ansi_Bg_Red:
		fallthrough
	case Ansi_Fg_Red:
		return gglm.Vec4{Data: [4]float32{0.7, 0, 0, 1}}

	case Ansi_Bg_Green:
		fallthrough
	case Ansi_Fg_Green:
		return gglm.Vec4{Data: [4]float32{0, 0.7, 0, 1}}

	case Ansi_Bg_Yellow:
		fallthrough
	case Ansi_Fg_Yellow:
		return gglm.Vec4{Data: [4]float32{0.7, 0.7, 0, 1}}

	case Ansi_Bg_Blue:
		fallthrough
	case Ansi_Fg_Blue:
		return gglm.Vec4{Data: [4]float32{0, 0, 0.7, 1}}

	case Ansi_Bg_Magenta:
		fallthrough
	case Ansi_Fg_Magenta:
		return gglm.Vec4{Data: [4]float32{0.7, 0, 0.7, 1}}

	case Ansi_Bg_Cyan:
		fallthrough
	case Ansi_Fg_Cyan:
		return gglm.Vec4{Data: [4]float32{0, 0.66, 0.66, 1}}

	case Ansi_Bg_White:
		fallthrough
	case Ansi_Fg_White:
		return gglm.Vec4{Data: [4]float32{0.8, 0.8, 0.8, 1}}

	case Ansi_Bg_Gray:
		fallthrough
	case Ansi_Fg_Gray:
		return gglm.Vec4{Data: [4]float32{0.7, 0.7, 0.7, 1}}

	//Bright foreground and background
	case Ansi_Bg_Bright_Red:
		fallthrough
	case Ansi_Fg_Bright_Red:
		return gglm.Vec4{Data: [4]float32{1, 0, 0, 1}}

	case Ansi_Bg_Bright_Green:
		fallthrough
	case Ansi_Fg_Bright_Green:
		return gglm.Vec4{Data: [4]float32{0, 1, 0, 1}}

	case Ansi_Bg_Bright_Yellow:
		fallthrough
	case Ansi_Fg_Bright_Yellow:
		return gglm.Vec4{Data: [4]float32{1, 1, 0, 1}}

	case Ansi_Bg_Bright_Blue:
		fallthrough
	case Ansi_Fg_Bright_Blue:
		return gglm.Vec4{Data: [4]float32{0, 0, 1, 1}}

	case Ansi_Bg_Bright_Magenta:
		fallthrough
	case Ansi_Fg_Bright_Magenta:
		return gglm.Vec4{Data: [4]float32{1, 0, 1, 1}}

	case Ansi_Bg_Bright_Cyan:
		fallthrough
	case Ansi_Fg_Bright_Cyan:
		return gglm.Vec4{Data: [4]float32{0, 1, 1, 1}}

	case Ansi_Bg_Bright_White:
		fallthrough
	case Ansi_Fg_Bright_White:
		return gglm.Vec4{Data: [4]float32{1, 1, 1, 1}}

	}

	panic("Invalid ansi code: " + fmt.Sprint(code))
}
