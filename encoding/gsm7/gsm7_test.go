// SPDX-License-Identifier: MIT

package gsm7_test

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/gomaja/go-sms/encoding/gsm7"
	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type decoderPattern struct {
	name string
	in   []byte
	out  []byte
	err  error
}

type encoderPattern struct {
	name string
	in   []byte
	out  []byte
	err  error
}

func testDecoder(t *testing.T, d gsm7.Decoder, patterns []decoderPattern) {
	for _, p := range patterns {
		f := func(t *testing.T) {
			out, err := d.Decode(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

func testEncoder(t *testing.T, e gsm7.Encoder, patterns []encoderPattern) {
	for _, p := range patterns {
		f := func(t *testing.T) {
			out, err := e.Encode(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

// TestConcurrentUse encodes and decodes with every character set from
// several goroutines at once. Run it with -race. It is the first test in the
// package to use the character sets, so it is also their first use.
func TestConcurrentUse(t *testing.T) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for nli := charset.Default; nli < charset.End; nli++ {
				_, _ = gsm7.Encode([]byte("A1 €"), gsm7.WithCharset(nli), gsm7.WithExtCharset(nli))
				_, _ = gsm7.Decode([]byte{0x41, 0x1b, 0x65}, gsm7.WithCharset(nli), gsm7.WithExtCharset(nli))
			}
			_, _ = gsm7.Encode([]byte("A1 €"))
			_, _ = gsm7.Decode([]byte{0x41, 0x1b, 0x65})
		}()
	}
	close(start)
	wg.Wait()
}

// TestCharsetTablesAreNotShared checks that a caller changing a table it got
// from the charset package does not change what this package encodes or
// decodes.
func TestCharsetTablesAreNotShared(t *testing.T) {
	ext := charset.DefaultExtDecoder()
	ext[0x65] = '$'
	t.Cleanup(func() { ext[0x65] = '€' })
	set := charset.DefaultEncoder()
	set['X'] = 0x00
	t.Cleanup(func() { set['X'] = 0x58 })
	tr := charset.NewDecoder(charset.Turkish)
	tr[0x00] = 'Z'
	t.Cleanup(func() { tr[0x00] = '@' })

	out, err := gsm7.Decode([]byte{0x1b, 0x65})
	require.NoError(t, err)
	assert.Equal(t, "€", string(out))
	out, err = gsm7.Encode([]byte("X"))
	require.NoError(t, err)
	assert.Equal(t, []byte{0x58}, out)
	out, err = gsm7.Decode([]byte{0x00}, gsm7.WithCharset(charset.Turkish))
	require.NoError(t, err)
	assert.Equal(t, "@", string(out))
}

func TestDecode(t *testing.T) {
	d := gsm7.NewDecoder()
	p := []decoderPattern{
		{"empty", nil, nil, nil},
		{"base", []byte("message"), []byte("message"), nil},
		{"ext", []byte("\x1b\x28\x1b\x29"), []byte("{}"), nil},
		{"escaped", []byte("mes\x1b\x40sage"), []byte("mes|sage"), nil},
		{"double escaped", []byte("mes\x1b\x1b\x40sage"), []byte("mes ¡sage"), nil},
		{"dangling escape", []byte("message\x1b"), []byte("message "), nil},
		// 3GPP TS 23.038 V20.0.0 Section 6.2.1.1: a code with no symbol in
		// the extension table is displayed as the main table character.
		{"undefined ext", []byte("\x1b\x41\x1b\x61"), []byte("Aa"), nil},
		{"not a septet", []byte("A\x80B\xff"), []byte("A B "), nil},
		{"escaped not a septet", []byte("A\x1b\x80B"), []byte("A B"), nil},
	}
	testDecoder(t, d, p)
}

func TestDecoderWithCharset(t *testing.T) {
	set := map[byte]rune{'m': 'M', 'e': 'E', 's': 'S', 'a': 'A', 'g': 'G'}
	d := gsm7.NewDecoder().WithCharset(set)
	p := []decoderPattern{
		{"base", []byte("message"), []byte("MESSAGE"), nil},
		{"ext", []byte("\x1b\x28\x1b\x29"), []byte("{}"), nil},
		{"escaped", []byte("mes\x1b\x40sage"), []byte("MES|SAGE"), nil},
		{"double escaped", []byte("mes\x1b\x1b\x40sage"), []byte("MES  SAGE"), nil},
		{"dangling escape", []byte("message\x1b"), []byte("MESSAGE "), nil},
		{"unknown", []byte("mesMsage"), []byte("MES SAGE"), nil},
	}
	testDecoder(t, d, p)
}

func TestDecoderWithExtCharset(t *testing.T) {
	ext := map[byte]rune{0x40: 'Q'}
	d := gsm7.NewDecoder().WithExtCharset(ext)
	p := []decoderPattern{
		{"base", []byte("\x40"), []byte("¡"), nil},
		{"ext", []byte("\x1b\x40"), []byte("Q"), nil},
	}
	testDecoder(t, d, p)
}

func TestDecoderStrict(t *testing.T) {
	// the entries at 0x1b and 0x80 are not characters, and are ignored
	set := map[byte]rune{'m': 'M', 'e': 'E', 's': 'S', 'a': 'A', 'g': 'G', 0x1b: 'Z', 0x80: 'Z'}
	ext := map[byte]rune{'e': 'E', 'x': 'X', 't': 'T', 0x1b: 'Z', 0x80: 'Z'}
	d := gsm7.NewDecoder().Strict().WithCharset(set).WithExtCharset(ext)
	p := []decoderPattern{
		{"known", []byte("message"), []byte("MESSAGE"), nil},
		{"ext", []byte("\x1be\x1bx\x1bt"), []byte("EXT"), nil},
		{"unknown", []byte("mesMsage"), nil,
			gsm7.ErrInvalidSeptet{Offset: 3, Septet: 'M'}},
		// 'm' is in the character set, but not in the extension set
		{"unknown ext", []byte("mes\x1bmsage"), nil,
			gsm7.ErrInvalidSeptet{Offset: 4, Septet: 'm', Escaped: true}},
		// reserved for another extension table (6.2.1.1 Note 1)
		{"double escape", []byte("mes\x1b\x1bsage"), nil,
			gsm7.ErrInvalidSeptet{Offset: 4, Septet: 0x1b, Escaped: true}},
		{"dangling escape", []byte("message\x1b"), nil,
			gsm7.ErrInvalidSeptet{Offset: 7, Septet: 0x1b}},
		{"only escape", []byte("\x1b"), nil,
			gsm7.ErrInvalidSeptet{Offset: 0, Septet: 0x1b}},
		{"not a septet", []byte("mes\x80"), nil,
			gsm7.ErrInvalidSeptet{Offset: 3, Septet: 0x80}},
		{"escaped not a septet", []byte("mes\x1b\x80"), nil,
			gsm7.ErrInvalidSeptet{Offset: 4, Septet: 0x80, Escaped: true}},
	}
	testDecoder(t, d, p)
}

// TestDecoderNotStrict decodes, with the tables of TestDecoderStrict, each
// input a Strict Decoder rejects, applying the receiver rules of 3GPP TS
// 23.038 V20.0.0 Sections 6.2.1 and 6.2.1.1.
func TestDecoderNotStrict(t *testing.T) {
	set := map[byte]rune{'m': 'M', 'e': 'E', 's': 'S', 'a': 'A', 'g': 'G', 0x1b: 'Z', 0x80: 'Z'}
	ext := map[byte]rune{'e': 'E', 'x': 'X', 't': 'T', 0x1b: 'Z', 0x80: 'Z'}
	d := gsm7.NewDecoder().WithCharset(set).WithExtCharset(ext)
	p := []decoderPattern{
		{"unknown", []byte("mesMsage"), []byte("MES SAGE"), nil},
		{"unknown ext", []byte("mes\x1bmsage"), []byte("MESMSAGE"), nil},
		{"unknown ext and set", []byte("mes\x1bMsage"), []byte("MES SAGE"), nil},
		{"double escape", []byte("mes\x1b\x1bsage"), []byte("MES SAGE"), nil},
		{"dangling escape", []byte("message\x1b"), []byte("MESSAGE "), nil},
		{"only escape", []byte("\x1b"), []byte(" "), nil},
		{"not a septet", []byte("mes\x80"), []byte("MES "), nil},
		{"escaped not a septet", []byte("mes\x1b\x80"), []byte("MES "), nil},
	}
	testDecoder(t, d, p)
}

func TestEncode(t *testing.T) {
	e := gsm7.NewEncoder()
	p := []encoderPattern{
		{"empty", nil, nil, nil},
		{"base", []byte("message"), []byte("message"), nil},
		{"ext", []byte("{}"), []byte("\x1b\x28\x1b\x29"), nil},
		{"escaped", []byte("mes|sage"), []byte("mes\x1b\x40sage"), nil},
		{"invalid", []byte("mesŞsage"), nil, gsm7.ErrInvalidUTF8('Ş')},
	}
	testEncoder(t, e, p)
}

func TestEncoderWithCharset(t *testing.T) {
	set := map[rune]byte{'Ş': 0x40}
	e := gsm7.NewEncoder().WithCharset(set)
	p := []encoderPattern{
		{"base", []byte("Ş"), []byte("\x40"), nil},
		{"ext", []byte("|"), []byte("\x1b\x40"), nil},
	}
	testEncoder(t, e, p)
}

func TestEncoderWithExtCharset(t *testing.T) {
	ext := map[rune]byte{'Ş': 0x40}
	e := gsm7.NewEncoder().WithExtCharset(ext)
	p := []encoderPattern{
		{"base", []byte("¡"), []byte("\x40"), nil},
		{"ext", []byte("Ş"), []byte("\x1b\x40"), nil},
	}
	testEncoder(t, e, p)
}

// TestErrInvalidSeptet tests that the errors can be stringified.
// It is fragile, as it compares the strings exactly, but its main purpose is
// to confirm the Error function doesn't recurse, as that is bad.
func TestErrInvalidSeptet(t *testing.T) {
	patterns := []struct {
		name string
		err  gsm7.ErrInvalidSeptet
		out  string
	}{
		{"septet", gsm7.ErrInvalidSeptet{Offset: 3, Septet: 0x4d},
			"gsm7: invalid septet 0x4d at offset 3"},
		{"octet", gsm7.ErrInvalidSeptet{Septet: 0xff},
			"gsm7: invalid septet 0xff at offset 0"},
		{"escaped", gsm7.ErrInvalidSeptet{Offset: 4, Septet: 0x6d, Escaped: true},
			"gsm7: invalid septet 0x6d after escape at offset 4"},
		{"double escape", gsm7.ErrInvalidSeptet{Offset: 4, Septet: 0x1b, Escaped: true},
			"gsm7: invalid septet 0x1b after escape at offset 4"},
		{"dangling escape", gsm7.ErrInvalidSeptet{Offset: 7, Septet: 0x1b},
			"gsm7: escape at offset 7 has no septet after it"},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			assert.Equal(t, p.out, p.err.Error())
		}
		t.Run(p.name, f)
	}
}

// TestErrInvalidUTF8 tests that the errors can be stringified.
// It is fragile, as it compares the strings exactly, but its main purpose is
// to confirm the Error function doesn't recurse, as that is bad.
func TestErrInvalidUTF8(t *testing.T) {
	patterns := []struct {
		r   rune
		out string
	}{
		{0x00, `gsm7: invalid utf8 '\x00' (U+0000)`},
		{0x0a, `gsm7: invalid utf8 '\n' (U+000A)`},
		// control characters are escaped, so none reaches a log or terminal
		{0x1b, `gsm7: invalid utf8 '\x1b' (U+001B)`},
		{0x9a, `gsm7: invalid utf8 '\u009a' (U+009A)`},
		{0xa9, `gsm7: invalid utf8 '©' (U+00A9)`},
		{0xff, `gsm7: invalid utf8 'ÿ' (U+00FF)`},
		{'€', `gsm7: invalid utf8 '€' (U+20AC)`},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			assert.Equal(t, p.out, gsm7.ErrInvalidUTF8(p.r).Error())
		}
		t.Run(fmt.Sprintf("%x", p.r), f)
	}
}

func TestWithCharset(t *testing.T) {
	out, err := gsm7.Encode([]byte("ıĞ"), gsm7.WithCharset(charset.Turkish))
	assert.Nil(t, err)
	assert.Equal(t, []byte{0x07, 0x0b}, out)
	out, err = gsm7.Decode(out, gsm7.WithCharset(charset.Turkish))
	assert.Nil(t, err)
	assert.Equal(t, []byte("ıĞ"), out)
}

func TestWithExtCharset(t *testing.T) {
	out, err := gsm7.Encode([]byte("ıĞ"), gsm7.WithExtCharset(charset.Turkish))
	assert.Nil(t, err)
	assert.Equal(t, []byte{0x1b, 0x69, 0x1b, 0x47}, out)
	out, err = gsm7.Decode(out, gsm7.WithExtCharset(charset.Turkish))
	assert.Nil(t, err)
	assert.Equal(t, []byte("ıĞ"), out)
}

// TestEncodeEscape checks that U+001B cannot be encoded with any combination
// of locking and single shift tables. Septet 0x1B is the escape to the
// extension table (3GPP TS 23.038 V20.0.0 Section 6.2.1, Note 1), not a
// character, so sending it would change the character that follows.
func TestEncodeEscape(t *testing.T) {
	for set := charset.Default; set < charset.End; set++ {
		for ext := charset.Default; ext < charset.End; ext++ {
			for _, in := range []string{"\x1b", "\x1be", "a\x1b", "\x1b<"} {
				out, err := gsm7.Encode([]byte(in), gsm7.WithCharset(set), gsm7.WithExtCharset(ext))
				assert.Equal(t, gsm7.ErrInvalidUTF8(0x1b), err, "set=%d ext=%d %q", set, ext, in)
				assert.Nil(t, out, "set=%d ext=%d %q", set, ext, in)
			}
		}
	}
}

// TestEncodeInvalidTableEntry checks that the Encoder ignores entries of a
// caller's table that map to the escape or to a value that is not a septet.
func TestEncodeInvalidTableEntry(t *testing.T) {
	for _, g := range []byte{0x1b, 0x80, 0xff} {
		bad := map[rune]byte{'x': g}
		e := gsm7.NewEncoder().WithCharset(bad).WithExtCharset(bad)
		out, err := e.Encode([]byte("x"))
		assert.Equal(t, gsm7.ErrInvalidUTF8('x'), err, "%#x", g)
		assert.Nil(t, out, "%#x", g)
		// a bad locking entry falls back to a good extension entry
		e = gsm7.NewEncoder().WithCharset(bad).WithExtCharset(map[rune]byte{'x': 0x40})
		out, err = e.Encode([]byte("x"))
		assert.NoError(t, err, "%#x", g)
		assert.Equal(t, []byte{0x1b, 0x40}, out, "%#x", g)
	}
}

// TestDecodeEscapedCR checks ESC 0x0D with every combination of locking and
// single shift tables. No extension table has a symbol at 0x0D: 3GPP TS
// 23.038 V20.0.0 Section 6.2.1.1 leaves it empty ("NOTE 2: Void") and Annex
// A.2 marks it only as a control character (Note 4). So a receiver displays
// the character the locking shift table has at 0x0D, which is CR in all of
// them, and a Strict Decoder rejects the sequence.
func TestDecodeEscapedCR(t *testing.T) {
	for set := charset.Default; set < charset.End; set++ {
		for ext := charset.Default; ext < charset.End; ext++ {
			out, err := gsm7.Decode([]byte("1\x1b\x0d2"), gsm7.WithCharset(set), gsm7.WithExtCharset(ext))
			assert.NoError(t, err, "set=%d ext=%d", set, ext)
			assert.Equal(t, "1\r2", string(out), "set=%d ext=%d", set, ext)
			out, err = gsm7.Decode([]byte("1\x1b\x0d2"),
				gsm7.WithCharset(set), gsm7.WithExtCharset(ext), gsm7.Strict)
			assert.Equal(t, gsm7.ErrInvalidSeptet{Offset: 2, Septet: 0x0d, Escaped: true}, err,
				"set=%d ext=%d", set, ext)
			assert.Nil(t, out, "set=%d ext=%d", set, ext)
		}
	}
}

// TestTeluguEuro checks that € is not sent through the Telugu single shift
// table, which has no symbol at 0x65 (3GPP TS 23.038 V20.0.0 Annex A.2.12),
// so a receiver decodes ESC 0x65 as the locking shift character at 0x65.
func TestTeluguEuro(t *testing.T) {
	telugu := []gsm7.EncoderOption{gsm7.WithCharset(charset.Telugu), gsm7.WithExtCharset(charset.Telugu)}
	_, err := gsm7.Encode([]byte("€"), telugu...)
	assert.Equal(t, gsm7.ErrInvalidUTF8('€'), err)
	_, err = gsm7.Encode([]byte("€"), gsm7.WithExtCharset(charset.Telugu))
	assert.Equal(t, gsm7.ErrInvalidUTF8('€'), err)
	// the default extension table still has € at 0x65
	out, err := gsm7.Encode([]byte("€"), gsm7.WithCharset(charset.Telugu))
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x1b, 0x65}, out)

	out, err = gsm7.Decode([]byte{0x1b, 0x65},
		gsm7.WithCharset(charset.Telugu), gsm7.WithExtCharset(charset.Telugu))
	assert.NoError(t, err)
	assert.Equal(t, "e", string(out))
	out, err = gsm7.Decode([]byte{0x1b, 0x65},
		gsm7.WithCharset(charset.Telugu), gsm7.WithExtCharset(charset.Telugu), gsm7.Strict)
	assert.Equal(t, gsm7.ErrInvalidSeptet{Offset: 1, Septet: 0x65, Escaped: true}, err)
	assert.Nil(t, out)
}

// TestUnknownLanguage checks that a reserved or unknown national language
// identifier selects the default tables, as a receiver ignores it (3GPP TS
// 23.038 Section 6.2.1.2.5).
func TestUnknownLanguage(t *testing.T) {
	for _, nli := range []int{-1, charset.End, 0xff} {
		out, err := gsm7.Encode([]byte("Ø€"), gsm7.WithCharset(nli), gsm7.WithExtCharset(nli))
		assert.NoError(t, err, "%d", nli)
		assert.Equal(t, []byte{0x0b, 0x1b, 0x65}, out, "%d", nli)
		dec, err := gsm7.Decode(out, gsm7.WithCharset(nli), gsm7.WithExtCharset(nli), gsm7.Strict)
		assert.NoError(t, err, "%d", nli)
		assert.Equal(t, "Ø€", string(dec), "%d", nli)
	}
}

// TestHindi round trips Hindi text through the Hindi locking and single shift
// tables. The septets are taken from 3GPP TS 23.038 V20.0.0 Annex A.3.6,
// where 0x00-0x02 hold the Devanagari signs candrabindu, anusvara and visarga
// (U+0901-U+0903), and A.2.6, where 0x19 holds the danda.
func TestHindi(t *testing.T) {
	patterns := []struct {
		name    string
		text    string
		septets []byte
	}{
		{"anusvara", "हिंदी", []byte{0x4d, 0x51, 0x01, 0x2b, 0x52}},
		{"candrabindu", "चाँद", []byte{0x1a, 0x50, 0x00, 0x2b}},
		{"visarga", "दुःख", []byte{0x2b, 0x53, 0x02, 0x16}},
		{"sentence", "मैं हिंदी बोलता हूँ।", []byte{
			0x42, 0x5a, 0x01, 0x20, 0x4d, 0x51, 0x01, 0x2b, 0x52, 0x20, 0x40,
			0x5d, 0x46, 0x27, 0x50, 0x20, 0x4d, 0x54, 0x00, 0x1b, 0x19,
		}},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			out, err := gsm7.Encode([]byte(p.text),
				gsm7.WithCharset(charset.Hindi), gsm7.WithExtCharset(charset.Hindi))
			require.NoError(t, err)
			assert.Equal(t, p.septets, out)
			out, err = gsm7.Decode(p.septets,
				gsm7.WithCharset(charset.Hindi), gsm7.WithExtCharset(charset.Hindi))
			require.NoError(t, err)
			assert.Equal(t, p.text, string(out))
		})
	}
	// the Bengali signs at the same code points are not in the Hindi table
	_, err := gsm7.Encode([]byte("\u0981"), gsm7.WithCharset(charset.Hindi))
	assert.Equal(t, gsm7.ErrInvalidUTF8('\u0981'), err)
}

func TestWithoutExtCharset(t *testing.T) {
	out, err := gsm7.Decode([]byte{0x1b, 0x69, 0x1b, 0x47}, gsm7.WithoutExtCharset)
	assert.Nil(t, err)
	assert.Equal(t, []byte("iG"), out)
	out, err = gsm7.Decode([]byte{0x1b, 0x69, 0x1b, 0x47}, gsm7.WithoutExtCharset, gsm7.Strict)
	assert.Equal(t, gsm7.ErrInvalidSeptet{Offset: 1, Septet: 0x69, Escaped: true}, err)
	assert.Nil(t, out)
}

// The tables of every national language identifier, for the fuzz oracles.
var (
	lockingDecoders [charset.End]charset.Decoder
	shiftDecoders   [charset.End]charset.Decoder
	lockingEncoders [charset.End]charset.Encoder
	shiftEncoders   [charset.End]charset.Encoder
)

func init() {
	for nli := charset.Default; nli < charset.End; nli++ {
		lockingDecoders[nli] = charset.NewDecoder(nli)
		shiftDecoders[nli] = charset.NewExtDecoder(nli)
		lockingEncoders[nli] = charset.NewEncoder(nli)
		shiftEncoders[nli] = charset.NewExtEncoder(nli)
	}
}

// tableText returns every character of the locking and single shift tables
// of nli, in septet order.
func tableText(nli int) string {
	var b strings.Builder
	for _, d := range []charset.Decoder{lockingDecoders[nli], shiftDecoders[nli]} {
		for g := 0; g < 0x80; g++ {
			if r, ok := d[byte(g)]; ok {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// FuzzEncode encodes text with every combination of locking and single
// shift tables. Encoding must fail on the first character neither table
// has, and otherwise produce septets that a Strict Decoder turns back into
// the text.
func FuzzEncode(f *testing.F) {
	for nli := charset.Default; nli < charset.End; nli++ {
		f.Add(tableText(nli), uint8(nli), uint8(nli))
		f.Add(tableText(nli), uint8(charset.Default), uint8(nli))
	}
	f.Add("hello {€} \x1be \r\n", uint8(0), uint8(0))
	f.Add("\xff\xfe", uint8(0), uint8(0))
	f.Fuzz(func(t *testing.T, s string, set, ext uint8) {
		ls, ss := int(set)%charset.End, int(ext)%charset.End
		out, err := gsm7.Encode([]byte(s), gsm7.WithCharset(ls), gsm7.WithExtCharset(ss))
		for _, r := range s {
			_, inSet := lockingEncoders[ls][r]
			_, inExt := shiftEncoders[ss][r]
			if !inSet && !inExt {
				if err != gsm7.ErrInvalidUTF8(r) || out != nil {
					t.Fatalf("set %d ext %d encode %q: got % x, %v, want %v", ls, ss, s, out, err, gsm7.ErrInvalidUTF8(r))
				}
				return
			}
		}
		if err != nil {
			t.Fatalf("set %d ext %d encode %q: %v", ls, ss, s, err)
		}
		for i, g := range out {
			if g > 0x7f || (g == 0x1b && (i+1 == len(out) || out[i+1] == 0x1b)) {
				t.Fatalf("set %d ext %d encode %q: invalid septets % x", ls, ss, s, out)
			}
		}
		got, err := gsm7.Decode(out, gsm7.WithCharset(ls), gsm7.WithExtCharset(ss), gsm7.Strict)
		if err != nil || string(got) != s {
			t.Fatalf("set %d ext %d encode %q -> % x -> decode %q, %v", ls, ss, s, out, got, err)
		}
	})
}

// FuzzDecode decodes arbitrary bytes with every combination of locking and
// single shift tables. It checks the result against a walk of the receiver
// rules of 3GPP TS 23.038 Sections 6.2.1 and 6.2.1.1, and that a Strict
// Decoder fails exactly where that walk finds no character, and otherwise
// agrees with the lenient one.
func FuzzDecode(f *testing.F) {
	all := make([]byte, 0x100)
	escaped := make([]byte, 0, 0x200)
	for g := range all {
		all[g] = byte(g)
		escaped = append(escaped, 0x1b, byte(g))
	}
	for nli := charset.Default; nli < charset.End; nli++ {
		f.Add(all[:0x80], uint8(nli), uint8(nli))
		f.Add(escaped[:0x100], uint8(nli), uint8(nli))
	}
	f.Add(all, uint8(0), uint8(0))
	f.Add(escaped, uint8(0), uint8(0))
	f.Add([]byte("mes\x1b\x1b\x40sage\x1b"), uint8(0), uint8(0))
	f.Add([]byte("message\x1b"), uint8(0), uint8(0))
	f.Add([]byte{0x1b}, uint8(0), uint8(0))
	f.Fuzz(func(t *testing.T, src []byte, set, ext uint8) {
		ls, ss := int(set)%charset.End, int(ext)%charset.End
		lock, shift := lockingDecoders[ls], shiftDecoders[ss]
		char := func(d charset.Decoder, g byte) (rune, bool) {
			r, ok := d[g]
			return r, ok && g < 0x80 && g != 0x1b
		}
		var want []rune
		var wantErr error
		for i := 0; i < len(src); i++ {
			if src[i] != 0x1b {
				r, ok := char(lock, src[i])
				if !ok {
					r = ' '
					if wantErr == nil {
						wantErr = gsm7.ErrInvalidSeptet{Offset: i, Septet: src[i]}
					}
				}
				want = append(want, r)
				continue
			}
			if i+1 == len(src) {
				want = append(want, ' ')
				if wantErr == nil {
					wantErr = gsm7.ErrInvalidSeptet{Offset: i, Septet: 0x1b}
				}
				break
			}
			i++
			if r, ok := char(shift, src[i]); ok {
				want = append(want, r)
				continue
			}
			if wantErr == nil {
				wantErr = gsm7.ErrInvalidSeptet{Offset: i, Septet: src[i], Escaped: true}
			}
			r, ok := char(lock, src[i])
			if !ok {
				r = ' '
			}
			want = append(want, r)
		}

		opts := []gsm7.DecoderOption{gsm7.WithCharset(ls), gsm7.WithExtCharset(ss)}
		got, err := gsm7.Decode(src, opts...)
		if err != nil || string(got) != string(want) || !utf8.Valid(got) {
			t.Fatalf("set %d ext %d decode % x: got %q, %v, want %q", ls, ss, src, got, err, string(want))
		}
		strict, err := gsm7.Decode(src, append(opts, gsm7.Strict)...)
		if err != wantErr {
			t.Fatalf("set %d ext %d strict decode % x: got %v, want %v", ls, ss, src, err, wantErr)
		}
		if err != nil {
			if strict != nil {
				t.Fatalf("set %d ext %d strict decode % x: got %q with %v", ls, ss, src, strict, err)
			}
			return
		}
		if !bytes.Equal(strict, got) {
			t.Fatalf("set %d ext %d decode % x: strict %q, lenient %q", ls, ss, src, strict, got)
		}
		// what a Strict Decoder accepts encodes again, to septets that
		// decode to the same text
		enc, err := gsm7.Encode(strict, gsm7.WithCharset(ls), gsm7.WithExtCharset(ss))
		if err != nil {
			t.Fatalf("set %d ext %d decode % x -> %q -> encode: %v", ls, ss, src, strict, err)
		}
		again, err := gsm7.Decode(enc, append(opts, gsm7.Strict)...)
		if err != nil || !bytes.Equal(again, strict) {
			t.Fatalf("set %d ext %d decode % x -> %q -> % x -> %q, %v", ls, ss, src, strict, enc, again, err)
		}
	})
}
