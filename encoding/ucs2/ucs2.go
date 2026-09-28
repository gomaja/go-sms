// SPDX-License-Identifier: MIT

// Package ucs2 provides conversions between UCS-2 and UTF-8.
package ucs2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf16"
)

// Decode converts an array of UCS2 characters into an array of runes.
//
// As the UCS2 characters are packed into a byte array, the length of the byte
// array provided must be even.
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

// Encode converts an array of UCS2 runes into an array of bytes, where pairs
// of bytes (in Big Endian) represent a UCS2 character.
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
