// SPDX-License-Identifier: MIT

// Package ucs2 converts between runes and the octets of SMS user data coded
// in the UCS2 alphabet.
//
// 3GPP TS 23.038 Section 6.2.3 defines the UCS2 alphabet as 16 bits per
// character, with ISO/IEC 10646 as the character table. As is common practice,
// this package codes the characters as UTF-16: a code point up to U+FFFF is
// one big-endian 16-bit code unit, and a code point above U+FFFF is a
// surrogate pair of two code units, four octets in all. So a message that
// contains such code points holds fewer characters than the octet count
// suggests. Neither function reads or writes UTF-8.
package ucs2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf16"
)

// Decode converts an array of big-endian UTF-16 code units into an array of
// runes.
//
// As each code unit is two bytes, the length of the byte array provided must
// be even.
//
// A high surrogate followed by a low surrogate is decoded as one rune. Any
// other surrogate is decoded as U+FFFD and consumes only its own two bytes. If
// the array ends with a high surrogate then the runes before it are returned
// with an ErrDanglingSurrogate holding that code unit.
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

// Encode converts an array of runes into an array of big-endian UTF-16 code
// units, two bytes per code unit.
//
// A rune above U+FFFF is encoded as a surrogate pair, so it takes four bytes.
// A rune that UTF-16 cannot represent, i.e. a surrogate code point or a value
// outside 0..U+10FFFF, is encoded as U+FFFD.
func Encode(src []rune) []byte {
	if len(src) == 0 {
		return nil
	}
	u := utf16.Encode(src)
	dst := make([]byte, len(u)*2)
	wi := 0
	for _, r := range u {
		binary.BigEndian.PutUint16(dst[wi:], uint16(r))
		wi += 2
	}
	return dst
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
// that one 16-bit code unit holds.
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
