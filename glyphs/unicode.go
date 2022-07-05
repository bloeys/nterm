package glyphs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
)

const UnicodeVersion = "13.0.0"

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

	BidiCategory_NSM // Non-Spacing Mark
	BidiCategory_BN  // Boundary Neutral
	BidiCategory_B   // Paragraph Separator
	BidiCategory_S   // Segment Separator
	BidiCategory_WS  // Whitespace
	BidiCategory_ON  // Other Neutrals
)

type DecompTag uint8

const (
	DecompTag_font     DecompTag = iota // A font variant (e.g. a blackletter form).
	DecompTag_noBreak                   // A no-break version of a space or hyphen.
	DecompTag_initial                   // An initial presentation form (Arabic).
	DecompTag_medial                    // A medial presentation form (Arabic).
	DecompTag_final                     // A final presentation form (Arabic).
	DecompTag_isolated                  // An isolated presentation form (Arabic).
	DecompTag_circle                    // An encircled form.
	DecompTag_super                     // A superscript form.
	DecompTag_sub                       // A subscript form.
	DecompTag_vertical                  // A vertical layout presentation form.
	DecompTag_wide                      // A wide (or zenkaku) compatibility character.
	DecompTag_narrow                    // A narrow (or hankaku) compatibility character.
	DecompTag_small                     // A small variant form (CNS compatibility).
	DecompTag_square                    // A CJK squared font variant.
	DecompTag_fraction                  // A vulgar fraction form.
	DecompTag_compat                    // Otherwise unspecified compatibility character.
	DecompTag_NONE                      // Not decomposition mapping tag, which indicates canonical form.
)

func (cd DecompTag) String() string {
	switch cd {

	case DecompTag_font:
		return "font"
	case DecompTag_noBreak:
		return "noBreak"
	case DecompTag_initial:
		return "initial"
	case DecompTag_medial:
		return "medial"
	case DecompTag_final:
		return "final"
	case DecompTag_isolated:
		return "isolated"
	case DecompTag_circle:
		return "circle"
	case DecompTag_super:
		return "super"
	case DecompTag_sub:
		return "sub"
	case DecompTag_vertical:
		return "vertical"
	case DecompTag_wide:
		return "wide"
	case DecompTag_narrow:
		return "narrow"
	case DecompTag_small:
		return "small"
	case DecompTag_square:
		return "square"
	case DecompTag_fraction:
		return "fraction"
	case DecompTag_compat:
		return "compat"
	case DecompTag_NONE:
		return "NONE"
	default:
		panic(fmt.Sprint("unknown CharDecompMapTag value:", uint8(cd)))
	}
}

type JoiningType uint8

const (
	JoiningType_Right JoiningType = iota
	JoiningType_Left
	JoiningType_Dual
	JoiningType_Causing
	JoiningType_None
	JoiningType_Transparent
)

type RuneInfo struct {
	Name       string
	Cat        Category
	BidiCat    BidiCategory
	DecompTag  DecompTag
	JoinType   JoiningType
	IsLigature bool

	//Decomp is the ordered set of runes this rune decomposes into
	//as defined by unicodeData.txt
	Decomp []rune

	//EquivalentRunes are runes that are canonically or compatiability equivalent to this rune
	EquivalentRunes []rune

	//ScriptTable is one of the script tables in the unicode package such as unicode.Arabic
	//and is guaranteed to not be nil.
	ScriptTable *unicode.RangeTable
}

// ParseUnicodeData decodes a 'UnicodeData' file according
// to http://www.unicode.org/Public/3.0-Update/UnicodeData-3.0.0.html and returns a map containing information
// on all runes within the passed ranges. If no ranges are passed then the full unicode data file will be decoded.
//
// Any runes that don't fall in a script range are ignored (usually only a handful).
//
//The latest file can be found at https://www.unicode.org/Public/UCD/latest/ucd/UnicodeData.txt
func ParseUnicodeData(unicodeDataFile, arabicShapingFile string, rangesToLoad ...*unicode.RangeTable) (map[rune]RuneInfo, error) {

	type field uint8
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

	asInfo, err := ParseArabicShaping(arabicShapingFile)
	if err != nil {
		return nil, err
	}

	fBytes, err := os.ReadFile(unicodeDataFile)
	if err != nil {
		return nil, err
	}

	ris := make(map[rune]RuneInfo)
	lines := strings.Split(string(fBytes), "\n")
	for _, l := range lines {

		fields := strings.SplitN(l, ";", 15)
		r := runeFromHexCodeString(fields[field_codeValue])
		if rangesToLoad != nil && !unicode.In(r, rangesToLoad...) {
			continue
		}

		scriptTable := ScriptTableFromRune(r)
		if scriptTable == nil {
			continue
		}

		ri := ris[r]
		ri = RuneInfo{
			Name:      fields[field_charName],
			Cat:       categoryStringToCategory(fields[field_generalCategory]),
			BidiCat:   bidiCategoryStringToBidiCategory(fields[field_bidiCategory]),
			DecompTag: DecompTag_NONE,

			//NOTE: This is not perfect (NamesList.txt notes some additional ligatures), but good enough :)
			IsLigature:  strings.Contains(fields[field_charName], "LIGATURE"),
			ScriptTable: scriptTable,
		}

		//Handle join type
		asi, ok := asInfo[r]
		if ok {
			ri.JoinType = asi.JoinType
		} else if ri.Cat == Category_Mn || ri.Cat == Category_Me || ri.Cat == Category_Cf {
			ri.JoinType = JoiningType_Transparent
		} else {
			ri.JoinType = JoiningType_None
		}

		//This might already be created for us by a previous ruen
		if ri.EquivalentRunes == nil {
			ri.EquivalentRunes = []rune{}
		}

		if len(fields[field_charDecomp]) > 0 {

			fieldItems := strings.Split(fields[field_charDecomp], " ")
			if fieldItems[0][0] == '<' {
				ri.DecompTag = charDecompMapStringToCharDecompMap(fieldItems[0])
				fieldItems = fieldItems[1:]
			}

			//One character decomposition indicates equivalence
			if len(fieldItems) == 1 {

				decompRune := runeFromHexCodeString(fieldItems[0])
				ri.Decomp = []rune{decompRune}
				ri.EquivalentRunes = append(ri.EquivalentRunes, decompRune)

				//Add this rune as equivalent to decomposed rune
				decompRuneInfo := ris[decompRune]
				if decompRuneInfo.EquivalentRunes == nil {
					decompRuneInfo.EquivalentRunes = []rune{r}
				} else {
					decompRuneInfo.EquivalentRunes = append(decompRuneInfo.EquivalentRunes, r)
				}

				ris[decompRune] = decompRuneInfo

			} else {

				ri.Decomp = make([]rune, len(fieldItems))
				for i := 0; i < len(fieldItems); i++ {
					ri.Decomp[i] = runeFromHexCodeString(fieldItems[i])
				}
			}
		}

		ris[r] = ri
	}

	return ris, nil
}

func runeFromHexCodeString(c string) rune {

	codepointU64, err := strconv.ParseUint(c, 16, 32)
	if err != nil {
		panic("Invalid rune: " + c)
	}

	return rune(codepointU64)
}

func ScriptTableFromRune(r rune) *unicode.RangeTable {

	if unicode.In(r, unicode.Adlam) {
		return unicode.Adlam
	} else if unicode.In(r, unicode.Ahom) {
		return unicode.Ahom
	} else if unicode.In(r, unicode.Anatolian_Hieroglyphs) {
		return unicode.Anatolian_Hieroglyphs
	} else if unicode.In(r, unicode.Arabic) {
		return unicode.Arabic
	} else if unicode.In(r, unicode.Armenian) {
		return unicode.Armenian
	} else if unicode.In(r, unicode.Avestan) {
		return unicode.Avestan
	} else if unicode.In(r, unicode.Balinese) {
		return unicode.Balinese
	} else if unicode.In(r, unicode.Bamum) {
		return unicode.Bamum
	} else if unicode.In(r, unicode.Bassa_Vah) {
		return unicode.Bassa_Vah
	} else if unicode.In(r, unicode.Batak) {
		return unicode.Batak
	} else if unicode.In(r, unicode.Bengali) {
		return unicode.Bengali
	} else if unicode.In(r, unicode.Bhaiksuki) {
		return unicode.Bhaiksuki
	} else if unicode.In(r, unicode.Bopomofo) {
		return unicode.Bopomofo
	} else if unicode.In(r, unicode.Brahmi) {
		return unicode.Brahmi
	} else if unicode.In(r, unicode.Braille) {
		return unicode.Braille
	} else if unicode.In(r, unicode.Buginese) {
		return unicode.Buginese
	} else if unicode.In(r, unicode.Buhid) {
		return unicode.Buhid
	} else if unicode.In(r, unicode.Canadian_Aboriginal) {
		return unicode.Canadian_Aboriginal
	} else if unicode.In(r, unicode.Carian) {
		return unicode.Carian
	} else if unicode.In(r, unicode.Caucasian_Albanian) {
		return unicode.Caucasian_Albanian
	} else if unicode.In(r, unicode.Chakma) {
		return unicode.Chakma
	} else if unicode.In(r, unicode.Cham) {
		return unicode.Cham
	} else if unicode.In(r, unicode.Cherokee) {
		return unicode.Cherokee
	} else if unicode.In(r, unicode.Chorasmian) {
		return unicode.Chorasmian
	} else if unicode.In(r, unicode.Common) {
		return unicode.Common
	} else if unicode.In(r, unicode.Coptic) {
		return unicode.Coptic
	} else if unicode.In(r, unicode.Cuneiform) {
		return unicode.Cuneiform
	} else if unicode.In(r, unicode.Cypriot) {
		return unicode.Cypriot
	} else if unicode.In(r, unicode.Cyrillic) {
		return unicode.Cyrillic
	} else if unicode.In(r, unicode.Deseret) {
		return unicode.Deseret
	} else if unicode.In(r, unicode.Devanagari) {
		return unicode.Devanagari
	} else if unicode.In(r, unicode.Dives_Akuru) {
		return unicode.Dives_Akuru
	} else if unicode.In(r, unicode.Dogra) {
		return unicode.Dogra
	} else if unicode.In(r, unicode.Duployan) {
		return unicode.Duployan
	} else if unicode.In(r, unicode.Egyptian_Hieroglyphs) {
		return unicode.Egyptian_Hieroglyphs
	} else if unicode.In(r, unicode.Elbasan) {
		return unicode.Elbasan
	} else if unicode.In(r, unicode.Elymaic) {
		return unicode.Elymaic
	} else if unicode.In(r, unicode.Ethiopic) {
		return unicode.Ethiopic
	} else if unicode.In(r, unicode.Georgian) {
		return unicode.Georgian
	} else if unicode.In(r, unicode.Glagolitic) {
		return unicode.Glagolitic
	} else if unicode.In(r, unicode.Gothic) {
		return unicode.Gothic
	} else if unicode.In(r, unicode.Grantha) {
		return unicode.Grantha
	} else if unicode.In(r, unicode.Greek) {
		return unicode.Greek
	} else if unicode.In(r, unicode.Gujarati) {
		return unicode.Gujarati
	} else if unicode.In(r, unicode.Gunjala_Gondi) {
		return unicode.Gunjala_Gondi
	} else if unicode.In(r, unicode.Gurmukhi) {
		return unicode.Gurmukhi
	} else if unicode.In(r, unicode.Han) {
		return unicode.Han
	} else if unicode.In(r, unicode.Hangul) {
		return unicode.Hangul
	} else if unicode.In(r, unicode.Hanifi_Rohingya) {
		return unicode.Hanifi_Rohingya
	} else if unicode.In(r, unicode.Hanunoo) {
		return unicode.Hanunoo
	} else if unicode.In(r, unicode.Hatran) {
		return unicode.Hatran
	} else if unicode.In(r, unicode.Hebrew) {
		return unicode.Hebrew
	} else if unicode.In(r, unicode.Hiragana) {
		return unicode.Hiragana
	} else if unicode.In(r, unicode.Imperial_Aramaic) {
		return unicode.Imperial_Aramaic
	} else if unicode.In(r, unicode.Inherited) {
		return unicode.Inherited
	} else if unicode.In(r, unicode.Inscriptional_Pahlavi) {
		return unicode.Inscriptional_Pahlavi
	} else if unicode.In(r, unicode.Inscriptional_Parthian) {
		return unicode.Inscriptional_Parthian
	} else if unicode.In(r, unicode.Javanese) {
		return unicode.Javanese
	} else if unicode.In(r, unicode.Kaithi) {
		return unicode.Kaithi
	} else if unicode.In(r, unicode.Kannada) {
		return unicode.Kannada
	} else if unicode.In(r, unicode.Katakana) {
		return unicode.Katakana
	} else if unicode.In(r, unicode.Kayah_Li) {
		return unicode.Kayah_Li
	} else if unicode.In(r, unicode.Kharoshthi) {
		return unicode.Kharoshthi
	} else if unicode.In(r, unicode.Khitan_Small_Script) {
		return unicode.Khitan_Small_Script
	} else if unicode.In(r, unicode.Khmer) {
		return unicode.Khmer
	} else if unicode.In(r, unicode.Khojki) {
		return unicode.Khojki
	} else if unicode.In(r, unicode.Khudawadi) {
		return unicode.Khudawadi
	} else if unicode.In(r, unicode.Lao) {
		return unicode.Lao
	} else if unicode.In(r, unicode.Latin) {
		return unicode.Latin
	} else if unicode.In(r, unicode.Lepcha) {
		return unicode.Lepcha
	} else if unicode.In(r, unicode.Limbu) {
		return unicode.Limbu
	} else if unicode.In(r, unicode.Linear_A) {
		return unicode.Linear_A
	} else if unicode.In(r, unicode.Linear_B) {
		return unicode.Linear_B
	} else if unicode.In(r, unicode.Lisu) {
		return unicode.Lisu
	} else if unicode.In(r, unicode.Lycian) {
		return unicode.Lycian
	} else if unicode.In(r, unicode.Lydian) {
		return unicode.Lydian
	} else if unicode.In(r, unicode.Mahajani) {
		return unicode.Mahajani
	} else if unicode.In(r, unicode.Makasar) {
		return unicode.Makasar
	} else if unicode.In(r, unicode.Malayalam) {
		return unicode.Malayalam
	} else if unicode.In(r, unicode.Mandaic) {
		return unicode.Mandaic
	} else if unicode.In(r, unicode.Manichaean) {
		return unicode.Manichaean
	} else if unicode.In(r, unicode.Marchen) {
		return unicode.Marchen
	} else if unicode.In(r, unicode.Masaram_Gondi) {
		return unicode.Masaram_Gondi
	} else if unicode.In(r, unicode.Medefaidrin) {
		return unicode.Medefaidrin
	} else if unicode.In(r, unicode.Meetei_Mayek) {
		return unicode.Meetei_Mayek
	} else if unicode.In(r, unicode.Mende_Kikakui) {
		return unicode.Mende_Kikakui
	} else if unicode.In(r, unicode.Meroitic_Cursive) {
		return unicode.Meroitic_Cursive
	} else if unicode.In(r, unicode.Meroitic_Hieroglyphs) {
		return unicode.Meroitic_Hieroglyphs
	} else if unicode.In(r, unicode.Miao) {
		return unicode.Miao
	} else if unicode.In(r, unicode.Modi) {
		return unicode.Modi
	} else if unicode.In(r, unicode.Mongolian) {
		return unicode.Mongolian
	} else if unicode.In(r, unicode.Mro) {
		return unicode.Mro
	} else if unicode.In(r, unicode.Multani) {
		return unicode.Multani
	} else if unicode.In(r, unicode.Myanmar) {
		return unicode.Myanmar
	} else if unicode.In(r, unicode.Nabataean) {
		return unicode.Nabataean
	} else if unicode.In(r, unicode.Nandinagari) {
		return unicode.Nandinagari
	} else if unicode.In(r, unicode.New_Tai_Lue) {
		return unicode.New_Tai_Lue
	} else if unicode.In(r, unicode.Newa) {
		return unicode.Newa
	} else if unicode.In(r, unicode.Nko) {
		return unicode.Nko
	} else if unicode.In(r, unicode.Nushu) {
		return unicode.Nushu
	} else if unicode.In(r, unicode.Nyiakeng_Puachue_Hmong) {
		return unicode.Nyiakeng_Puachue_Hmong
	} else if unicode.In(r, unicode.Ogham) {
		return unicode.Ogham
	} else if unicode.In(r, unicode.Ol_Chiki) {
		return unicode.Ol_Chiki
	} else if unicode.In(r, unicode.Old_Hungarian) {
		return unicode.Old_Hungarian
	} else if unicode.In(r, unicode.Old_Italic) {
		return unicode.Old_Italic
	} else if unicode.In(r, unicode.Old_North_Arabian) {
		return unicode.Old_North_Arabian
	} else if unicode.In(r, unicode.Old_Permic) {
		return unicode.Old_Permic
	} else if unicode.In(r, unicode.Old_Persian) {
		return unicode.Old_Persian
	} else if unicode.In(r, unicode.Old_Sogdian) {
		return unicode.Old_Sogdian
	} else if unicode.In(r, unicode.Old_South_Arabian) {
		return unicode.Old_South_Arabian
	} else if unicode.In(r, unicode.Old_Turkic) {
		return unicode.Old_Turkic
	} else if unicode.In(r, unicode.Oriya) {
		return unicode.Oriya
	} else if unicode.In(r, unicode.Osage) {
		return unicode.Osage
	} else if unicode.In(r, unicode.Osmanya) {
		return unicode.Osmanya
	} else if unicode.In(r, unicode.Pahawh_Hmong) {
		return unicode.Pahawh_Hmong
	} else if unicode.In(r, unicode.Palmyrene) {
		return unicode.Palmyrene
	} else if unicode.In(r, unicode.Pau_Cin_Hau) {
		return unicode.Pau_Cin_Hau
	} else if unicode.In(r, unicode.Phags_Pa) {
		return unicode.Phags_Pa
	} else if unicode.In(r, unicode.Phoenician) {
		return unicode.Phoenician
	} else if unicode.In(r, unicode.Psalter_Pahlavi) {
		return unicode.Psalter_Pahlavi
	} else if unicode.In(r, unicode.Rejang) {
		return unicode.Rejang
	} else if unicode.In(r, unicode.Runic) {
		return unicode.Runic
	} else if unicode.In(r, unicode.Samaritan) {
		return unicode.Samaritan
	} else if unicode.In(r, unicode.Saurashtra) {
		return unicode.Saurashtra
	} else if unicode.In(r, unicode.Sharada) {
		return unicode.Sharada
	} else if unicode.In(r, unicode.Shavian) {
		return unicode.Shavian
	} else if unicode.In(r, unicode.Siddham) {
		return unicode.Siddham
	} else if unicode.In(r, unicode.SignWriting) {
		return unicode.SignWriting
	} else if unicode.In(r, unicode.Sinhala) {
		return unicode.Sinhala
	} else if unicode.In(r, unicode.Sogdian) {
		return unicode.Sogdian
	} else if unicode.In(r, unicode.Sora_Sompeng) {
		return unicode.Sora_Sompeng
	} else if unicode.In(r, unicode.Soyombo) {
		return unicode.Soyombo
	} else if unicode.In(r, unicode.Sundanese) {
		return unicode.Sundanese
	} else if unicode.In(r, unicode.Syloti_Nagri) {
		return unicode.Syloti_Nagri
	} else if unicode.In(r, unicode.Syriac) {
		return unicode.Syriac
	} else if unicode.In(r, unicode.Tagalog) {
		return unicode.Tagalog
	} else if unicode.In(r, unicode.Tagbanwa) {
		return unicode.Tagbanwa
	} else if unicode.In(r, unicode.Tai_Le) {
		return unicode.Tai_Le
	} else if unicode.In(r, unicode.Tai_Tham) {
		return unicode.Tai_Tham
	} else if unicode.In(r, unicode.Tai_Viet) {
		return unicode.Tai_Viet
	} else if unicode.In(r, unicode.Takri) {
		return unicode.Takri
	} else if unicode.In(r, unicode.Tamil) {
		return unicode.Tamil
	} else if unicode.In(r, unicode.Tangut) {
		return unicode.Tangut
	} else if unicode.In(r, unicode.Telugu) {
		return unicode.Telugu
	} else if unicode.In(r, unicode.Thaana) {
		return unicode.Thaana
	} else if unicode.In(r, unicode.Thai) {
		return unicode.Thai
	} else if unicode.In(r, unicode.Tibetan) {
		return unicode.Tibetan
	} else if unicode.In(r, unicode.Tifinagh) {
		return unicode.Tifinagh
	} else if unicode.In(r, unicode.Tirhuta) {
		return unicode.Tirhuta
	} else if unicode.In(r, unicode.Ugaritic) {
		return unicode.Ugaritic
	} else if unicode.In(r, unicode.Vai) {
		return unicode.Vai
	} else if unicode.In(r, unicode.Wancho) {
		return unicode.Wancho
	} else if unicode.In(r, unicode.Warang_Citi) {
		return unicode.Warang_Citi
	} else if unicode.In(r, unicode.Yezidi) {
		return unicode.Yezidi
	} else if unicode.In(r, unicode.Yi) {
		return unicode.Yi
	} else if unicode.In(r, unicode.Zanabazar_Square) {
		return unicode.Zanabazar_Square
	}

	return nil
}

func PropertyTableFromRune(r rune) *unicode.RangeTable {
	if unicode.In(r, unicode.ASCII_Hex_Digit) {
		return unicode.ASCII_Hex_Digit
	} else if unicode.In(r, unicode.Bidi_Control) {
		return unicode.Bidi_Control
	} else if unicode.In(r, unicode.Dash) {
		return unicode.Dash
	} else if unicode.In(r, unicode.Deprecated) {
		return unicode.Deprecated
	} else if unicode.In(r, unicode.Diacritic) {
		return unicode.Diacritic
	} else if unicode.In(r, unicode.Extender) {
		return unicode.Extender
	} else if unicode.In(r, unicode.Hex_Digit) {
		return unicode.Hex_Digit
	} else if unicode.In(r, unicode.Hyphen) {
		return unicode.Hyphen
	} else if unicode.In(r, unicode.IDS_Binary_Operator) {
		return unicode.IDS_Binary_Operator
	} else if unicode.In(r, unicode.IDS_Trinary_Operator) {
		return unicode.IDS_Trinary_Operator
	} else if unicode.In(r, unicode.Ideographic) {
		return unicode.Ideographic
	} else if unicode.In(r, unicode.Join_Control) {
		return unicode.Join_Control
	} else if unicode.In(r, unicode.Logical_Order_Exception) {
		return unicode.Logical_Order_Exception
	} else if unicode.In(r, unicode.Noncharacter_Code_Point) {
		return unicode.Noncharacter_Code_Point
	} else if unicode.In(r, unicode.Other_Alphabetic) {
		return unicode.Other_Alphabetic
	} else if unicode.In(r, unicode.Other_Default_Ignorable_Code_Point) {
		return unicode.Other_Default_Ignorable_Code_Point
	} else if unicode.In(r, unicode.Other_Grapheme_Extend) {
		return unicode.Other_Grapheme_Extend
	} else if unicode.In(r, unicode.Other_ID_Continue) {
		return unicode.Other_ID_Continue
	} else if unicode.In(r, unicode.Other_ID_Start) {
		return unicode.Other_ID_Start
	} else if unicode.In(r, unicode.Other_Lowercase) {
		return unicode.Other_Lowercase
	} else if unicode.In(r, unicode.Other_Math) {
		return unicode.Other_Math
	} else if unicode.In(r, unicode.Other_Uppercase) {
		return unicode.Other_Uppercase
	} else if unicode.In(r, unicode.Pattern_Syntax) {
		return unicode.Pattern_Syntax
	} else if unicode.In(r, unicode.Pattern_White_Space) {
		return unicode.Pattern_White_Space
	} else if unicode.In(r, unicode.Prepended_Concatenation_Mark) {
		return unicode.Prepended_Concatenation_Mark
	} else if unicode.In(r, unicode.Quotation_Mark) {
		return unicode.Quotation_Mark
	} else if unicode.In(r, unicode.Radical) {
		return unicode.Radical
	} else if unicode.In(r, unicode.Regional_Indicator) {
		return unicode.Regional_Indicator
	} else if unicode.In(r, unicode.STerm) {
		return unicode.STerm
	} else if unicode.In(r, unicode.Sentence_Terminal) {
		return unicode.Sentence_Terminal
	} else if unicode.In(r, unicode.Soft_Dotted) {
		return unicode.Soft_Dotted
	} else if unicode.In(r, unicode.Terminal_Punctuation) {
		return unicode.Terminal_Punctuation
	} else if unicode.In(r, unicode.Unified_Ideograph) {
		return unicode.Unified_Ideograph
	} else if unicode.In(r, unicode.Variation_Selector) {
		return unicode.Variation_Selector
	} else if unicode.In(r, unicode.White_Space) {
		return unicode.White_Space
	}

	return nil
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

func charDecompMapStringToCharDecompMap(c string) DecompTag {

	switch c {
	case "<font>":
		return DecompTag_font
	case "<noBreak>":
		return DecompTag_noBreak
	case "<initial>":
		return DecompTag_initial
	case "<medial>":
		return DecompTag_medial
	case "<final>":
		return DecompTag_final
	case "<isolated>":
		return DecompTag_isolated
	case "<circle>":
		return DecompTag_circle
	case "<super>":
		return DecompTag_super
	case "<sub>":
		return DecompTag_sub
	case "<vertical>":
		return DecompTag_vertical
	case "<wide>":
		return DecompTag_wide
	case "<narrow>":
		return DecompTag_narrow
	case "<small>":
		return DecompTag_small
	case "<square>":
		return DecompTag_square
	case "<fraction>":
		return DecompTag_fraction
	case "<compat>":
		return DecompTag_compat
	case "":
		return DecompTag_NONE
	default:
		panic("unknown charDecomMap string: " + c)
	}
}

type ArabicShapingInfo struct {
	JoinType JoiningType
}

func ParseArabicShaping(arabicShapingFile string) (map[rune]ArabicShapingInfo, error) {

	type field int
	const (
		field_codeValue    field = 0
		field_charName     field = 1
		field_joiningType  field = 2
		field_joiningGroup field = 3
	)

	fBytes, err := os.ReadFile(arabicShapingFile)
	if err != nil {
		return nil, err
	}

	asInfo := map[rune]ArabicShapingInfo{}
	lines := strings.Split(string(fBytes), "\n")
	for _, l := range lines {

		if len(l) == 0 || l[0] == '#' {
			continue
		}

		fields := strings.SplitN(l, ";", 4)
		asInfo[runeFromHexCodeString(fields[field_codeValue])] = ArabicShapingInfo{
			JoinType: joiningTypeFromString(fields[field_joiningType]),
		}
	}

	return asInfo, nil
}

func joiningTypeFromString(c string) JoiningType {

	switch c {
	case " R":
		return JoiningType_Right
	case " L":
		return JoiningType_Left
	case " D":
		return JoiningType_Dual
	case " C":
		return JoiningType_Causing
	case " U":
		return JoiningType_None
	case " T":
		return JoiningType_Transparent
	default:
		panic("unknown joining type string: " + c)
	}
}
