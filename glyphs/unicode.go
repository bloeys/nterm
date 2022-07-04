package glyphs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
)

type Category uint8

const (
	//Normative categories
	Category_Lu Category = iota // Letter, Uppercase
	Category_Ll                 // Letter, Lowercase
	Category_Lt                 // Letter, Titlecase
	Category_Mn                 // Mark, Non-Spacing
	Category_Mc                 // Mark, Spacing Combining
	Category_Me                 // Mark, Enclosing
	Category_Nd                 // Number, Decimal Digit
	Category_Nl                 // Number, Letter
	Category_No                 // Number, Other
	Category_Zs                 // Separator, Space
	Category_Zl                 // Separator, Line
	Category_Zp                 // Separator, Paragraph
	Category_Cc                 // Other, Control
	Category_Cf                 // Other, Format
	Category_Cs                 // Other, Surrogate
	Category_Co                 // Other, Private Use
	Category_Cn                 // Other, Not Assigned (no characters in the file have this property)

	//Informative categories
	Category_Lm // Letter, Modifier
	Category_Lo // Letter, Other
	Category_Pc // Punctuation, Connector
	Category_Pd // Punctuation, Dash
	Category_Ps // Punctuation, Open
	Category_Pe // Punctuation, Close
	Category_Pi // Punctuation, Initial quote (may behave like Ps or Pe depending on usage)
	Category_Pf // Punctuation, Final quote (may behave like Ps or Pe depending on usage)
	Category_Po // Punctuation, Other
	Category_Sm // Symbol, Math
	Category_Sc // Symbol, Currency
	Category_Sk // Symbol, Modifier
	Category_So // Symbol, Other
)

type BidiCategory uint8

const (
	BidiCategory_L   BidiCategory = iota // Left-to-Right
	BidiCategory_LRE                     // Left-to-Right Embedding
	BidiCategory_LRO                     // Left-to-Right Override
	BidiCategory_LRI                     // Left‑to‑Right Isolate
	BidiCategory_R                       // Right-to-Left
	BidiCategory_AL                      // Right-to-Left Arabic
	BidiCategory_RLE                     // Right-to-Left Embedding
	BidiCategory_RLO                     // Right-to-Left Override
	BidiCategory_RLI                     // Right-to-Left Isolate
	BidiCategory_PDF                     // Pop Directional Format
	BidiCategory_PDI                     // Pop Directional Isolate
	BidiCategory_FSI                     // First Strong Isolate
	BidiCategory_EN                      // European Number
	BidiCategory_ES                      // European Number Separator
	BidiCategory_ET                      // European Number Terminator
	BidiCategory_AN                      // Arabic Number
	BidiCategory_CS                      // Common Number Separator
	BidiCategory_LRM                     // Left-to-Right Mark
	BidiCategory_RLM                     // Right-to-Left Mark
	BidiCategory_ALM                     // Arabic Letter Mark
	BidiCategory_NSM                     // Non-Spacing Mark
	BidiCategory_BN                      // Boundary Neutral
	BidiCategory_B                       // Paragraph Separator
	BidiCategory_S                       // Segment Separator
	BidiCategory_WS                      // Whitespace
	BidiCategory_ON                      // Other Neutrals
)

type CharDecompMapTag uint8

const (
	CharDecompMap_font     CharDecompMapTag = iota // A font variant (e.g. a blackletter form).
	CharDecompMap_noBreak                          // A no-break version of a space or hyphen.
	CharDecompMap_initial                          // An initial presentation form (Arabic).
	CharDecompMap_medial                           // A medial presentation form (Arabic).
	CharDecompMap_final                            // A final presentation form (Arabic).
	CharDecompMap_isolated                         // An isolated presentation form (Arabic).
	CharDecompMap_circle                           // An encircled form.
	CharDecompMap_super                            // A superscript form.
	CharDecompMap_sub                              // A subscript form.
	CharDecompMap_vertical                         // A vertical layout presentation form.
	CharDecompMap_wide                             // A wide (or zenkaku) compatibility character.
	CharDecompMap_narrow                           // A narrow (or hankaku) compatibility character.
	CharDecompMap_small                            // A small variant form (CNS compatibility).
	CharDecompMap_square                           // A CJK squared font variant.
	CharDecompMap_fraction                         // A vulgar fraction form.
	CharDecompMap_compat                           // Otherwise unspecified compatibility character.
	CharDecompMap_NONE                             // Not decomposition mapping tag, which indicates canonical form.
)

func (cd CharDecompMapTag) String() string {
	switch cd {

	case CharDecompMap_font:
		return "font"
	case CharDecompMap_noBreak:
		return "noBreak"
	case CharDecompMap_initial:
		return "initial"
	case CharDecompMap_medial:
		return "medial"
	case CharDecompMap_final:
		return "final"
	case CharDecompMap_isolated:
		return "isolated"
	case CharDecompMap_circle:
		return "circle"
	case CharDecompMap_super:
		return "super"
	case CharDecompMap_sub:
		return "sub"
	case CharDecompMap_vertical:
		return "vertical"
	case CharDecompMap_wide:
		return "wide"
	case CharDecompMap_narrow:
		return "narrow"
	case CharDecompMap_small:
		return "small"
	case CharDecompMap_square:
		return "square"
	case CharDecompMap_fraction:
		return "fraction"
	case CharDecompMap_compat:
		return "compat"
	case CharDecompMap_NONE:
		return "NONE"
	default:
		panic(fmt.Sprint("unknown CharDecompMapTag value:", uint8(cd)))
	}
}

type runeInfo struct {
	Name    string
	Cat     Category
	BidiCat BidiCategory

	DecompTag      CharDecompMapTag
	IsLigature     bool
	DecompMappings map[rune]struct{}
}

//loadUnicodeData decodes a 'UnicodeData' file according
//to http://www.unicode.org/Public/3.0-Update/UnicodeData-3.0.0.html
//
//The latest file is at https://www.unicode.org/Public/UCD/latest/ucd/UnicodeData.txt
func loadUnicodeData(unicodeFile string) (map[rune]runeInfo, error) {

	type field int
	const (
		field_codeValue         field = 0
		field_charName          field = 1
		field_generalCategory   field = 2
		field_combClasses       field = 3
		field_bidiCategory      field = 4
		field_charDecomp        field = 5
		field_decimalDigitValue field = 6
		field_digitValue        field = 7
		field_numericValue      field = 8
		field_mirrored          field = 9
		field_unicode1Name      field = 10
		field_comment           field = 11
		field_upperCaseMap      field = 12
		field_lowerCaseMap      field = 13
		field_titleCaseMap      field = 14
	)

	fBytes, err := os.ReadFile(unicodeFile)
	if err != nil {
		return nil, err
	}

	ris := make(map[rune]runeInfo)
	lines := strings.Split(string(fBytes), "\n")
	for _, l := range lines {

		fields := strings.SplitN(l, ";", 15)
		r := runeFromHexCodeString(fields[field_codeValue])
		if !unicode.Is(unicode.Arabic, r) {
			continue
		}

		ri := ris[r]
		ri = runeInfo{
			Name:      fields[field_charName],
			Cat:       categoryStringToCategory(fields[field_generalCategory]),
			BidiCat:   bidiCategoryStringToBidiCategory(fields[field_bidiCategory]),
			DecompTag: CharDecompMap_NONE,
		}

		//This field might already be set by another char mapping to us
		if ri.DecompMappings == nil {
			ri.DecompMappings = make(map[rune]struct{})
		}

		if len(fields[field_charDecomp]) > 0 {

			fieldItems := strings.Split(fields[field_charDecomp], " ")
			if fieldItems[0][0] == '<' {
				ri.DecompTag = charDecompMapStringToCharDecompMap(fieldItems[0])
			}

			//We consider a unicode codepoint that decomposes to more than one char a ligature
			ri.IsLigature = len(fieldItems) >= 3
			for i := 1; i < len(fieldItems); i++ {

				mappedRune := runeFromHexCodeString(fieldItems[i])
				ri.DecompMappings[mappedRune] = struct{}{}

				//Add this rune as a map of the other rune as well
				otherRi, ok := ris[mappedRune]
				if !ok {
					otherRi.DecompMappings = make(map[rune]struct{})
				}
				otherRi.DecompMappings[r] = struct{}{}
				ris[mappedRune] = otherRi
			}
		}

		ris[r] = ri
	}

	// meemRi := ris['م']
	// for mappedRune := range meemRi.DecompMappings {

	// 	mappedRuneInfo := ris[mappedRune]
	// 	if mappedRuneInfo.IsLigature {
	// 		continue
	// 	}
	// 	fmt.Printf("Meem mapping: %c. code=%x. Type=%s\n", mappedRune, mappedRune, mappedRuneInfo.DecompTag.String())
	// }

	return ris, nil
}

func runeFromHexCodeString(c string) rune {

	codepointU64, err := strconv.ParseUint(c, 16, 32)
	if err != nil {
		return invalidRune
	}

	return rune(codepointU64)
}

func categoryStringToCategory(c string) Category {

	switch c {
	case "Lu":
		return Category_Lu
	case "Ll":
		return Category_Ll
	case "Lt":
		return Category_Lt
	case "Mn":
		return Category_Mn
	case "Mc":
		return Category_Mc
	case "Me":
		return Category_Me
	case "Nd":
		return Category_Nd
	case "Nl":
		return Category_Nl
	case "No":
		return Category_No
	case "Zs":
		return Category_Zs
	case "Zl":
		return Category_Zl
	case "Zp":
		return Category_Zp
	case "Cc":
		return Category_Cc
	case "Cf":
		return Category_Cf
	case "Cs":
		return Category_Cs
	case "Co":
		return Category_Co
	case "Cn":
		return Category_Cn
	case "Lm":
		return Category_Lm
	case "Lo":
		return Category_Lo
	case "Pc":
		return Category_Pc
	case "Pd":
		return Category_Pd
	case "Ps":
		return Category_Ps
	case "Pe":
		return Category_Pe
	case "Pi":
		return Category_Pi
	case "Pf":
		return Category_Pf
	case "Po":
		return Category_Po
	case "Sm":
		return Category_Sm
	case "Sc":
		return Category_Sc
	case "Sk":
		return Category_Sk
	case "So":
		return Category_So
	default:
		panic("unknown Category string: " + c)
	}
}

func bidiCategoryStringToBidiCategory(c string) BidiCategory {

	switch c {
	case "L":
		return BidiCategory_L
	case "LRE":
		return BidiCategory_LRE
	case "LRO":
		return BidiCategory_LRO
	case "LRI":
		return BidiCategory_LRI
	case "R":
		return BidiCategory_R
	case "AL":
		return BidiCategory_AL
	case "RLE":
		return BidiCategory_RLE
	case "RLO":
		return BidiCategory_RLO
	case "RLI":
		return BidiCategory_RLI
	case "PDF":
		return BidiCategory_PDF
	case "PDI":
		return BidiCategory_PDI
	case "FSI":
		return BidiCategory_FSI
	case "EN":
		return BidiCategory_EN
	case "ES":
		return BidiCategory_ES
	case "ET":
		return BidiCategory_ET
	case "AN":
		return BidiCategory_AN
	case "CS":
		return BidiCategory_CS
	case "LRM":
		return BidiCategory_LRM
	case "RLM":
		return BidiCategory_RLM
	case "ALM":
		return BidiCategory_ALM
	case "NSM":
		return BidiCategory_NSM
	case "BN":
		return BidiCategory_BN
	case "B":
		return BidiCategory_B
	case "S":
		return BidiCategory_S
	case "WS":
		return BidiCategory_WS
	case "ON":
		return BidiCategory_ON
	default:
		panic("unknown bidiCategory string: " + c)
	}
}

func charDecompMapStringToCharDecompMap(c string) CharDecompMapTag {

	switch c {
	case "<font>":
		return CharDecompMap_font
	case "<noBreak>":
		return CharDecompMap_noBreak
	case "<initial>":
		return CharDecompMap_initial
	case "<medial>":
		return CharDecompMap_medial
	case "<final>":
		return CharDecompMap_final
	case "<isolated>":
		return CharDecompMap_isolated
	case "<circle>":
		return CharDecompMap_circle
	case "<super>":
		return CharDecompMap_super
	case "<sub>":
		return CharDecompMap_sub
	case "<vertical>":
		return CharDecompMap_vertical
	case "<wide>":
		return CharDecompMap_wide
	case "<narrow>":
		return CharDecompMap_narrow
	case "<small>":
		return CharDecompMap_small
	case "<square>":
		return CharDecompMap_square
	case "<fraction>":
		return CharDecompMap_fraction
	case "<compat>":
		return CharDecompMap_compat
	case "":
		return CharDecompMap_NONE
	default:
		panic("unknown charDecomMap string: " + c)
	}
}
