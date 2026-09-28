// SPDX-License-Identifier: MIT

package gsm7_test

import (
	"fmt"
	"sync"
	"testing"

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
	set := map[byte]rune{'m': 'M', 'e': 'E', 's': 'S', 'a': 'A', 'g': 'G'}
	ext := map[byte]rune{'e': 'E', 'x': 'X', 't': 'T'}
	d := gsm7.NewDecoder().Strict().WithCharset(set).WithExtCharset(ext)
	p := []decoderPattern{
		{"known", []byte("message"), []byte("MESSAGE"), nil},
		{"ext", []byte("\x1be\x1bx\x1bt"), []byte("EXT"), nil},
		{"unknown", []byte("mesMsage"), nil, gsm7.ErrInvalidSeptet('M')},
		{"unknown ext", []byte("mes\x1bmsage"), nil, gsm7.ErrInvalidSeptet('m')},
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
	patterns := []byte{0x00, 0xa0, 0x0a, 0x9a, 0xa9, 0xff}
	for _, p := range patterns {
		f := func(t *testing.T) {
			err := gsm7.ErrInvalidSeptet(p)
			expected := fmt.Sprintf("gsm7: invalid septet 0x%02x", int(err))
			s := err.Error()
			assert.Equal(t, expected, s)
		}
		t.Run(fmt.Sprintf("%x", p), f)
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
	_, err := gsm7.Encode([]byte("ঁ"), gsm7.WithCharset(charset.Hindi))
	assert.Equal(t, gsm7.ErrInvalidUTF8('ঁ'), err)
}

func TestWithoutExtCharset(t *testing.T) {
	out, err := gsm7.Decode([]byte{0x1b, 0x69, 0x1b, 0x47}, gsm7.WithoutExtCharset)
	assert.Nil(t, err)
	assert.Equal(t, []byte("iG"), out)
	out, err = gsm7.Decode([]byte{0x1b, 0x69, 0x1b, 0x47}, gsm7.WithoutExtCharset, gsm7.Strict)
	assert.Equal(t, gsm7.ErrInvalidSeptet(0x69), err)
	assert.Nil(t, out)
}
