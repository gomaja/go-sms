// SPDX-License-Identifier: MIT

package cbs

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/gomaja/go-sms/encoding/gsm7"
	"github.com/gomaja/go-sms/encoding/ucs2"
)

var (
	// ErrUnsupported means a TS 23.038 Section 5 coding cannot be used
	// by the text helpers: "compressed", "8 bit data", "Message with
	// User Data Header (UDH) structure", "I1 protocol message", or
	// "Defined by the WAP Forum". It is wrapped with the DCS and reason.
	ErrUnsupported = errors.New("cbs: unsupported coding")
	// ErrReserved rejects a reserved coding on encode. TS 23.038 Section 5
	// defines its meaning only for reception: "Any reserved codings shall
	// be assumed to be the GSM 7 bit default alphabet (the same as codepoint
	// 00001111) by a receiving entity." It is wrapped with the DCS.
	ErrReserved = errors.New("cbs: reserved coding")
	// ErrLanguageIndication reports a malformed or unexpected language
	// prefix of TS 23.038 Section 5, group 0001 ("message preceded by
	// language indication"), including table 5-1's reserved letter values
	// and padding: "If the padding bit is set to one, the MS shall discard
	// the message." It is wrapped with the DCS and reason.
	ErrLanguageIndication = errors.New("cbs: invalid language indication")
)

// DecodeUSSD decodes USSD octets according to TS 23.038 Section 5. Reserved
// codings "shall be assumed to be the GSM 7 bit default alphabet (the same
// as codepoint 00001111) by a receiving entity". Unsupported codings return
// a wrapped ErrUnsupported, and malformed language prefixes return a wrapped
// ErrLanguageIndication. Errors from gsm7.Decode and ucs2.Decode are wrapped.
// Text and lang are always valid UTF-8; lang is empty without an indication.
// Language() reports a DCS language independently of this message prefix.
//
// GSM7 uses the default alphabet and extension table (Sections 6.2.1 and
// 6.2.1.1) with USSD packing (Section 6.1.2.3.1): "The receiving entity
// shall remove the final <CR> character where the message ends on an octet
// boundary with <CR> as the last character." A doubled trailing CR is kept
// when it does not end on that boundary. UCS2 uses ucs2.Decode, including
// its acceptance of UTF-16 surrogate pairs. No length limit is imposed.
func (d DCS) DecodeUSSD(b []byte) (text, lang string, err error) {
	if !d.Reserved() {
		if err := d.unsupported(); err != nil {
			return "", "", err
		}
	}
	switch d.LanguageIndication() {
	case LanguageIndicationGSM7:
		septets := gsm7.Unpack7BitUSSD(b)
		// Section 5: "The first 3 characters of the message are a
		// two-character representation of the language encoded according
		// to ISO 639 [12], followed by a CR character."
		if len(septets) < 3 || septets[0] == 0x1b || septets[1] == 0x1b || septets[2] != 0x0d {
			return "", "", d.languageError("expected two language septets followed by CR")
		}
		lang, err = decodeGSM7(septets[:2])
		if err != nil {
			return "", "", err
		}
		text, err = decodeGSM7(septets[3:])
		return text, lang, err
	case LanguageIndicationUCS2:
		if len(b) < 2 {
			return "", "", d.languageError("expected two language octets")
		}
		// Section 5: "padded to the octet boundary with two bits set
		// to 0". Unpack7Bit ignores these fill bits on reception. An
		// escape would join the two septets into one character.
		septets := gsm7.Unpack7Bit(b[:2], 0)
		if septets[0] == 0x1b || septets[1] == 0x1b {
			return "", "", d.languageError("expected two language septets")
		}
		lang, err = decodeGSM7(septets)
		if err != nil {
			return "", "", err
		}
		text, err = decodeUCS2(b[2:])
		return text, lang, err
	case LanguageIndicationUCS2Letters:
		if len(b) < 2 {
			return "", "", d.languageError("expected two language octets")
		}
		// Section 5, figure 5-1 and table 5-1: 5 bits per letter,
		// MSB first, then "Padding (octet 2, bit 0)".
		first := b[0] >> 3
		second := (b[0]&7)<<2 | b[1]>>6
		third := (b[1] >> 1) & 31
		if b[1]&1 != 0 {
			return "", "", d.languageError("padding bit is set")
		}
		if first > 25 || second > 25 || third > 25 {
			return "", "", d.languageError("reserved letter value")
		}
		lang = string([]byte{'a' + first, 'a' + second, 'a' + third})
		text, err = decodeUCS2(b[2:])
		return text, lang, err
	default:
		if d.Charset() == CharsetUCS2 {
			text, err = decodeUCS2(b)
		} else {
			text, err = decodeGSM7(gsm7.Unpack7BitUSSD(b))
		}
		return text, "", err
	}
}

// EncodeUSSD encodes text using the TS 23.038 Section 5 coding in d.
// Reserved codings return a wrapped ErrReserved; unsupported codings return
// a wrapped ErrUnsupported. Text must be valid UTF-8. UCS2 is strict:
// Section 6.2.3 specifies "Bits per character: 16", so a character above
// U+FFFF is rejected. Codec errors are wrapped, preserving errors.Is/As.
// No text or octet length limit is imposed, and empty text is accepted.
//
// Without a language indication, lang must be empty. For 00010000 and
// 00010001, lang must be two ASCII letters, with case preserved; for
// 00010010 it must be three lower-case ASCII letters. Invalid lang returns
// a wrapped ErrLanguageIndication. Section 5 specifies a "two-character
// representation of the language" for 00010000, "two GSM 7-bit default
// alphabet character representation" for 00010001, and table 5-1 maps
// 00000..11001 to a..z for 00010010. No language registry is consulted.
//
// GSM7 uses gsm7.Encode and gsm7.Pack7BitUSSD. Section 6.1.2.3.1 requires
// that, for a final CR on an octet boundary, "another <CR> must be added
// together with a padding bit 0". DecodeUSSD retains both CRs, so this
// case preserves display semantics rather than exact string equality.
func (d DCS) EncodeUSSD(text, lang string) ([]byte, error) {
	if d.Reserved() {
		return nil, fmt.Errorf("encoding USSD with DCS 0x%02x: %w", byte(d), ErrReserved)
	}
	if err := d.unsupported(); err != nil {
		return nil, err
	}
	indication := d.LanguageIndication()
	if err := d.checkLanguage(lang, indication); err != nil {
		return nil, err
	}
	if d.Charset() == CharsetGSM7 {
		if indication == LanguageIndicationGSM7 {
			text = lang + "\r" + text
		}
		septets, err := gsm7.Encode([]byte(text))
		if err != nil {
			return nil, fmt.Errorf("encoding GSM7 USSD text: %w", err)
		}
		out, err := gsm7.Pack7BitUSSD(septets)
		if err != nil {
			return nil, fmt.Errorf("packing GSM7 USSD text: %w", err)
		}
		return out, nil
	}
	// []rune replaces invalid UTF-8 with U+FFFD, which UCS2 could encode.
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf("encoding UCS2 USSD text: invalid UTF-8")
	}
	body, err := ucs2.Encode([]rune(text))
	if err != nil {
		return nil, fmt.Errorf("encoding UCS2 USSD text: %w", err)
	}
	switch indication {
	case LanguageIndicationUCS2:
		// ASCII letters are the same two septets in the default alphabet.
		// Section 5 requires two zero padding bits, not USSD CR padding.
		prefix, err := gsm7.Pack7Bit([]byte(lang), 0)
		if err != nil {
			return nil, fmt.Errorf("packing USSD language: %w", err)
		}
		return append(prefix, body...), nil
	case LanguageIndicationUCS2Letters:
		// Section 5, figure 5-1 and table 5-1, with padding bit zero.
		first, second, third := lang[0]-'a', lang[1]-'a', lang[2]-'a'
		prefix := []byte{first<<3 | second>>2, second<<6 | third<<1}
		return append(prefix, body...), nil
	default:
		return body, nil
	}
}

func decodeGSM7(septets []byte) (string, error) {
	b, err := gsm7.Decode(septets)
	if err != nil {
		return "", fmt.Errorf("decoding GSM7 USSD text: %w", err)
	}
	return string(b), nil
}

func decodeUCS2(b []byte) (string, error) {
	runes, err := ucs2.Decode(b)
	if err != nil {
		return "", fmt.Errorf("decoding UCS2 USSD text: %w", err)
	}
	return string(runes), nil
}

func (d DCS) languageError(reason string) error {
	return fmt.Errorf("DCS 0x%02x (%s): %w", byte(d), reason, ErrLanguageIndication)
}

func (d DCS) checkLanguage(lang string, indication LanguageIndication) error {
	if indication == NoLanguageIndication {
		if lang != "" {
			return d.languageError("coding has no language indication")
		}
		return nil
	}
	length := 2
	if indication == LanguageIndicationUCS2Letters {
		length = 3
	}
	if len(lang) != length {
		return d.languageError(fmt.Sprintf("expected %d ASCII letters", length))
	}
	for i := range len(lang) {
		c := lang[i]
		if c >= 'a' && c <= 'z' || length == 2 && c >= 'A' && c <= 'Z' {
			continue
		}
		return d.languageError("expected ASCII letters (lower case for three-letter indication)")
	}
	return nil
}

func (d DCS) unsupported() error {
	var reason string
	switch {
	case d.Compressed():
		reason = "compressed text (TS 23.042)"
	case d.Group() == GroupUserDataHeader:
		reason = "user data header structure"
	case d.Group() == GroupI1Protocol:
		reason = "I1 protocol (TS 24.294)"
	case d.Group() == GroupWAP:
		reason = "WAP"
	case d.Charset() == Charset8Bit:
		reason = "8 bit data"
	default:
		return nil
	}
	return fmt.Errorf("USSD DCS 0x%02x (%s): %w", byte(d), reason, ErrUnsupported)
}
