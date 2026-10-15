// SPDX-License-Identifier: MIT

package cbs_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gomaja/go-sms/encoding/cbs"
	"github.com/gomaja/go-sms/encoding/gsm7"
)

func FuzzDCS(f *testing.F) {
	ref := specDCS(f)
	for i := 0; i < 256; i++ {
		f.Add(byte(i))
	}
	f.Fuzz(func(t *testing.T, value byte) {
		d := cbs.DCS(value)
		lang, ok := d.Language()
		got := classification{d.Group(), d.Charset(), d.Reserved(), d.Compressed(), d.Class(), lang, d.LanguageIndication()}
		if got != ref[value] || ok != (ref[value].language != "") {
			t.Fatalf("DCS %02x: %+v, want %+v", value, got, ref[value])
		}
		if d.Reserved() && (d.Charset() != cbs.CharsetGSM7 || d.Class() != cbs.NoClass || d.Compressed() || ok || d.LanguageIndication() != cbs.NoLanguageIndication) {
			t.Fatalf("reserved DCS %02x has active coding fields", value)
		}
		if d.LanguageIndication() != cbs.NoLanguageIndication && d.Group() != cbs.GroupLanguageIndication {
			t.Fatalf("language indication outside group 0001: %02x", value)
		}
	})
}

func FuzzDecodeUSSD(f *testing.F) {
	for _, v := range ussdVectors {
		f.Add(byte(v.dcs), mustHex(f, v.hex))
	}
	// Codec seeds from gsmmap's ussd_fuzz_test.go, plus language prefixes
	// and all 256 coding values. MAP envelopes are outside this package.
	for _, v := range []struct {
		dcs byte
		hex string
	}{
		{0x48, "d83dde00"}, {0x48, "d83d"}, {0x48, "004100"},
		{0x00, "31"}, {0x13, "31"}, {0x10, "31"}, {0x44, "31"},
		{0x10, "657700"}, {0x10, "1b7703"}, {0x11, "65f70041"},
		{0x11, "1b32"}, {0x12, "234d0041"}, {0x12, "d000"},
		// A noncanonical escape can shorten the decoded text enough that
		// re-encoding adds CR under Section 6.1.2.3.1.
		{0x0f, "9ba070482c1a8f0d"},
	} {
		f.Add(v.dcs, mustHex(f, v.hex))
	}
	for i := 0; i < 256; i++ {
		f.Add(byte(i), []byte{0x31})
	}
	f.Fuzz(func(t *testing.T, value byte, b []byte) {
		d := cbs.DCS(value)
		text, lang, err := d.DecodeUSSD(b)
		if !utf8.ValidString(text) || !utf8.ValidString(lang) {
			t.Fatalf("DCS %02x returned invalid UTF-8", value)
		}
		if err != nil || d.Reserved() {
			return
		}
		out, err := d.EncodeUSSD(text, lang)
		if err != nil {
			return
		}
		back, language, err := d.DecodeUSSD(out)
		if err != nil || language != lang {
			t.Fatalf("DCS %02x round trip language/error: %q, %v; want %q", value, language, err, lang)
		}
		if back == text {
			return
		}
		// Section 6.1.2.3.1 requires a second CR after a final CR on an
		// octet boundary; the receiver performs both carriage returns.
		// Only that exact change is permitted by the reference codec.
		if d.Charset() == cbs.CharsetGSM7 && strings.HasSuffix(text, "\r") && back == text+"\r" {
			whole := text
			if d.LanguageIndication() == cbs.LanguageIndicationGSM7 {
				whole = lang + "\r" + text
			}
			septets, encodeErr := gsm7.Encode([]byte(whole))
			if encodeErr == nil && len(septets)%8 == 0 {
				return
			}
		}
		t.Fatalf("DCS %02x round trip = %q, want %q (in=%x out=%x)", value, back, text, b, out)
	})
}
