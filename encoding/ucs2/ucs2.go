// SPDX-License-Identifier: MIT

// Package ucs2 converts between runes and the octets of SMS user data coded
// in the UCS2 alphabet.
//
// 3GPP TS 23.038 Section 6.2.3 defines the UCS2 alphabet with "Bits per
// character: 16" and "Character table: ISO/IEC 10646", which 3GPP TS 23.040
// cites as "UCS2, 16 bit coding" (its reference [24]). So UCS2 codes each
// character up to U+FFFF in one 16-bit unit, and has no coding for a
// character above U+FFFF. Neither specification states the order of the two
// octets of a unit: this package serializes the units big-endian, the order
// that ISO/IEC 10646 prefers (RFC 2781 Section 3.1). Encode codes strictly
// UCS2: it returns an error for a rune it cannot code, rather than altering
// the text.
//
// Handsets send a character above U+FFFF, such as an emoji, in user data
// coded in UCS2 as UTF-16 (RFC 2781): a surrogate pair of two 16-bit units,
// four octets in all, so such a message holds fewer characters than its octet
// count suggests. That is an extension beyond TS 23.038 Section 6.2.3, which
// EncodeUTF16 codes and Decode accepts. UTF-16 codes each character up to
// U+FFFF as UCS2 does, so Decode reads strict UCS2 too.
//
// No function reads or writes UTF-8.
package ucs2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf16"
)

// Decode converts big-endian UTF-16, as RFC 2781 defines it, into an array of
// runes.
//
// That reads UCS2, of 3GPP TS 23.038 Section 6.2.3, in which each character
// is one 16-bit unit, and, as an extension beyond that section, the UTF-16
// surrogate pairs that handsets send for characters above U+FFFF: a high
// surrogate followed by a low surrogate is decoded as one rune, as RFC 2781
// Section 2.2 decodes it.
//
// As each unit is two bytes, the length of the byte array provided must be
// even, or ErrInvalidLength is returned.
//
// Any other surrogate is unpaired, an error from which RFC 2781 Section 2.2
// leaves the recovery to the decoder ("Error recovery is not specified by
// this document"). Decode does not fail: an unpaired surrogate is decoded as
// U+FFFD and consumes only its own two bytes. If the array ends with a high
// surrogate then the runes before it are returned with an
// ErrDanglingSurrogate holding that unit, which the next segment of a
// concatenated message may complete.
func Decode(src []byte) ([]rune, error) {
	if len(src) == 0 {
		return nil, nil
	}
	if len(src)&0x01 == 0x01 {
		return nil, ErrInvalidLength
	}
	l := len(src) / 2
	dst := make([]rune, 0, l)
	for ri := 0; ri < len(src); ri += 2 {
		r := rune(binary.BigEndian.Uint16(src[ri:]))
		switch {
		case isHighSurrogate(r):
			if ri+2 == len(src) {
				// Copied, as the caller may append to it.
				return dst, ErrDanglingSurrogate(append([]byte(nil), src[ri:]...))
			}
			// Only a high surrogate followed by a low surrogate forms a pair.
			// Any other surrogate is replaced on its own, so the unit that
			// follows it is still decoded.
			r2 := rune(binary.BigEndian.Uint16(src[ri+2:]))
			if isLowSurrogate(r2) {
				r = utf16.DecodeRune(r, r2)
				ri += 2
			} else {
				r = unicode.ReplacementChar
			}
		case isLowSurrogate(r):
			r = unicode.ReplacementChar
		}
		dst = append(dst, r)
	}
	return dst, nil
}

func isHighSurrogate(r rune) bool {
	return r >= 0xd800 && r < 0xdc00
}

func isLowSurrogate(r rune) bool {
	return r >= 0xdc00 && r < 0xe000
}

// Encode converts an array of runes into UCS2, the alphabet of 3GPP TS 23.038
// Section 6.2.3 with 16 bits per character: each rune is one 16-bit unit
// holding its value, serialized big-endian, so two bytes.
//
// UCS2 has 16 bits per character, so it cannot code a character above
// U+FFFF, for which the error is an ErrUnencodable. For a rune that is not a
// Unicode scalar value, so is not a character, the error is an
// ErrInvalidRune. Either error holds the first such rune and its index, and
// no bytes are returned with it. No rune is replaced by U+FFFD, nor coded as
// a surrogate pair: EncodeUTF16 codes characters above U+FFFF as handsets do.
//
// An empty or nil array is encoded as nil.
func Encode(src []rune) ([]byte, error) {
	// Check every rune before writing any of the result.
	for i, r := range src {
		switch {
		case !isScalarValue(r):
			return nil, ErrInvalidRune{Index: i, Rune: r}
		case r > maxBMP:
			return nil, ErrUnencodable{Index: i, Rune: r}
		}
	}
	if len(src) == 0 {
		return nil, nil
	}
	dst := make([]byte, 2*len(src))
	for i, r := range src {
		binary.BigEndian.PutUint16(dst[2*i:], uint16(r))
	}
	return dst, nil
}

// EncodeUTF16 converts an array of runes into big-endian UTF-16, as RFC 2781
// defines it, two bytes per code unit.
//
// A rune up to U+FFFF is one code unit and a rune above it is a surrogate
// pair, so it takes four bytes (RFC 2781 Section 2.1). The code units are
// serialized big-endian, with no byte order mark, as for the UTF-16BE label
// (RFC 2781 Section 3.3). This is an extension beyond the UCS2 alphabet of
// 3GPP TS 23.038 Section 6.2.3, which has 16 bits per character, but it is
// how handsets send characters above U+FFFF, such as emoji, in user data
// coded in UCS2, and how they expect to receive them. Decode reads it back.
// For a message of characters up to U+FFFF only, it returns what Encode
// returns.
//
// If a rune is not a Unicode scalar value, so is not a character, no bytes
// are returned, and the error is an ErrInvalidRune holding the first such
// rune and its index. It is not replaced by U+FFFD.
//
// An empty or nil array is encoded as nil.
func EncodeUTF16(src []rune) ([]byte, error) {
	// Check every rune, and size the result, before writing any of it.
	n := 0
	for i, r := range src {
		switch {
		case !isScalarValue(r):
			return nil, ErrInvalidRune{Index: i, Rune: r}
		case r > maxBMP:
			n += 4
		default:
			n += 2
		}
	}
	if n == 0 {
		return nil, nil
	}
	dst := make([]byte, 0, n)
	for _, r := range src {
		if r <= maxBMP {
			dst = binary.BigEndian.AppendUint16(dst, uint16(r))
			continue
		}
		// RFC 2781 Section 2.1, steps 2 to 4: the 20 bits of r - 0x10000,
		// the 10 high-order bits in the high surrogate and the 10 low-order
		// bits in the low surrogate.
		u := r - 0x10000
		dst = binary.BigEndian.AppendUint16(dst, uint16(0xd800|u>>10))
		dst = binary.BigEndian.AppendUint16(dst, uint16(0xdc00|u&0x3ff))
	}
	return dst, nil
}

// maxBMP is the last code point of the Basic Multilingual Plane, the last
// that one 16-bit unit holds.
const maxBMP = 0xffff

// isScalarValue reports whether r is a Unicode scalar value, the number of a
// character, as RFC 2781 Section 2 calls it: a code point from U+0000 to
// U+10FFFF other than a surrogate, U+D800 to U+DFFF, which RFC 2781 Section 2
// says are "specifically reserved for use with UTF-16, and don't have any
// characters assigned to them".
func isScalarValue(r rune) bool {
	return r >= 0 && r < 0xd800 || r > 0xdfff && r <= unicode.MaxRune
}

// ErrInvalidRune indicates a rune that is not a Unicode scalar value, so is
// not a character to encode: a surrogate code point, U+D800 to U+DFFF, a
// negative value, or a value above U+10FFFF, the last code point.
type ErrInvalidRune struct {
	// Index is the index of Rune in the runes being encoded.
	Index int
	// Rune is the value that is not a Unicode scalar value.
	Rune rune
}

func (e ErrInvalidRune) Error() string {
	return fmt.Sprintf("ucs2: rune %s at index %d is not a Unicode scalar value", runeName(e.Rune), e.Index)
}

// runeName names r as a code point, U+ and its hex digits, or, if r is
// negative, which no code point is, as its value.
func runeName(r rune) string {
	if r < 0 {
		return fmt.Sprintf("%d", r)
	}
	return fmt.Sprintf("%U", r)
}

// ErrUnencodable indicates a character that UCS2 cannot code: a Unicode
// scalar value above U+FFFF, while UCS2 has 16 bits per character (3GPP TS
// 23.038 Section 6.2.3). EncodeUTF16 codes it as a surrogate pair.
type ErrUnencodable struct {
	// Index is the index of Rune in the runes being encoded.
	Index int
	// Rune is the character that cannot be encoded.
	Rune rune
}

func (e ErrUnencodable) Error() string {
	return fmt.Sprintf("ucs2: %q (%U) at index %d has no UCS2 encoding, as it is above U+FFFF", e.Rune, e.Rune, e.Index)
}

// ErrDanglingSurrogate indicates the byte array being decoded ends with a high
// surrogate, the first half of a surrogate pair.
//
// The error holds that final code unit so the caller can prepend it to the
// next segment of a concatenated message, which may carry the low surrogate.
// A low surrogate at the end cannot be completed by later data, so it is
// decoded as U+FFFD instead.
type ErrDanglingSurrogate []byte

func (e ErrDanglingSurrogate) Error() string {
	return fmt.Sprintf("ucs2: dangling surrogate: %#v", []byte(e))
}

var (
	// ErrInvalidLength indicates the binary provided has an invalid (odd) length.
	ErrInvalidLength = errors.New("ucs2: length must be even")
)
