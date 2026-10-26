// SPDX-License-Identifier: MIT

// Package gsm7 provides conversions to and from 7bit packed user data.
package gsm7

import (
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/gomaja/go-sms/encoding/gsm7/charset"
)

const (
	esc byte = 0x1b
	sp  byte = 0x20
)

// The character set tables used by the Encoders and Decoders of this package,
// indexed by national language identifier. Each table is built once, when
// an Encoder or Decoder first needs it, and is never handed out or changed,
// so every Encoder and Decoder can share it.
var (
	decoders    = lazyTables[charset.Decoder]{build: charset.NewDecoder}
	extDecoders = lazyTables[charset.Decoder]{build: charset.NewExtDecoder}
	encoders    = lazyTables[charset.Encoder]{build: charset.NewEncoder}
	extEncoders = lazyTables[charset.Encoder]{build: charset.NewExtEncoder}

	// noExt is the extension table of WithoutExtCharset.
	noExt = charset.Decoder{}
)

// lazyTables builds and shares one table per national language identifier.
// It must not be copied after first use.
type lazyTables[T any] struct {
	build func(nli int) T
	once  [charset.End]sync.Once
	t     [charset.End]T
}

// get returns the table for the national language identifier nli, or the
// default table for an identifier that has none, building it on first use.
func (l *lazyTables[T]) get(nli int) T {
	if nli < 0 || nli >= len(l.t) {
		nli = charset.Default
	}
	l.once[nli].Do(func() { l.t[nli] = l.build(nli) })
	return l.t[nli]
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
		d.set = decoders.get(charset.Default)
	}
	if d.ext == nil {
		d.ext = extDecoders.get(charset.Default)
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
		e.set = encoders.get(charset.Default)
	}
	if e.ext == nil {
		e.ext = extEncoders.get(charset.Default)
	}
	return e
}

// Decode converts the src from unpacked GSM7 to UTF-8.
func Decode(src []byte, options ...DecoderOption) ([]byte, error) {
	d := NewDecoder(options...)
	return d.Decode(src)
}

// DecodeReport converts src from unpacked GSM7 to UTF-8 and reports each
// replacement. It is the same as NewDecoder(options...).DecodeReport(src).
func DecodeReport(src []byte, options ...DecoderOption) ([]byte, []Replacement, error) {
	d := NewDecoder(options...)
	return d.DecodeReport(src)
}

// Encode converts the src from UTF-8 to unpacked GSM7.
//
// It is the same as NewEncoder(options...).Encode(src). See Encoder.Encode
// for the errors it returns.
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
// these it finds. DecodeReport reports each replacement made by a non-Strict
// Decoder.
func (d *Decoder) Decode(src []byte) ([]byte, error) {
	return d.decode(src, nil)
}

// A Replacement records a septet that a Decoder decoded by one of the rules
// 3GPP TS 23.038 gives a receiver, rather than as a character of its tables.
type Replacement struct {
	// Offset is the index of Septet in the septets, as in ErrInvalidSeptet.
	Offset int
	// Septet is the replaced septet, or a byte above 0x7F.
	Septet byte
	// Escaped reports that Septet follows an escape.
	Escaped bool
	// Rule is the receiver rule applied to Septet.
	Rule ReplacementRule
	// Rune is the character Septet was decoded as.
	Rune rune
}

// ReplacementRule identifies the receiver rule applied to a replaced septet.
// The zero value is not a rule.
type ReplacementRule int

const (
	// ExtensionFallback decodes an escaped septet with no character in the
	// extension set as its character in the character set (3GPP TS 23.038
	// Section 6.2.1.1).
	ExtensionFallback ReplacementRule = iota + 1
	// ReservedEscape decodes an escape after an escape as a space (3GPP TS
	// 23.038 Section 6.2.1.1, Note 1). The replacement records the second escape.
	ReservedEscape
	// DanglingEscape decodes an escape at the end of the septets as a space
	// (3GPP TS 23.038 Section 6.2.1, Note 1). Escaped is false.
	DanglingEscape
	// Unassigned decodes a septet with no character in the character set
	// (when escaped, in neither set) as a space.
	Unassigned
	// NotSeptet decodes a byte above 0x7F, which is not a septet, as a space.
	NotSeptet
)

// String returns the name of the rule, or ReplacementRule(n) for an unknown value.
func (r ReplacementRule) String() string {
	switch r {
	case ExtensionFallback:
		return "ExtensionFallback"
	case ReservedEscape:
		return "ReservedEscape"
	case DanglingEscape:
		return "DanglingEscape"
	case Unassigned:
		return "Unassigned"
	case NotSeptet:
		return "NotSeptet"
	default:
		return fmt.Sprintf("ReplacementRule(%d)", int(r))
	}
}

// DecodeReport decodes src as Decode does and reports each replaced septet
// in septet order, with the receiver rule applied and the resulting character.
// The rules are those of 3GPP TS 23.038 Sections 6.2.1 and 6.2.1.1 described
// by Decode. Replacements is nil when no septet is replaced. A Strict
// Decoder returns nil text and nil replacements on an ErrInvalidSeptet.
func (d *Decoder) DecodeReport(src []byte) ([]byte, []Replacement, error) {
	var replacements []Replacement
	dst, err := d.decode(src, &replacements)
	return dst, replacements, err
}

func (d *Decoder) decode(src []byte, replacements *[]Replacement) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	dst := make([]byte, 0, len(src))
	for i := 0; i < len(src); i++ {
		g := src[i]
		escaped := false
		if g == esc {
			i++
			if i == len(src) {
				// an escape sequence is never split, even across the
				// segments of a concatenated message (3GPP TS 23.040
				// Section 9.2.3.24.1)
				if d.strict {
					return nil, ErrInvalidSeptet{Offset: i - 1, Septet: esc}
				}
				if replacements != nil {
					*replacements = append(*replacements, Replacement{Offset: i - 1, Septet: esc, Rule: DanglingEscape, Rune: ' '})
				}
				dst = append(dst, sp)
				break
			}
			g = src[i]
			escaped = true
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
		if replacements != nil && (escaped || !ok) {
			rule := Unassigned
			switch {
			case g == esc:
				rule = ReservedEscape
			case g > 0x7f:
				rule = NotSeptet
			case escaped && ok:
				rule = ExtensionFallback
			}
			*replacements = append(*replacements, Replacement{Offset: i, Septet: g, Escaped: escaped, Rule: rule, Rune: r})
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
	d.set = decoders.get(o.nli)
}

func (o CharsetOption) applyEncoderOption(e *Encoder) {
	e.set = encoders.get(o.nli)
}

// ExtCharsetOption specifies the extension character set to be used for
// encoding and decoding.
type ExtCharsetOption struct {
	nli int
}

func (o ExtCharsetOption) applyDecoderOption(d *Decoder) {
	d.ext = extDecoders.get(o.nli)
}

func (o ExtCharsetOption) applyEncoderOption(e *Encoder) {
	e.ext = extEncoders.get(o.nli)
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

// Encode converts the src from UTF-8 to unpacked GSM7.
//
// Each character is encoded as its septet in the character set or, if the
// character set has no septet for it, as the escape 0x1B followed by its
// septet in the extension character set.
//
// Encode stops at the first of these errors, and returns no septets:
//
//   - ErrInvalidUTF8, with its offset, for a byte of src that does not begin
//     a valid UTF-8 sequence;
//   - ErrUnencodable, with the character and its offset, for a character that
//     neither the character set nor the extension character set has.
//
// An invalid byte is never read as the replacement character U+FFFD, so it
// is reported even by an Encoder whose tables have a septet for U+FFFD.
//
// A table entry that maps a rune to the escape septet 0x1B, or to a value
// above 0x7F, is treated as absent, as neither is a character (3GPP TS 23.038
// Section 6.2.1).
func (e *Encoder) Encode(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	dst := make([]byte, 0, len(src))
	for i := 0; i < len(src); {
		u, n := utf8.DecodeRune(src[i:])
		if u == utf8.RuneError && n == 1 {
			return nil, ErrInvalidUTF8{Offset: i, Byte: src[i]}
		}
		if g, ok := e.set[u]; ok && isCharacter(g) {
			dst = append(dst, g)
		} else if g, ok := e.ext[u]; ok && isCharacter(g) {
			dst = append(dst, esc, g)
		} else {
			return nil, ErrUnencodable{Offset: i, Rune: u}
		}
		i += n
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

// ErrInvalidUTF8 indicates that the source of an Encoder is not valid UTF-8
// (RFC 3629 Section 3): Byte does not begin a valid UTF-8 sequence. It is a
// byte that cannot begin a sequence, or it begins one that is truncated, that
// is longer than its character needs, or that encodes a surrogate half
// (U+D800 to U+DFFF) or a value above U+10FFFF.
type ErrInvalidUTF8 struct {
	// Offset is the index of Byte in the source.
	Offset int
	// Byte is the first byte that does not begin a valid UTF-8 sequence.
	Byte byte
}

func (e ErrInvalidUTF8) Error() string {
	return fmt.Sprintf("gsm7: invalid UTF-8 byte 0x%02x at offset %d", e.Byte, e.Offset)
}

// ErrUnencodable indicates a character that has no GSM7 encoding with the
// tables of the Encoder: neither its character set nor its extension
// character set has a septet for it.
type ErrUnencodable struct {
	// Offset is the index in the source of the first byte of Rune.
	Offset int
	// Rune is the character that cannot be encoded.
	Rune rune
}

func (e ErrUnencodable) Error() string {
	return fmt.Sprintf("gsm7: %q (%U) at offset %d has no GSM7 encoding", e.Rune, e.Rune, e.Offset)
}
