// SPDX-License-Identifier: MIT

package cbs_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gomaja/go-sms/encoding/cbs"
	"github.com/gomaja/go-sms/encoding/gsm7"
	"github.com/gomaja/go-sms/encoding/ucs2"
)

type ussdVector struct {
	name            string
	dcs             cbs.DCS
	text, lang, hex string
	decoded         string // only set where the receiver retains the doubled CR
}

// These octets are literal vectors from TS 23.038 V20.0.0 Sections 5,
// 6.1.2.3.1, 6.2.1 and 6.2.3. The plain-text vectors also exercise the
// cases in go-asn1-gsmmap's ussd_7bit_test.go and ussd_golden_test.go.
var ussdVectors = []ussdVector{
	{"seven", cbs.GSM7, "ABCDEFG", "", "41e19058341e1b", ""},
	{"seven at", cbs.GSM7, "ABCDEF@", "", "41e1905834021a", ""},
	{"seven CR", cbs.GSM7, "ABCDEF\r", "", "41e1905834361a", ""},
	{"eight", cbs.GSM7, "ABCDEFGH", "", "41e19058341e91", ""},
	{"eight CR", cbs.GSM7, "ABCDEFG\r", "", "41e19058341e1b0d", "ABCDEFG\r\r"},
	{"eight at", cbs.GSM7, "ABCDEFG@", "", "41e19058341e01", ""},
	{"digits symbols", cbs.GSM7, "*123*456*789*0#", "", "aa986ca6a2d56caa1b2ea7828d1a", ""},
	{"fifteen at", cbs.GSM7, "ABCDEFGHIJKLMN@", "", "41e19058341e9149e592d974021a", ""},
	{"fifteen CR", cbs.GSM7, "ABCDEFGHIJKLMN\r", "", "41e19058341e9149e592d974361a", ""},
	{"sixteen", cbs.GSM7, "ABCDEFGHIJKLMNOP", "", "41e19058341e9149e592d9743ea1", ""},
	{"sixteen CR", cbs.GSM7, "ABCDEFGHIJKLMNO\r", "", "41e19058341e9149e592d9743e1b0d", "ABCDEFGHIJKLMNO\r\r"},
	{"sixteen at", cbs.GSM7, "ABCDEFGHIJKLMNO@", "", "41e19058341e9149e592d9743e01", ""},
	{"pound", cbs.GSM7, "A£", "", "c100", ""},
	{"two digits", cbs.GSM7, "14", "", "311a", ""},
	{"two digits 47", cbs.GSM7, "47", "", "b41b", ""},
	{"dial string", cbs.GSM7, "*140*0761241377#", "", "aa180da682dd6c31192d36bbdd46", ""},
	{"prompt", cbs.GSM7, "Enter PIN:", "", "4537bd2c0741934e1d", ""},
	{"price", cbs.GSM7, "Price 5€ [ok]", "", "50797a5c06d53665d086f75e6f7c", ""},
	{"short dial string", cbs.GSM7, "*100#", "", "aa180c3602", ""},
	{"welcome", cbs.GSM7, "Welcome!", "", "d7327bfc6e9743", ""},
	{"digit", cbs.GSM7, "1", "", "31", ""},
	{"russian", cbs.UCS2, "Привет", "", "041f04400438043204350442", ""},
	{"russian euro", cbs.UCS2, "Привет €", "", "041f04400438043204350442002020ac", ""},
	{"ucs2 BMP", cbs.UCS2, "A\uffff", "", "0041ffff", ""},
	{"gsm7 empty", cbs.GSM7, "", "", "", ""},
	{"ucs2 empty", cbs.UCS2, "", "", "", ""},
	// 'e'=65, 'n'=6e, CR=0d, 'A'=41: least-significant bits first.
	{"language gsm7", 0x10, "A", "en", "65772308", ""},
	{"language gsm7 filler", 0x10, "ABCD", "en", "657723281c121b", ""},
	{"language gsm7 empty", 0x10, "", "en", "657703", ""},
	// Two septets: 65 | (6e << 7) = 3765, followed by big-endian UCS2.
	{"language ucs2", 0x11, "A€", "en", "6537004120ac", ""},
	{"language ucs2 uppercase", 0x11, "A", "EN", "45270041", ""},
	{"language ucs2 mixed case", 0x11, "A", "eN", "65270041", ""},
	{"language ucs2 empty", 0x11, "", "en", "6537", ""},
	// Figure 5-1: eng = 00100 01101 00110 0 = 23 4c;
	// deu = 00011 00100 10100 0 = 19 28;
	// zzz = 11001 11001 11001 0 = ce 72; aaa = all zero bits.
	{"letters eng", 0x12, "A€", "eng", "234c004120ac", ""},
	{"letters deu", 0x12, "A", "deu", "19280041", ""},
	{"letters zzz", 0x12, "A", "zzz", "ce720041", ""},
	{"letters aaa", 0x12, "A", "aaa", "00000041", ""},
	{"letters empty", 0x12, "", "eng", "234c", ""},
}

func mustHex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestUSSDVectors(t *testing.T) {
	for _, v := range ussdVectors {
		t.Run(v.name, func(t *testing.T) {
			want := v.text
			if v.decoded != "" {
				want = v.decoded
			}
			b := mustHex(t, v.hex)
			text, lang, err := v.dcs.DecodeUSSD(b)
			if err != nil || text != want || lang != v.lang {
				t.Errorf("DecodeUSSD(%x) = %q, %q, %v; want %q, %q", b, text, lang, err, want, v.lang)
			}
			out, err := v.dcs.EncodeUSSD(v.text, v.lang)
			if err != nil || !bytes.Equal(out, b) {
				t.Fatalf("EncodeUSSD(%q,%q) = %x, %v; want %x", v.text, v.lang, out, err, b)
			}
			back, language, err := v.dcs.DecodeUSSD(out)
			if err != nil || back != want || language != v.lang {
				t.Errorf("round trip = %q,%q,%v; want %q,%q", back, language, err, want, v.lang)
			}
		})
	}
}

func TestUSSDAll256(t *testing.T) {
	for i, ref := range specDCS(t) {
		t.Run(fmt.Sprintf("%02x", i), func(t *testing.T) {
			d := cbs.DCS(i)
			b := mustHex(t, "41e19058341e1b")
			want, lang := "ABCDEFG", ""
			var decodeErr, encodeErr error
			switch {
			case ref.reserved:
				encodeErr = cbs.ErrReserved
			case ref.compressed || ref.charset == cbs.Charset8Bit || ref.charset == cbs.CharsetUnknown || ref.group == cbs.GroupUserDataHeader:
				decodeErr, encodeErr = cbs.ErrUnsupported, cbs.ErrUnsupported
			case ref.indication == cbs.LanguageIndicationGSM7:
				b, want, lang = mustHex(t, "65772308"), "A", "en"
			case ref.indication == cbs.LanguageIndicationUCS2:
				b, want, lang = mustHex(t, "65370041"), "A", "en"
			case ref.indication == cbs.LanguageIndicationUCS2Letters:
				b, want, lang = mustHex(t, "234c0041"), "A", "eng"
			case ref.charset == cbs.CharsetUCS2:
				b, want = mustHex(t, "00410042"), "AB"
			}
			text, language, err := d.DecodeUSSD(b)
			if decodeErr != nil {
				checkWrapped(t, err, decodeErr)
			} else if err != nil || text != want || language != lang {
				t.Errorf("DecodeUSSD = %q,%q,%v; want %q,%q", text, language, err, want, lang)
			}
			out, err := d.EncodeUSSD(want, lang)
			if encodeErr != nil {
				checkWrapped(t, err, encodeErr)
			} else if err != nil || !bytes.Equal(out, b) {
				t.Errorf("EncodeUSSD = %x,%v; want %x", out, err, b)
			}
		})
	}
}

func checkWrapped(t testing.TB, err, target error) {
	t.Helper()
	if !errors.Is(err, target) || err == target {
		t.Errorf("error = %v, want wrapped %v", err, target)
	}
}

func TestUSSDMalformedLanguage(t *testing.T) {
	for _, v := range []struct {
		name string
		dcs  cbs.DCS
		hex  string
	}{
		{"gsm7 nil", 0x10, ""}, {"gsm7 one", 0x10, "65"}, {"gsm7 two", 0x10, "6537"},
		{"gsm7 missing CR", 0x10, "653700"},
		{"gsm7 first escape", 0x10, "1b7703"}, {"gsm7 second escape", 0x10, "e54d03"},
		{"ucs2 nil", 0x11, ""}, {"ucs2 one", 0x11, "65"},
		{"ucs2 first escape", 0x11, "1b320041"}, {"ucs2 second escape", 0x11, "e50d0041"},
		{"letters nil", 0x12, ""}, {"letters one", 0x12, "23"},
		{"letters padding", 0x12, "234d0041"},
		{"letters first reserved", 0x12, "d0000041"},
		{"letters second reserved", 0x12, "06800041"},
		{"letters third reserved", 0x12, "00340041"},
		{"letters all reserved", 0x12, "fffe0041"},
	} {
		t.Run(v.name, func(t *testing.T) {
			text, _, err := v.dcs.DecodeUSSD(mustHex(t, v.hex))
			checkWrapped(t, err, cbs.ErrLanguageIndication)
			if !utf8.ValidString(text) {
				t.Fatal("invalid UTF-8 on error")
			}
		})
	}
}

func TestUSSDLanguageDecodePermissive(t *testing.T) {
	// Section 5's two zero fill bits in 0x11 are ignored on receive.
	// Two-character indicators use the default alphabet, not an ASCII
	// validator: 01 00 = £@, and an escape may not consume a third septet.
	for _, v := range []struct {
		dcs       cbs.DCS
		hex, want string
	}{
		{0x11, "65f70041", "en"}, {0x11, "01000041", "£@"},
		{0x10, "01402308", "£@"},
	} {
		text, lang, err := v.dcs.DecodeUSSD(mustHex(t, v.hex))
		if err != nil || text != "A" || lang != v.want {
			t.Errorf("%02x/%s: %q,%q,%v", v.dcs, v.hex, text, lang, err)
		}
	}
}

func TestUSSDLanguageEncodeErrors(t *testing.T) {
	for _, d := range []cbs.DCS{cbs.GSM7, cbs.UCS2, 0x00, 0x20, 0x50, 0xf0} {
		_, err := d.EncodeUSSD("A", "en")
		checkWrapped(t, err, cbs.ErrLanguageIndication)
	}
	for _, d := range []cbs.DCS{0x10, 0x11} {
		for _, lang := range []string{"", "e", "eng", "e1", " e", "é", "e\r", "e\x1b", "e\xff", "中文"} {
			_, err := d.EncodeUSSD("A", lang)
			checkWrapped(t, err, cbs.ErrLanguageIndication)
		}
	}
	for _, lang := range []string{"", "en", "engl", "Eng", "ENg", "eNg", "enG", "en1", "én", "e\xffg"} {
		_, err := cbs.DCS(0x12).EncodeUSSD("A", lang)
		checkWrapped(t, err, cbs.ErrLanguageIndication)
	}
}

func TestUSSDCodecErrors(t *testing.T) {
	for _, v := range []struct {
		dcs  cbs.DCS
		lang string
	}{{cbs.GSM7, ""}, {cbs.UCS2, ""}, {0x10, "en"}, {0x11, "en"}, {0x12, "eng"}} {
		if _, err := v.dcs.EncodeUSSD("a\xffb", v.lang); err == nil {
			t.Errorf("%02x accepted invalid UTF-8", v.dcs)
		}
		if v.dcs == cbs.GSM7 || v.dcs == 0x10 {
			_, err := v.dcs.EncodeUSSD("ab日", v.lang)
			var unencodable gsm7.ErrUnencodable
			if !errors.As(err, &unencodable) || unencodable.Rune != '日' {
				t.Errorf("gsm7 error = %v", err)
			}
		} else {
			_, err := v.dcs.EncodeUSSD("ab😀", v.lang)
			var unencodable ucs2.ErrUnencodable
			if !errors.As(err, &unencodable) || unencodable.Rune != '😀' {
				t.Errorf("ucs2 error = %v", err)
			}
		}
	}
	for _, v := range []struct {
		dcs    cbs.DCS
		prefix string
	}{{cbs.UCS2, ""}, {0x11, "6537"}, {0x12, "234c"}} {
		_, _, err := v.dcs.DecodeUSSD(mustHex(t, v.prefix+"004100"))
		checkWrapped(t, err, ucs2.ErrInvalidLength)
		_, _, err = v.dcs.DecodeUSSD(mustHex(t, v.prefix+"0041d83d"))
		var dangling ucs2.ErrDanglingSurrogate
		if !errors.As(err, &dangling) {
			t.Errorf("dangling surrogate error = %v", err)
		}
		text, _, err := v.dcs.DecodeUSSD(mustHex(t, v.prefix+"d83dde00"))
		if err != nil || text != "😀" {
			t.Errorf("surrogate pair = %q,%v", text, err)
		}
	}
}

func TestUSSDNoLengthLimitAndDefaultAlphabet(t *testing.T) {
	for _, v := range []struct {
		dcs        cbs.DCS
		text, lang string
		length     int
	}{
		{cbs.GSM7, strings.Repeat("A", 182), "", 160},
		{cbs.GSM7, strings.Repeat("A", 183), "", 161},
		{cbs.GSM7, strings.Repeat("€", 91), "", 160},
		{cbs.GSM7, strings.Repeat("€", 92), "", 161},
		{cbs.GSM7, strings.Repeat("A", 1000), "", 875},
		{cbs.UCS2, strings.Repeat("Ж", 80), "", 160},
		{cbs.UCS2, strings.Repeat("Ж", 81), "", 162},
		{0x10, strings.Repeat("A", 184), "en", 164},
		{0x11, strings.Repeat("Ж", 81), "en", 164},
		{0x12, strings.Repeat("Ж", 81), "eng", 164},
	} {
		out, err := v.dcs.EncodeUSSD(v.text, v.lang)
		if err != nil || len(out) != v.length {
			t.Fatalf("%02x: encoded %d bytes, %v; want %d", v.dcs, len(out), err, v.length)
		}
		text, lang, err := v.dcs.DecodeUSSD(out)
		if err != nil || text != v.text || lang != v.lang {
			t.Errorf("%02x: round trip differs: %v", v.dcs, err)
		}
	}
	// Language labels never select a national-language shift table.
	for _, d := range []cbs.DCS{0x0c, 0x21, 0x22, 0x23} {
		out, err := d.EncodeUSSD("@", "")
		if err != nil || !bytes.Equal(out, []byte{0}) {
			t.Errorf("%02x: @ = %x,%v", d, out, err)
		}
		if _, err := d.EncodeUSSD("Ğ", ""); err == nil {
			t.Errorf("%02x selected a national table", d)
		}
	}
}

func TestUSSDExtensionTable(t *testing.T) {
	// Section 6.2.1.1: every extension takes ESC plus a second septet.
	// One-character messages leave no USSD CR ambiguity.
	for _, v := range []struct{ text, hex string }{
		{"€", "9b32"}, {"[", "1b1e"}, {"]", "1b1f"}, {"{", "1b14"}, {"}", "9b14"},
		{"\\", "9b17"}, {"~", "9b1e"}, {"^", "1b0a"}, {"|", "1b20"}, {"\f", "1b05"},
	} {
		out, err := cbs.GSM7.EncodeUSSD(v.text, "")
		if err != nil || !bytes.Equal(out, mustHex(t, v.hex)) {
			t.Errorf("%q = %x,%v; want %s", v.text, out, err, v.hex)
		}
		text, _, err := cbs.GSM7.DecodeUSSD(mustHex(t, v.hex))
		if err != nil || text != v.text {
			t.Errorf("%s = %q,%v", v.hex, text, err)
		}
	}
	for _, text := range []string{"€[]{}\\~^|", "Balance: 12.50 € (£ ok) {a|b}", "A\rB", "line1\nline2"} {
		out, err := cbs.GSM7.EncodeUSSD(text, "")
		if err != nil {
			t.Fatal(err)
		}
		back, _, err := cbs.GSM7.DecodeUSSD(out)
		if err != nil || back != text {
			t.Errorf("round trip %q = %q,%v", text, back, err)
		}
	}
}
