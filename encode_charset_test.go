// SPDX-License-Identifier: MIT

package sms_test

import (
	"sync"
	"testing"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncoderDefaultCharsetReset(t *testing.T) {
	patterns := []struct {
		name    string
		options []sms.EncoderOption
		dcs     tpdu.DCS
		udh     tpdu.UserDataHeader
	}{
		{"reset template", []sms.EncoderOption{sms.WithDefaultCharset}, tpdu.DcsUCS2Data, nil},
		{"all then reset", []sms.EncoderOption{sms.WithAllCharsets, sms.WithDefaultCharset}, tpdu.DcsUCS2Data, nil},
		{"shift then reset", []sms.EncoderOption{sms.WithDefaultCharset, sms.WithShiftCharset(charset.Turkish), sms.WithDefaultCharset}, tpdu.DcsUCS2Data, nil},
		{"reset then Turkish", []sms.EncoderOption{sms.WithDefaultCharset, sms.WithCharset(charset.Turkish)}, 0, tpdu.UserDataHeader{lockingIE(charset.Turkish)}},
		{"reset then all", []sms.EncoderOption{sms.WithDefaultCharset, sms.WithAllCharsets}, 0, tpdu.UserDataHeader{lockingIE(charset.Turkish)}},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			e := sms.NewEncoder(sms.WithCharset(charset.Turkish))
			out, err := e.Encode([]byte("ı"), p.options...)
			require.NoError(t, err)
			require.Len(t, out, 1)
			assert.Equal(t, p.dcs, out[0].DCS)
			assert.Equal(t, p.udh, out[0].UDH)
			out, err = e.Encode([]byte("ı"))
			require.NoError(t, err)
			require.Len(t, out, 1)
			assert.Equal(t, tpdu.DCS(0), out[0].DCS)
			assert.Equal(t, tpdu.UserDataHeader{lockingIE(charset.Turkish)}, out[0].UDH)
		})
	}
}

// 3GPP TS 23.038 Section 6.2.1.2.5: "the receiving entity shall ignore the
// IE" when the identifier is not supported. The default table has ì at 0x07 (Section
// 6.2.1), while the Turkish locking table has ı (Annex A.3.1).
func TestDecodeDefaultCharsetReset(t *testing.T) {
	segments := []*tpdu.TPDU{{
		UDH: tpdu.UserDataHeader{lockingIE(charset.Turkish)},
		UD:  tpdu.UserData{0x07},
	}}
	patterns := []struct {
		name    string
		options []sms.DecodeOption
		want    string
	}{
		{"implicit all", nil, "ı"},
		{"default only", []sms.DecodeOption{sms.WithDefaultCharset}, "ì"},
		{"all then reset", []sms.DecodeOption{sms.WithAllCharsets, sms.WithDefaultCharset}, "ì"},
		{"Turkish then reset", []sms.DecodeOption{sms.WithCharset(charset.Turkish), sms.WithDefaultCharset}, "ì"},
		{"reset then all", []sms.DecodeOption{sms.WithDefaultCharset, sms.WithAllCharsets}, "ı"},
		{"reset then Turkish", []sms.DecodeOption{sms.WithDefaultCharset, sms.WithCharset(charset.Turkish)}, "ı"},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			msg, err := sms.Decode(segments, p.options...)
			require.NoError(t, err)
			assert.Equal(t, p.want, string(msg))
		})
	}
}

func TestEncoderConcurrentDefaultCharsetReset(t *testing.T) {
	e := sms.NewEncoder(sms.WithCharset(charset.Turkish))
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(reset bool) {
			defer wg.Done()
			var options []sms.EncoderOption
			want := tpdu.DCS(0)
			if reset {
				options = []sms.EncoderOption{sms.WithDefaultCharset}
				want = tpdu.DcsUCS2Data
			}
			for i := 0; i < 200; i++ {
				out, err := e.Encode([]byte("ı"), options...)
				if err != nil || len(out) != 1 || out[0].DCS != want {
					t.Errorf("reset %t: got %v, %v; want DCS %02x", reset, out, err, byte(want))
					return
				}
			}
		}(w%2 == 0)
	}
	wg.Wait()
}

// charsetPatterns are the charset options, each with a message only the
// national language table it is given can code in GSM 7 bit, and the IE
// that table gives it.
var charsetPatterns = []struct {
	name   string
	option func(nli ...int) sms.EncoderOption
	dopt   func(nli ...int) sms.DecodeOption
	nli    int
	msg    string
	ie     tpdu.InformationElement
}{
	{"charset",
		func(nli ...int) sms.EncoderOption { return sms.WithCharset(nli...) },
		func(nli ...int) sms.DecodeOption { return sms.WithCharset(nli...) },
		charset.Urdu, "hello ٻ",
		tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Urdu)}}},
	{"locking charset",
		func(nli ...int) sms.EncoderOption { return sms.WithLockingCharset(nli...) },
		func(nli ...int) sms.DecodeOption { return sms.WithLockingCharset(nli...) },
		charset.Urdu, "hello ٻ",
		tpdu.InformationElement{ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(charset.Urdu)}}},
	{"shift charset",
		func(nli ...int) sms.EncoderOption { return sms.WithShiftCharset(nli...) },
		func(nli ...int) sms.DecodeOption { return sms.WithShiftCharset(nli...) },
		charset.Turkish, "hello ş",
		tpdu.InformationElement{ID: tpdu.IEINationalLanguageSingleShift, Data: []byte{byte(charset.Turkish)}}},
}

// The charset options copy the national language identifiers they are
// given, so a later write by the caller to its slice changes neither an
// option nor an Encoder built with it.
func TestCharsetOptionsCopyIdentifiers(t *testing.T) {
	for _, p := range charsetPatterns {
		f := func(t *testing.T) {
			check := func(t *testing.T, out []tpdu.TPDU, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Len(t, out, 1)
				assert.Equal(t, tpdu.DCS(0x00), out[0].DCS)
				assert.Equal(t, tpdu.UserDataHeader{p.ie}, out[0].UDH)
			}
			nli := []int{p.nli}
			e := sms.NewEncoder(sms.AsSubmit, p.option(nli...))
			option := p.option(nli...)
			dopt := p.dopt(nli...)
			// Spanish has none of the characters, so a message coded with
			// it would be UCS2.
			nli[0] = charset.Spanish
			out, err := e.Encode([]byte(p.msg))
			check(t, out, err)
			out, err = sms.Encode([]byte(p.msg), option)
			check(t, out, err)
			out, err = sms.NewEncoder(sms.AsSubmit).Encode([]byte(p.msg), option)
			check(t, out, err)
			msg, err := sms.Decode([]*tpdu.TPDU{&out[0]}, dopt)
			require.NoError(t, err)
			assert.Equal(t, p.msg, string(msg))
		}
		t.Run(p.name, f)
	}
}

// Writes by the caller to the slice it built an Encoder from do not race
// with the Encoder in use.
func TestEncoderCharsetConcurrentWrite(t *testing.T) {
	for _, p := range charsetPatterns {
		f := func(t *testing.T) {
			nli := []int{p.nli}
			e := sms.NewEncoder(sms.AsSubmit, p.option(nli...))
			var wg sync.WaitGroup
			wrong := make([]int, 4)
			for w := range wrong {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					for i := 0; i < 200; i++ {
						out, err := e.Encode([]byte(p.msg))
						if err != nil || len(out) != 1 || len(out[0].UDH) != 1 || out[0].UDH[0].Data[0] != byte(p.nli) {
							wrong[w]++
						}
					}
				}(w)
			}
			for i := 0; i < 200; i++ {
				nli[0] = i % charset.End
			}
			wg.Wait()
			for w, n := range wrong {
				assert.Zero(t, n, "worker %d", w)
			}
		}
		t.Run(p.name, f)
	}
}
