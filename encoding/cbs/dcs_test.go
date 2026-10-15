// SPDX-License-Identifier: MIT

package cbs_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gomaja/go-sms/encoding/cbs"
)

type classification struct {
	group                cbs.Group
	charset              cbs.Charset
	reserved, compressed bool
	class                cbs.MessageClass
	language             string
	indication           cbs.LanguageIndication
}

// specDCS transcribes every row of TS 23.038 V20.0.0 Section 5. Ranges,
// class patterns, and language lists are literal expectations, independent
// of the implementation's bit masks. '-' means "No message class".
// The gsmmap USSD classification reference supplies the same coding ranges;
// Section 5's reserved rule also covers 6c..6f, 7c..7f and 9c..9f.
func specDCS(t testing.TB) [256]classification {
	t.Helper()
	rows := []struct {
		first, last          int
		group                cbs.Group
		charset              cbs.Charset
		reserved, compressed bool
		classes, languages   string
		indication           cbs.LanguageIndication
	}{
		{0x00, 0x0e, cbs.GroupLanguage, cbs.CharsetGSM7, false, false, "-", "de en it fr es nl sv da pt fi no el tr hu pl", cbs.NoLanguageIndication},
		{0x0f, 0x0f, cbs.GroupLanguage, cbs.CharsetGSM7, false, false, "-", "", cbs.NoLanguageIndication},
		{0x10, 0x10, cbs.GroupLanguageIndication, cbs.CharsetGSM7, false, false, "-", "", cbs.LanguageIndicationGSM7},
		{0x11, 0x11, cbs.GroupLanguageIndication, cbs.CharsetUCS2, false, false, "-", "", cbs.LanguageIndicationUCS2},
		{0x12, 0x12, cbs.GroupLanguageIndication, cbs.CharsetUCS2, false, false, "-", "", cbs.LanguageIndicationUCS2Letters},
		{0x13, 0x1f, cbs.GroupLanguageIndication, cbs.CharsetGSM7, true, false, "-", "", cbs.NoLanguageIndication},
		{0x20, 0x24, cbs.GroupLanguage2, cbs.CharsetGSM7, false, false, "-", "cs he ar ru is", cbs.NoLanguageIndication},
		{0x25, 0x2f, cbs.GroupLanguage2, cbs.CharsetGSM7, true, false, "-", "", cbs.NoLanguageIndication},
		{0x30, 0x3f, cbs.GroupLanguage3, cbs.CharsetGSM7, true, false, "-", "", cbs.NoLanguageIndication},
		{0x40, 0x43, cbs.GroupGeneralDataCoding, cbs.CharsetGSM7, false, false, "-", "", cbs.NoLanguageIndication},
		{0x44, 0x47, cbs.GroupGeneralDataCoding, cbs.Charset8Bit, false, false, "-", "", cbs.NoLanguageIndication},
		{0x48, 0x4b, cbs.GroupGeneralDataCoding, cbs.CharsetUCS2, false, false, "-", "", cbs.NoLanguageIndication},
		{0x4c, 0x4f, cbs.GroupGeneralDataCoding, cbs.CharsetGSM7, true, false, "-", "", cbs.NoLanguageIndication},
		{0x50, 0x53, cbs.GroupGeneralDataCoding, cbs.CharsetGSM7, false, false, "0123", "", cbs.NoLanguageIndication},
		{0x54, 0x57, cbs.GroupGeneralDataCoding, cbs.Charset8Bit, false, false, "0123", "", cbs.NoLanguageIndication},
		{0x58, 0x5b, cbs.GroupGeneralDataCoding, cbs.CharsetUCS2, false, false, "0123", "", cbs.NoLanguageIndication},
		{0x5c, 0x5f, cbs.GroupGeneralDataCoding, cbs.CharsetGSM7, true, false, "-", "", cbs.NoLanguageIndication},
		{0x60, 0x63, cbs.GroupGeneralDataCoding, cbs.CharsetGSM7, false, true, "-", "", cbs.NoLanguageIndication},
		{0x64, 0x67, cbs.GroupGeneralDataCoding, cbs.Charset8Bit, false, true, "-", "", cbs.NoLanguageIndication},
		{0x68, 0x6b, cbs.GroupGeneralDataCoding, cbs.CharsetUCS2, false, true, "-", "", cbs.NoLanguageIndication},
		{0x6c, 0x6f, cbs.GroupGeneralDataCoding, cbs.CharsetGSM7, true, false, "-", "", cbs.NoLanguageIndication},
		{0x70, 0x73, cbs.GroupGeneralDataCoding, cbs.CharsetGSM7, false, true, "0123", "", cbs.NoLanguageIndication},
		{0x74, 0x77, cbs.GroupGeneralDataCoding, cbs.Charset8Bit, false, true, "0123", "", cbs.NoLanguageIndication},
		{0x78, 0x7b, cbs.GroupGeneralDataCoding, cbs.CharsetUCS2, false, true, "0123", "", cbs.NoLanguageIndication},
		{0x7c, 0x7f, cbs.GroupGeneralDataCoding, cbs.CharsetGSM7, true, false, "-", "", cbs.NoLanguageIndication},
		{0x80, 0x8f, cbs.GroupReserved, cbs.CharsetGSM7, true, false, "-", "", cbs.NoLanguageIndication},
		{0x90, 0x93, cbs.GroupUserDataHeader, cbs.CharsetGSM7, false, false, "0123", "", cbs.NoLanguageIndication},
		{0x94, 0x97, cbs.GroupUserDataHeader, cbs.Charset8Bit, false, false, "0123", "", cbs.NoLanguageIndication},
		{0x98, 0x9b, cbs.GroupUserDataHeader, cbs.CharsetUCS2, false, false, "0123", "", cbs.NoLanguageIndication},
		{0x9c, 0x9f, cbs.GroupUserDataHeader, cbs.CharsetGSM7, true, false, "-", "", cbs.NoLanguageIndication},
		{0xa0, 0xcf, cbs.GroupReserved, cbs.CharsetGSM7, true, false, "-", "", cbs.NoLanguageIndication},
		{0xd0, 0xdf, cbs.GroupI1Protocol, cbs.CharsetUnknown, false, false, "-", "", cbs.NoLanguageIndication},
		{0xe0, 0xef, cbs.GroupWAP, cbs.CharsetUnknown, false, false, "-", "", cbs.NoLanguageIndication},
		{0xf0, 0xf3, cbs.GroupDataCoding, cbs.CharsetGSM7, false, false, "-123", "", cbs.NoLanguageIndication},
		{0xf4, 0xf7, cbs.GroupDataCoding, cbs.Charset8Bit, false, false, "-123", "", cbs.NoLanguageIndication},
		{0xf8, 0xff, cbs.GroupDataCoding, cbs.CharsetGSM7, true, false, "-", "", cbs.NoLanguageIndication},
	}
	var refs [256]classification
	var seen [256]bool
	classes := map[byte]cbs.MessageClass{'-': cbs.NoClass, '0': cbs.Class0, '1': cbs.Class1, '2': cbs.Class2, '3': cbs.Class3}
	for _, row := range rows {
		langs := strings.Fields(row.languages)
		for i := row.first; i <= row.last; i++ {
			if seen[i] {
				t.Fatalf("duplicate reference for %02x", i)
			}
			seen[i] = true
			lang := ""
			if len(langs) != 0 {
				lang = langs[i-row.first]
			}
			refs[i] = classification{row.group, row.charset, row.reserved, row.compressed, classes[row.classes[(i-row.first)%len(row.classes)]], lang, row.indication}
		}
	}
	for i, ok := range seen {
		if !ok {
			t.Fatalf("no reference for %02x", i)
		}
	}
	return refs
}

func TestDCSAll256(t *testing.T) {
	for i, want := range specDCS(t) {
		t.Run(fmt.Sprintf("%02x", i), func(t *testing.T) {
			d := cbs.DCS(i)
			lang, ok := d.Language()
			got := classification{d.Group(), d.Charset(), d.Reserved(), d.Compressed(), d.Class(), lang, d.LanguageIndication()}
			if got != want || ok != (want.language != "") {
				t.Errorf("got %+v (language present %v), want %+v", got, ok, want)
			}
		})
	}
}
