// SPDX-License-Identifier: MIT

package sms_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Per call options apply to that call only, even when calls are concurrent
// and the Encoder options were built with spare capacity: three charset
// options leave the Encoder options with a length of 3 and a capacity of 4.
func TestEncoderConcurrentOptions(t *testing.T) {
	e := sms.NewEncoder(
		sms.AsSubmit,
		sms.WithCharset(charset.Turkish),
		sms.WithCharset(charset.Portuguese),
		sms.WithCharset(charset.Spanish),
	)
	const workers = 8
	const calls = 500
	var wg sync.WaitGroup
	wrong := make([]int, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < calls; i++ {
				if w%2 == 0 {
					// Urdu has the character, so it is GSM 7 bit.
					out, err := e.Encode([]byte("ت"), sms.WithCharset(charset.Urdu))
					if err != nil || len(out) != 1 || out[0].Alphabet() != tpdu.Alpha7Bit ||
						len(out[0].UDH) != 1 || int(out[0].UDH[0].Data[0]) != charset.Urdu {
						wrong[w]++
					}
				} else {
					// Hindi does not, so it is UCS2.
					out, err := e.Encode([]byte("ت"), sms.WithCharset(charset.Hindi))
					if err != nil || len(out) != 1 || out[0].Alphabet() != tpdu.AlphaUCS2 ||
						out[0].UDH != nil {
						wrong[w]++
					}
				}
			}
		}(w)
	}
	wg.Wait()
	for w, n := range wrong {
		assert.Zero(t, n, "worker %d", w)
	}
}

// Concurrent calls with a template UDH that has spare capacity, and with
// per call templates, neither race nor mix their results.
func TestEncoderConcurrentTemplate(t *testing.T) {
	udh := make(tpdu.UserDataHeader, 1, 4)
	udh[0] = portIE
	e := sms.NewEncoder(sms.AsSubmit, sms.To("1234"), sms.WithTemplateOption(tpdu.WithUDH(udh)))
	long := []byte(strings.Repeat("a", 300))
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				var options []sms.EncoderOption
				if w%2 == 1 {
					options = append(options, sms.WithCharset(charset.Urdu))
				}
				msg := long
				if w%4 >= 2 {
					msg = []byte("hello ت")
				}
				out, err := e.Encode(msg, options...)
				if !assert.NoError(t, err) {
					return
				}
				for _, p := range out {
					if !assert.Equal(t, portIE, p.UDH[0]) {
						return
					}
					// Changing a result does not change the Encoder.
					p.UDH[0].Data[0] = byte(w)
					p.UDH[0] = tpdu.InformationElement{ID: byte(w)}
				}
			}
		}(w)
	}
	wg.Wait()
	assert.Equal(t, portIE, udh[0])
}

// The TPDUs returned share no memory with each other, the Encoder, its
// template or the message, so changing one changes nothing else.
func TestEncodeResultsAreIndependent(t *testing.T) {
	t.Run("segments", func(t *testing.T) {
		out, err := sms.Encode([]byte(strings.Repeat("a", 200)))
		require.NoError(t, err)
		require.Len(t, out, 2)
		second := append([]byte(nil), out[1].UD...)
		out[0].UD = append(out[0].UD, 'x')
		out[0].UDH[0].Data[0]++
		assert.Equal(t, second, []byte(out[1].UD))
		ci0, _ := out[0].ConcatInfo()
		ci1, _ := out[1].ConcatInfo()
		assert.NotEqual(t, ci0.Ref, ci1.Ref)
	})
	t.Run("message", func(t *testing.T) {
		for _, alpha := range []sms.EncoderOption{sms.As8Bit, sms.AsUCS2} {
			msg := []byte("abcd")
			out, err := sms.Encode(msg, alpha)
			require.NoError(t, err)
			require.Len(t, out, 1)
			msg[0] = 'X'
			assert.Equal(t, "abcd", string(out[0].UD))
			out[0].UD[1] = 'Y'
			assert.Equal(t, "Xbcd", string(msg))
		}
	})
	t.Run("template", func(t *testing.T) {
		udh := tpdu.UserDataHeader{{ID: 0x05, Data: []byte{0x0b, 0x84, 0x23, 0xf0}}}
		tmpl := tpdu.TPDU{
			Direction:  tpdu.MO,
			FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
			DA:         tpdu.Address{TOA: 0x91, Addr: "1234"},
			UDH:        udh,
		}
		e := sms.NewEncoder(sms.WithTemplate(tmpl))
		// changing the caller's template after NewEncoder does not change
		// the Encoder
		udh[0].Data[0] = 0xff
		out, err := e.Encode([]byte("hello"))
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, portIE, out[0].UDH[0])
		// nor does changing a result
		out[0].UDH[0].Data[1] = 0xff
		out, err = e.Encode([]byte("hello"))
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, portIE, out[0].UDH[0])
	})
	t.Run("per call template", func(t *testing.T) {
		udh := tpdu.UserDataHeader{{ID: 0x05, Data: []byte{0x0b, 0x84, 0x23, 0xf0}}}
		e := sms.NewEncoder(sms.AsSubmit)
		out, err := e.Encode([]byte("hello"), sms.WithTemplateOption(tpdu.WithUDH(udh)))
		require.NoError(t, err)
		require.Len(t, out, 1)
		out[0].UDH[0].Data[0] = 0
		assert.True(t, bytes.Equal(portIE.Data, udh[0].Data))
	})
}
