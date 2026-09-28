// SPDX-License-Identifier: MIT

package sms_test

import (
	"sync"
	"testing"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With16BitConcatRef makes the Encoder use the Concatenated short messages,
// 16-bit reference number IE of 3GPP TS 23.040 Section 9.2.3.24.8.
func TestEncodeWith16BitConcatRef(t *testing.T) {
	out, err := sms.Encode(longMsg, sms.With16BitConcatRef, sms.WithConcatRef(sms.NewCounter(0x11233)))
	require.NoError(t, err)
	require.Len(t, out, 2)
	for i, p := range out {
		assert.Equal(t, tpdu.UserDataHeader{
			{ID: tpdu.IEIConcat16Bit, Data: []byte{0x12, 0x34, 2, byte(i + 1)}},
		}, p.UDH)
		ci, ok := p.ConcatInfo()
		assert.True(t, ok)
		assert.Equal(t, tpdu.ConcatInfo{Ref: 0x1234, Ref16Bit: true, Total: 2, Seqno: i + 1}, ci)
	}
	msg, err := sms.Decode(receive(t, out))
	require.NoError(t, err)
	assert.Equal(t, longMsg, msg)

	// per call, it applies to that call only
	e := sms.NewEncoder(sms.AsSubmit, sms.WithConcatRef(sms.NewCounter(0x1ff)))
	out, err = e.Encode(longMsg, sms.With16BitConcatRef)
	require.NoError(t, err)
	assert.Equal(t, tpdu.IEIConcat16Bit, out[0].UDH[0].ID)
	assert.Equal(t, []byte{0x02, 0x00, 2, 1}, out[0].UDH[0].Data)
	out, err = e.Encode(longMsg)
	require.NoError(t, err)
	assert.Equal(t, tpdu.IEIConcat8Bit, out[0].UDH[0].ID)
	assert.Equal(t, []byte{0x01, 2, 1}, out[0].UDH[0].Data)

	// a single segment has no concatenation IE
	out, err = sms.Encode([]byte("hi"), sms.With16BitConcatRef)
	require.NoError(t, err)
	assert.Nil(t, out[0].UDH)
}

// Concurrent calls with their own counters draw from their own counters,
// even when the segmentation options of the Encoder have spare capacity:
// five options leave them with a length of 5 and a capacity of 8, room for
// the counters of each call.
func TestEncoderConcurrentCounters(t *testing.T) {
	e := sms.NewEncoder(sms.AsSubmit,
		sms.With16BitConcatRef, sms.With16BitConcatRef, sms.With16BitConcatRef,
		sms.With16BitConcatRef, sms.With16BitConcatRef)
	const workers = 8
	const calls = 200
	counters := make([]*sms.Counter, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		counters[w] = &sms.Counter{}
		wg.Add(1)
		go func(c *sms.Counter) {
			defer wg.Done()
			for i := 0; i < calls; i++ {
				out, err := e.Encode(longMsg, sms.WithMR(c))
				if !assert.NoError(t, err) || !assert.Len(t, out, 2) {
					return
				}
				assert.Equal(t, byte(2*i+1), out[0].MR)
				assert.Equal(t, byte(2*i+2), out[1].MR)
				assert.Equal(t, tpdu.IEIConcat16Bit, out[0].UDH[0].ID)
			}
		}(counters[w])
	}
	wg.Wait()
	for w, c := range counters {
		assert.Equal(t, 2*calls, c.Read(), "worker %d", w)
	}
}
