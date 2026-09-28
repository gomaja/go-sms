// SPDX-License-Identifier: MIT

// Package gsm7 provides conversions to and from 7bit packed user data.
package gsm7

import (
	"fmt"
	"unicode/utf8"

	"github.com/gomaja/go-sms/encoding/gsm7/charset"
)

const (
	esc byte = 0x1b
	sp  byte = 0x20
)

// The character set tables used by the Encoders and Decoders of this package,
// indexed by national language identifier. They are built once from the
// copies the charset package returns, and are never handed out or changed,
// so every Encoder and Decoder can share them.
var (
	decoders    = tables(charset.NewDecoder)
	extDecoders = tables(charset.NewExtDecoder)
	encoders    = tables(charset.NewEncoder)
	extEncoders = tables(charset.NewExtEncoder)

	// noExt is the extension table of WithoutExtCharset.
	noExt = charset.Decoder{}
)

// tables builds the table for every national language identifier.
func tables[T any](f func(nli int) T) (t [charset.End]T) {
	for nli := range t {
		t[nli] = f(nli)
	}
	return t
}

// table returns the table for the national language identifier nli, or the
// default table for an identifier that has none.
func table[T any](t *[charset.End]T, nli int) T {
	if nli < 0 || nli >= len(t) {
		nli = charset.Default
	}
	return t[nli]
}

// Decoder converts from GSM7 to UTF-8 using a particular character set.
type Decoder struct {
	set    charset.Decoder
	ext    charset.Decoder
	strict bool
}

// Encoder converts from UTF-8 to GSM7 using a particular character set.
type Encoder struct {
	set charset.Encoder
	ext charset.Encoder
}

// DecoderOption applies an option to a Decoder.
type DecoderOption interface {
	applyDecoderOption(*Decoder)
}

// EncoderOption applies an option to an Encoder.
type EncoderOption interface {
	applyEncoderOption(*Encoder)
}

// NewDecoder returns a new GSM7 decoder which uses the default character set.
func NewDecoder(options ...DecoderOption) Decoder {
	d := Decoder{}
	for _, option := range options {
		option.applyDecoderOption(&d)
	}
	if d.set == nil {
		d.set = decoders[charset.Default]
	}
	if d.ext == nil {
		d.ext = extDecoders[charset.Default]
	}
	return d
}

// NewEncoder returns a new GSM7 encoder which uses the default character set.
func NewEncoder(options ...EncoderOption) Encoder {
	e := Encoder{}
	for _, option := range options {
		option.applyEncoderOption(&e)
	}
	if e.set == nil {
		e.set = encoders[charset.Default]
	}
	if e.ext == nil {
		e.ext = extEncoders[charset.Default]
	}
	return e
}

// Decode converts the src from unpacked GSM7 to UTF-8.
func Decode(src []byte, options ...DecoderOption) ([]byte, error) {
	d := NewDecoder(options...)
	return d.Decode(src)
}

// Encode converts the src from UTF-8 to GSM7 and writes the result to dst.
//
// The return value includes the encoded GSM7 bytes, and any error that
// occurred during encoding.
func Encode(src []byte, options ...EncoderOption) ([]byte, error) {
	e := NewEncoder(options...)
	return e.Encode(src)
}

// Decode converts the src from unpacked GSM7 to UTF-8.
//
// Each septet is looked up in the character set, except that the escape
// 0x1B makes the septet after it be looked up in the extension character set.
// Where the tables have no character, the Decoder applies the rules 3GPP TS
// 23.038 gives a receiver:
//
//   - a septet after an escape that has no character in the extension set is
//     decoded as the character the septet has in the character set (Section
//     6.2.1.1, "the MS shall display either the character shown in the main
//     GSM 7 bit default alphabet table ... or the character from the National
//     Language Locking Shift Table");
//   - an escape after an escape, which is "reserved for the extension to
//     another extension table", is decoded as a space (Section 6.2.1.1, Note
//     1), and the septet after it is decoded normally;
//   - an escape at the end of src is decoded as a space, as a receiver that
//     does not understand an escape "shall display it as a space character"
//     (Section 6.2.1, Note 1);
//   - a septet with no character in the character set, and a byte above 0x7F,
//     which is not a septet, are decoded as a space.
//
// A Strict Decoder instead returns an ErrInvalidSeptet for the first of
// these it finds.
func (d *Decoder) Decode(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	dst := make([]byte, 0, len(src))
	for i := 0; i < len(src); i++ {
		g := src[i]
		if g == esc {
			i++
			if i == len(src) {
				// an escape sequence is never split, even across the
				// segments of a concatenated message (3GPP TS 23.040
				// Section 9.2.3.24.1)
				if d.strict {
					return nil, ErrInvalidSeptet{Offset: i - 1, Septet: esc}
				}
				dst = append(dst, sp)
				break
			}
			g = src[i]
			if r, ok := lookup(d.ext, g); ok {
				dst = utf8.AppendRune(dst, r)
				continue
			}
			if d.strict {
				return nil, ErrInvalidSeptet{Offset: i, Septet: g, Escaped: true}
			}
			// fall back to the character set, which has no character for
			// a second escape, so that becomes a space
		}
		r, ok := lookup(d.set, g)
		if !ok {
			if d.strict {
				return nil, ErrInvalidSeptet{Offset: i, Septet: g}
			}
			r = rune(sp)
		}
		dst = utf8.AppendRune(dst, r)
	}
	return dst, nil
}

// lookup returns the character for septet g in the table t. A table entry for
// the escape, or for a value above 0x7F, is treated as absent, as neither is
// a character.
func lookup(t charset.Decoder, g byte) (rune, bool) {
	if !isCharacter(g) {
		return 0, false
	}
	r, ok := t[g]
	return r, ok
}

// CharsetOption specifies the character set to be used for encoding and
// decoding.
type CharsetOption struct {
	nli int
}

func (o CharsetOption) applyDecoderOption(d *Decoder) {
	d.set = table(&decoders, o.nli)
}

func (o CharsetOption) applyEncoderOption(e *Encoder) {
	e.set = table(&encoders, o.nli)
}

// ExtCharsetOption specifies the extension character set to be used for
// encoding and decoding.
type ExtCharsetOption struct {
	nli int
}

func (o ExtCharsetOption) applyDecoderOption(d *Decoder) {
	d.ext = table(&extDecoders, o.nli)
}

func (o ExtCharsetOption) applyEncoderOption(e *Encoder) {
	e.ext = table(&extEncoders, o.nli)
}

// WithCharset specifies the character set map used for encoding or decoding.
func WithCharset(nli int) CharsetOption {
	return CharsetOption{nli}
}

// WithExtCharset replaces the extension character set map used for encoding or
// decoding.
func WithExtCharset(nli int) ExtCharsetOption {
	return ExtCharsetOption{nli}
}

// NullDecoder is the DecoderOption of WithoutExtCharset. It removes the
// extension character set, so the Decoder decodes every escaped septet from
// the character set, or rejects it if the Decoder is Strict.
type NullDecoder struct{}

func (o NullDecoder) applyDecoderOption(d *Decoder) {
	d.ext = noExt
}

// StrictOption is the DecoderOption of Strict.
type StrictOption struct{}

func (o StrictOption) applyDecoderOption(d *Decoder) {
	d.strict = true
}

var (
	// Strict makes the Decoder return an ErrInvalidSeptet, rather than a
	// substitute character, for any septet or escape sequence that has no
	// character in its tables. See Decoder.Decode.
	Strict = StrictOption{}

	// WithoutExtCharset specifies that no extension character set will be
	// available to decode escaped characters.
	WithoutExtCharset = NullDecoder{}
)

// WithCharset replaces the character set map used by the Decoder.
func (d Decoder) WithCharset(set charset.Decoder) Decoder {
	d.set = set
	return d
}

// WithExtCharset replaces the extension character set map used by the Decoder.
func (d Decoder) WithExtCharset(ext charset.Decoder) Decoder {
	d.ext = ext
	return d
}

// Strict makes the Decoder return an ErrInvalidSeptet, rather than a
// substitute character, for any septet or escape sequence that has no
// character in its tables: a septet with no character in the character set,
// an escaped septet with no character in the extension set (even if it has
// one in the character set), an escape followed by another escape, an escape
// at the end of the source, and a byte above 0x7F. See Decode.
func (d Decoder) Strict() Decoder {
	d.strict = true
	return d
}

// Encode converts the src from UTF-8 to GSM7 and writes the result to dst.
//
// The return value includes the encoded GSM7 bytes, and any error that
// occurred during encoding.
//
// A table entry that maps a rune to the escape septet 0x1B, or to a value
// above 0x7F, is treated as absent, as neither is a character (3GPP TS 23.038
// Section 6.2.1).
func (e *Encoder) Encode(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	dst := make([]byte, 0, len(src))
	for _, u := range string(src) {
		g, ok := e.set[u]
		if ok && isCharacter(g) {
			dst = append(dst, g)
			continue
		}
		g, ok = e.ext[u]
		if ok && isCharacter(g) {
			dst = append(dst, esc, g)
			continue
		}
		return nil, ErrInvalidUTF8(u)
	}
	return dst, nil
}

// isCharacter reports whether g is a septet that encodes a character, which
// is any septet but the escape.
func isCharacter(g byte) bool {
	return g <= 0x7f && g != esc
}

// WithCharset replaces the character set map used by the Encoder.
func (e Encoder) WithCharset(set charset.Encoder) Encoder {
	e.set = set
	return e
}

// WithExtCharset replaces the extension character set map used by the Encoder.
func (e Encoder) WithExtCharset(ext charset.Encoder) Encoder {
	e.ext = ext
	return e
}

// ErrInvalidSeptet indicates a septet that has no character.
//
// An escape (0x1B) at the end of the septets, with no septet after it, is
// reported as Septet 0x1B at the offset of the escape, with Escaped false.
type ErrInvalidSeptet struct {
	// Offset is the index of Septet in the septets.
	Offset int
	// Septet is the septet that has no character.
	Septet byte
	// Escaped reports that Septet follows an escape, so was looked up in the
	// extension character set.
	Escaped bool
}

func (e ErrInvalidSeptet) Error() string {
	switch {
	case e.Escaped:
		return fmt.Sprintf("gsm7: invalid septet 0x%02x after escape at offset %d", e.Septet, e.Offset)
	case e.Septet == esc:
		return fmt.Sprintf("gsm7: escape at offset %d has no septet after it", e.Offset)
	default:
		return fmt.Sprintf("gsm7: invalid septet 0x%02x at offset %d", e.Septet, e.Offset)
	}
}

// ErrInvalidUTF8 indicates a rune cannot be converted to GSM7.
type ErrInvalidUTF8 rune

func (e ErrInvalidUTF8) Error() string {
	return fmt.Sprintf("gsm7: invalid utf8 %q (%U)", rune(e), rune(e))
}
