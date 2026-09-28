// SPDX-License-Identifier: MIT

// Package gsm7 provides conversions to and from 7bit packed user data.
package gsm7

import (
	"fmt"

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
func (d *Decoder) Decode(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	dst := make([]byte, 0, len(src))
	escaped := false
	for _, g := range src {
		if escaped { // must be first to deal with double escapes
			escaped = false
			if g == esc {
				dst = append(dst, sp)
				continue
			}
			if m, ok := d.ext[g]; ok {
				dst = append(dst, []byte(string(m))...)
				continue
			}
			if d.strict {
				return nil, ErrInvalidSeptet(g)
			}
		} else if g == esc { // then regular escapes
			escaped = true
			continue
		}
		if m, ok := d.set[g]; ok {
			dst = append(dst, []byte(string(m))...)
			continue
		}
		if d.strict {
			return nil, ErrInvalidSeptet(g)
		}
		dst = append(dst, sp)
	}
	// handle dangling escape
	if escaped {
		dst = append(dst, sp)
	}
	return dst, nil
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

// NullDecoder fails to decode any characters.
type NullDecoder struct{}

func (o NullDecoder) applyDecoderOption(d *Decoder) {
	d.ext = noExt
}

// StrictOption specifies that the decoder should return an error rather than
// ignoring undecodable septets.
type StrictOption struct{}

func (o StrictOption) applyDecoderOption(d *Decoder) {
	d.strict = true
}

var (
	// Strict specifies that the decoder should return an error rather than
	// ignoring undecodable septets.
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

// Strict makes the Decoder return an error if an unknown character is detected
// when looking up a septet in the character set (not the extension set).
func (d Decoder) Strict() Decoder {
	d.strict = true
	return d
}

// Encode converts the src from UTF-8 to GSM7 and writes the result to dst.
//
// The return value includes the encoded GSM7 bytes, and any error that
// occurred during encoding.
func (e *Encoder) Encode(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	dst := make([]byte, 0, len(src))
	for _, u := range string(src) {
		g, ok := e.set[u]
		if ok {
			dst = append(dst, g)
			continue
		}
		g, ok = e.ext[u]
		if ok {
			dst = append(dst, esc, g)
			continue
		}
		return nil, ErrInvalidUTF8(u)
	}
	return dst, nil
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

// ErrInvalidSeptet indicates a septet cannot be decoded.
type ErrInvalidSeptet byte

func (e ErrInvalidSeptet) Error() string {
	return fmt.Sprintf("gsm7: invalid septet 0x%02x", int(e))
}

// ErrInvalidUTF8 indicates a rune cannot be converted to GSM7.
type ErrInvalidUTF8 rune

func (e ErrInvalidUTF8) Error() string {
	return fmt.Sprintf("gsm7: invalid utf8 '%c' (%U)", rune(e), int(e))
}
