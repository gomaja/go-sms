// SPDX-License-Identifier: MIT

// Package charset provides encoders and decoders for GSM character sets.
//
// The tables are built once, when the package is initialised, and are never
// handed out. Each function returns a new copy of its table, so a caller may
// change the table it gets without affecting any other caller.
package charset

import "maps"

// DefaultDecoder returns the default mapping table from GSM7 to UTF8.
func DefaultDecoder() Decoder {
	return maps.Clone(defaultDecoder)
}

// NewDecoder returns the mapping table from GSM7 to UTF8 for the given language.
//
// It returns the default table for Spanish, which has no locking shift table
// (3GPP TS 23.038 Annex A.3.2 is Void), and for an identifier that is
// reserved or unknown, as a receiver ignores those (Section 6.2.1.2.5).
func NewDecoder(nli int) Decoder {
	if d, ok := decoders[nli]; ok {
		return maps.Clone(d)
	}
	return DefaultDecoder()
}

// DefaultExtDecoder returns the default extension mapping table from GSM7 to UTF8.
func DefaultExtDecoder() Decoder {
	return maps.Clone(defaultExtDecoder)
}

// NewExtDecoder returns the extension mapping table from GSM7 to UTF8 for the given language.
//
// It returns the default table for an identifier that is reserved or
// unknown, as a receiver ignores those (3GPP TS 23.038 Section 6.2.1.2.5).
func NewExtDecoder(nli int) Decoder {
	if d, ok := extDecoders[nli]; ok {
		return maps.Clone(d)
	}
	return DefaultExtDecoder()
}

// DefaultEncoder returns the default mapping table from UTF8 to GSM7.
func DefaultEncoder() Encoder {
	return maps.Clone(defaultEncoder)
}

// NewEncoder returns the mapping table from UTF8 to GSM7 for the given language.
//
// It returns the default table where NewDecoder does.
func NewEncoder(nli int) Encoder {
	if e, ok := encoders[nli]; ok {
		return maps.Clone(e)
	}
	return DefaultEncoder()
}

// DefaultExtEncoder returns the default extension mapping table from UTF8 to GSM7.
func DefaultExtEncoder() Encoder {
	return maps.Clone(defaultExtEncoder)
}

// NewExtEncoder returns the extension mapping table from UTF8 to GSM7 for the given language.
//
// It returns the default table where NewExtDecoder does.
func NewExtEncoder(nli int) Encoder {
	if e, ok := extEncoders[nli]; ok {
		return maps.Clone(e)
	}
	return DefaultExtEncoder()
}

// Decoder provides a mapping from GSM7 byte to UTF8 rune.
//
// A Decoder returned by this package is a copy that belongs to the caller.
type Decoder map[byte]rune

// Encoder provides a mapping from UTF8 rune to GSM7 byte.
//
// An Encoder returned by this package is a copy that belongs to the caller.
type Encoder map[rune]byte

// NationalLanguageIdentifier indicates the character set in use, as defined in
// 3GPP TS 23.038 Section 6.2.1.2.4.
type NationalLanguageIdentifier int

const (
	// Default character set.
	Default int = iota
	// Turkish character set.
	Turkish
	// Spanish character set
	Spanish
	// Portuguese character set
	Portuguese
	// Bengali character set
	Bengali
	// Gujarati character set
	Gujarati
	// Hindi character set
	Hindi
	// Kannada character set
	Kannada
	// Malayalam character set
	Malayalam
	// Oriya character set
	Oriya
	// Punjabi character set
	Punjabi
	// Tamil character set
	Tamil
	// Telugu character set
	Telugu
	// Urdu character set
	Urdu

	// Helper consts for creating and iterating over slices

	// End marker for loops (exclusive)
	End
	// Start point for loops (inclusive)
	Start = Turkish
	// Size is for array sizing (excluding default)
	Size = End - Start
)

func generateEncoder(d Decoder) Encoder {
	e := make(Encoder, len(d))
	for k, v := range d {
		if ko, ok := e[v]; !ok || ko > k {
			e[v] = k
		}
	}
	return e
}

// esc is the escape to the extension table. It has a place in the rune
// tables, but it is not a character, so it has no entry in any Encoder or
// Decoder (3GPP TS 23.038 Section 6.2.1, Note 1).
const esc = 0x1b

func generateEncoderFromRunes(runes []rune) Encoder {
	e := make(Encoder, len(runes))
	for i, r := range runes {
		if i != esc {
			e[r] = byte(i)
		}
	}
	return e
}

func generateDecoderFromRunes(runes []rune) Decoder {
	dset := make(Decoder, len(runes))
	for i, r := range runes {
		if i != esc {
			dset[byte(i)] = r
		}
	}
	return dset
}

var (
	decoders = map[int]Decoder{
		Turkish: turkishDecoder,
		// Spanish uses default
		Portuguese: portugueseDecoder,
		Bengali:    bengaliDecoder,
		Gujarati:   gujaratiDecoder,
		Hindi:      hindiDecoder,
		Kannada:    kannadaDecoder,
		Malayalam:  malayalamDecoder,
		Oriya:      oriyaDecoder,
		Punjabi:    punjabiDecoder,
		Tamil:      tamilDecoder,
		Telugu:     teluguDecoder,
		Urdu:       urduDecoder,
	}
	// The national language single shift tables of 3GPP TS 23.038 V20.0.0
	// Annex A.2 have no symbol at 0x0D, which they mark only as a control
	// character (Note 4), so none of these has an entry for it and ESC 0x0D
	// decodes as the locking shift table's CR.
	extDecoders = map[int]Decoder{
		Turkish:    turkishExtDecoder,
		Spanish:    spanishExtDecoder,
		Portuguese: portugueseExtDecoder,
		Bengali:    bengaliExtDecoder,
		Gujarati:   gujaratiExtDecoder,
		Hindi:      hindiExtDecoder,
		Kannada:    kannadaExtDecoder,
		Malayalam:  malayalamExtDecoder,
		Oriya:      oriyaExtDecoder,
		Punjabi:    punjabiExtDecoder,
		Tamil:      tamilExtDecoder,
		Telugu:     teluguExtDecoder,
		Urdu:       urduExtDecoder,
	}
	encoders = map[int]Encoder{
		Turkish: turkishEncoder,
		// Spanish uses default
		Portuguese: portugueseEncoder,
		Bengali:    bengaliEncoder,
		Gujarati:   gujaratiEncoder,
		Hindi:      hindiEncoder,
		Kannada:    kannadaEncoder,
		Malayalam:  malayalamEncoder,
		Oriya:      oriyaEncoder,
		Punjabi:    punjabiEncoder,
		Tamil:      tamilEncoder,
		Telugu:     teluguEncoder,
		Urdu:       urduEncoder,
	}
	extEncoders = map[int]Encoder{
		Turkish:    turkishExtEncoder,
		Spanish:    spanishExtEncoder,
		Portuguese: portugueseExtEncoder,
		Bengali:    bengaliExtEncoder,
		Gujarati:   gujaratiExtEncoder,
		Hindi:      hindiExtEncoder,
		Kannada:    kannadaExtEncoder,
		Malayalam:  malayalamExtEncoder,
		Oriya:      oriyaExtEncoder,
		Punjabi:    punjabiExtEncoder,
		Tamil:      tamilExtEncoder,
		Telugu:     teluguExtEncoder,
		Urdu:       urduExtEncoder,
	}
)
