// SPDX-License-Identifier: MIT

package sms_test

import (
	"testing"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// submitSegment returns segment seqno of total of an SMS-SUBMIT to da, with
// the 8-bit reference ref.
func submitSegment(t *testing.T, da string, ref, total, seqno byte, ud string) *tpdu.TPDU {
	t.Helper()
	p := tpdu.TPDU{}
	require.NoError(t, p.SetSmsType(tpdu.SmsSubmit))
	p.DA = tpdu.Address{Addr: da, TOA: 0x91}
	if total > 0 {
		p.SetUDH(tpdu.UserDataHeader{{ID: 0, Data: []byte{ref, total, seqno}}})
	}
	p.UD = []byte(ud)
	return &p
}

// deliverSegment returns segment seqno of total of an SMS-DELIVER from oa,
// with the 8-bit reference ref.
func deliverSegment(oa string, ref, total, seqno byte, ud string) *tpdu.TPDU {
	p := tpdu.TPDU{OA: tpdu.Address{Addr: oa, TOA: 0x91}, UD: []byte(ud)}
	p.SetUDH(tpdu.UserDataHeader{{ID: 0, Data: []byte{ref, total, seqno}}})
	return &p
}

func decoded(t *testing.T, segs []*tpdu.TPDU) string {
	t.Helper()
	require.True(t, sms.IsCompleteMessage(segs))
	msg, err := sms.Decode(segs)
	require.NoError(t, err)
	return string(msg)
}

// The originator of an SMS-SUBMIT is not in the TPDU, so it is given by the
// caller, and segments from different originators are never reassembled
// together, as 3GPP TS 23.040 Section 9.2.3.24.1 requires.
func TestCollectorSubmitOriginator(t *testing.T) {
	c := sms.NewCollector()
	defer c.Close()
	alice := sms.WithOriginator("+15550001")
	bob := sms.WithOriginator("+15550002")

	out, err := c.Collect(submitSegment(t, "3000", 7, 2, 1, "alice part 1 "), alice)
	require.NoError(t, err)
	assert.Nil(t, out)
	out, err = c.Collect(submitSegment(t, "3000", 7, 2, 2, "bob part 2"), bob)
	require.NoError(t, err)
	assert.Nil(t, out)
	// the same originator to another destination
	out, err = c.Collect(submitSegment(t, "4000", 7, 2, 2, "alice to 4000 part 2"), alice)
	require.NoError(t, err)
	assert.Nil(t, out)
	pipes := c.Pipes()
	require.Len(t, pipes, 3)
	assert.Equal(t, "+15550001", pipes[0].Originator)
	assert.Equal(t, "+15550002", pipes[1].Originator)
	assert.Equal(t, "+15550001", pipes[2].Originator)
	for _, p := range pipes {
		assert.Equal(t, tpdu.SmsSubmit, p.SmsType)
		assert.Equal(t, 7, p.Ref)
		assert.False(t, p.Ref16Bit)
		assert.Equal(t, 2, p.Total)
	}
	assert.Equal(t, "3000", pipes[0].Address.Addr)
	assert.Equal(t, "4000", pipes[2].Address.Addr)

	out, err = c.Collect(submitSegment(t, "3000", 7, 2, 2, "alice part 2"), alice)
	require.NoError(t, err)
	assert.Equal(t, "alice part 1 alice part 2", decoded(t, out))
	out, err = c.Collect(submitSegment(t, "3000", 7, 2, 1, "bob part 1 "), bob)
	require.NoError(t, err)
	assert.Equal(t, "bob part 1 bob part 2", decoded(t, out))
	require.Len(t, c.Pipes(), 1)
}

// An SMS-SUBMIT without an originator is rejected, whether it is a segment
// or not, and nothing is stored.
func TestCollectorSubmitWithoutOriginator(t *testing.T) {
	c := sms.NewCollector()
	defer c.Close()
	for _, p := range []*tpdu.TPDU{
		submitSegment(t, "3000", 7, 2, 1, "part 1"),
		submitSegment(t, "3000", 0, 0, 0, "single"),
	} {
		for _, options := range [][]sms.CollectOption{nil, {sms.WithOriginator("")}} {
			out, err := c.Collect(p, options...)
			assert.Equal(t, sms.ErrMissingOriginator, err)
			assert.Nil(t, out)
			assert.Empty(t, c.Pipes())
		}
	}
	out, err := c.Collect(submitSegment(t, "3000", 0, 0, 0, "single"), sms.WithOriginator("+15550001"))
	require.NoError(t, err)
	assert.Equal(t, "single", decoded(t, out))
}

// The TP-OA of an SMS-DELIVER identifies its originator. An originator given
// by the caller, such as the address of the SC, keeps apart segments that
// the TP-OA alone would not.
func TestCollectorDeliverOriginator(t *testing.T) {
	c := sms.NewCollector()
	defer c.Close()
	sc1 := sms.WithOriginator("+61412290191")
	sc2 := sms.WithOriginator("+61412290192")
	type step struct {
		pdu     *tpdu.TPDU
		options []sms.CollectOption
		out     string
	}
	for _, s := range []step{
		{deliverSegment("1234", 7, 2, 1, "a1 "), nil, ""},
		{deliverSegment("1234", 7, 2, 2, "b2"), []sms.CollectOption{sc1}, ""},
		{deliverSegment("1234", 7, 2, 1, "c1 "), []sms.CollectOption{sc2}, ""},
		{deliverSegment("5678", 7, 2, 2, "d2"), nil, ""},
		{deliverSegment("1234", 7, 2, 2, "a2"), nil, "a1 a2"},
		{deliverSegment("1234", 7, 2, 1, "b1 "), []sms.CollectOption{sc1}, "b1 b2"},
		{deliverSegment("1234", 7, 2, 2, "c2"), []sms.CollectOption{sc2}, "c1 c2"},
	} {
		out, err := c.Collect(s.pdu, s.options...)
		require.NoError(t, err)
		if s.out == "" {
			assert.Nil(t, out)
		} else {
			assert.Equal(t, s.out, decoded(t, out))
		}
	}
	pipes := c.Pipes()
	require.Len(t, pipes, 1)
	assert.Equal(t, sms.Pipe{
		SmsType:  tpdu.SmsDeliver,
		Address:  tpdu.Address{Addr: "5678", TOA: 0x91},
		Ref:      7,
		Total:    2,
		Segments: []*tpdu.TPDU{nil, deliverSegment("5678", 7, 2, 2, "d2")},
	}, pipes[0])
}

// Collect copies the TPDU, so the caller may change and reuse it, and the
// TPDUs it returns are the caller's.
func TestCollectorCopiesTPDU(t *testing.T) {
	c := sms.NewCollector()
	defer c.Close()
	p := deliverSegment("1234", 7, 2, 1, "abc")
	out, err := c.Collect(p)
	require.NoError(t, err)
	require.Nil(t, out)
	// reuse the TPDU for the next segment, changing its slices in place
	p.UD[0] = 'X'
	p.UDH[0].Data[2] = 2
	p.OA.Addr = "1234"
	out, err = c.Collect(p)
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "abcXbc", decoded(t, out))
	assert.NotSame(t, p, out[1])

	// a single segment too
	s := &tpdu.TPDU{OA: tpdu.Address{Addr: "1234", TOA: 0x91}, UD: []byte("hi")}
	out, err = c.Collect(s)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.NotSame(t, s, out[0])
	s.UD[0] = 'X'
	assert.Equal(t, "hi", decoded(t, out))

	// and a reused TPDU unmarshalled into
	var u tpdu.TPDU
	for i, b := range [][]byte{
		{0x44, 0x04, 0x91, 0x21, 0x43, 0x00, 0x04, 0x62, 0x80, 0x92, 0x10, 0x00, 0x00, 0x00,
			0x08, 0x05, 0x00, 0x03, 0x09, 0x02, 0x01, 'a', 'b'},
		{0x44, 0x04, 0x91, 0x21, 0x43, 0x00, 0x04, 0x62, 0x80, 0x92, 0x10, 0x00, 0x00, 0x00,
			0x08, 0x05, 0x00, 0x03, 0x09, 0x02, 0x02, 'c', 'd'},
	} {
		require.NoError(t, u.UnmarshalBinary(b))
		out, err = c.Collect(&u)
		require.NoError(t, err)
		if i == 0 {
			require.Nil(t, out)
		}
	}
	assert.Equal(t, "abcd", decoded(t, out))
}

func TestCollectorNilTPDU(t *testing.T) {
	c := sms.NewCollector()
	defer c.Close()
	out, err := c.Collect(nil)
	assert.Equal(t, sms.ErrMissingSegment, err)
	assert.Nil(t, out)
}

// Pipes lists the pipes in the order they were created.
func TestCollectorPipesOrder(t *testing.T) {
	c := sms.NewCollector()
	defer c.Close()
	refs := []byte{50, 3, 250, 9, 1, 77, 120, 33, 5, 200, 64, 17}
	for _, ref := range refs {
		_, err := c.Collect(deliverSegment("1234", ref, 3, 2, "x"))
		require.NoError(t, err)
	}
	pipes := c.Pipes()
	require.Len(t, pipes, len(refs))
	for i, p := range pipes {
		assert.Equal(t, int(refs[i]), p.Ref)
	}
}
