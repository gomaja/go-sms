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
