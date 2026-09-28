// SPDX-License-Identifier: MIT

package sms_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCollector(t *testing.T) {
	c := sms.NewCollector()
	assert.NotNil(t, c)
}

func TestCollectorClose(t *testing.T) {
	c := sms.NewCollector(sms.WithReassemblyTimeout(time.Minute, nil))
	require.NotNil(t, c)
	c.Close() // when open
	c.Close() // when closed
	c = sms.NewCollector(sms.WithReassemblyTimeout(time.Minute, nil))
	require.NotNil(t, c)
	d := tpdu.TPDU{}
	d.OA = tpdu.Address{Addr: "1234", TOA: 0x91}
	d.SetUDH(tpdu.UserDataHeader{tpdu.InformationElement{ID: 0, Data: []byte{5, 2, 1}}})
	_, err := c.Collect(&d)
	require.NoError(t, err)
	c.Close() // with pipe active
}

// collectOptions returns the options to collect the TPDU with, which are the
// originator an SMS-SUBMIT requires.
func collectOptions(p *tpdu.TPDU) []sms.CollectOption {
	if p.SmsType() == tpdu.SmsSubmit {
		return []sms.CollectOption{sms.WithOriginator("+15550001")}
	}
	return nil
}

func TestCollectorCollect(t *testing.T) {
	patterns := []struct {
		name string
		in   tpdu.TPDU
		out  []*tpdu.TPDU
		err  error
	}{
		// The patterns are Collected sequentially, so the return value depends
		// on the preceding set of PDUs, not just the individual in. The
		// resulting tests must be run as a complete set.
		{
			"deliver single segment",
			tpdu.TPDU{OA: tpdu.Address{Addr: "1234", TOA: 0x91}},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				},
			},
			nil,
		},
		// 1 segment (shouldn't be seen in practice, but test in case)
		{
			"deliver one segment",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{1, 1, 1}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{1, 1, 1}},
					},
				},
			},
			nil,
		},
		{
			"deliver two a",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{2, 2, 1}},
				},
			},
			nil,
			nil,
		},
		{
			"deliver two b",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{2, 2, 2}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{2, 2, 1}},
					},
				},
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{2, 2, 2}},
					},
				},
			},
			nil,
		},
		{
			"deliver three a",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{3, 3, 1}},
				},
			},
			nil,
			nil,
		},
		{
			"deliver three b",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{3, 3, 2}},
				},
			},
			nil,
			nil,
		},
		{
			"deliver three c",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{3, 3, 3}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 3, 1}},
					},
				},
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 3, 2}},
					},
				},
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{3, 3, 3}},
					},
				},
			},
			nil,
		},
		{
			"jumbled a",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{4, 3, 1}},
				},
			},
			nil,
			nil,
		},
		{
			"jumbled c",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{4, 3, 3}},
				},
			},
			nil,
			nil,
		},
		{
			"duplicate",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{4, 3, 3}},
				},
			},
			nil,
			sms.ErrDuplicateSegment,
		},
		{
			"jumbled b",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{4, 3, 2}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{4, 3, 1}},
					},
				},
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{4, 3, 2}},
					},
				},
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{4, 3, 3}},
					},
				},
			},
			nil,
		},
		{
			"concurrent one a",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{5, 2, 1}},
				},
			},
			nil,
			nil,
		},
		{
			"concurrent two a",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{6, 2, 1}},
				},
			},
			nil,
			nil,
		},
		{
			"concurrent one b",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{5, 2, 2}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{5, 2, 1}},
					},
				},
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{5, 2, 2}},
					},
				},
			},
			nil,
		},
		{
			"concurrent two b",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{6, 2, 2}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{6, 2, 1}},
					},
				},
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{6, 2, 2}},
					},
				},
			},
			nil,
		},
		{
			"deliver 16bit concat a",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 8, Data: []byte{4, 4, 2, 1}},
				},
			},
			nil,
			nil,
		},
		{
			"deliver 16bit concat b",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 8, Data: []byte{4, 4, 2, 2}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 8, Data: []byte{4, 4, 2, 1}},
					},
				},
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 8, Data: []byte{4, 4, 2, 2}},
					},
				},
			},
			nil,
		},
		{
			"submit two a",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				DA:         tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{2, 2, 1}},
				},
			},
			nil,
			nil,
		},
		{
			"submit two b",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				DA:         tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{2, 2, 2}},
				},
			},
			[]*tpdu.TPDU{
				{
					Direction:  tpdu.MO,
					FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
					DA:         tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{2, 2, 1}},
					},
				},
				{
					Direction:  tpdu.MO,
					FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
					DA:         tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{2, 2, 2}},
					},
				},
			},
			nil,
		},
		// A concatenation IE with an invalid sequence number is ignored (3GPP
		// TS 23.040 Section 9.2.3.24.1), so the SM is delivered on its own.
		{
			"zero seqno",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{7, 2, 0}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{7, 2, 0}},
					},
				},
			},
			nil,
		},
		{
			"large seqno",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{8, 2, 3}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{8, 2, 3}},
					},
				},
			},
			nil,
		},
		{
			"zero total",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 8, Data: []byte{0, 8, 0, 1}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 8, Data: []byte{0, 8, 0, 1}},
					},
				},
			},
			nil,
		},
		// An 8-bit reference 9 and a 16-bit reference 9 identify different
		// concatenated messages.
		{
			"deliver 8bit ref a",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{9, 2, 1}},
				},
			},
			nil,
			nil,
		},
		{
			"deliver 16bit ref b",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 8, Data: []byte{0, 9, 2, 2}},
				},
			},
			nil,
			nil,
		},
		{
			"deliver 8bit ref b",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{9, 2, 2}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{9, 2, 1}},
					},
				},
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{9, 2, 2}},
					},
				},
			},
			nil,
		},
		{
			"deliver 16bit ref a",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 8, Data: []byte{0, 9, 2, 1}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 8, Data: []byte{0, 9, 2, 1}},
					},
				},
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 8, Data: []byte{0, 9, 2, 2}},
					},
				},
			},
			nil,
		},
		{
			"deliverreport concat",
			tpdu.TPDU{
				Direction: tpdu.MO,
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{8, 2, 1}},
				},
			},
			nil,
			tpdu.ErrUnsupportedSmsType(tpdu.SmsDeliverReport),
		},
	}
	var ae []*tpdu.TPDU
	exph := func(pp []*tpdu.TPDU) {
		ae = pp
	}
	c := sms.NewCollector(sms.WithReassemblyTimeout(time.Minute, exph))
	require.NotNil(t, c)
	for _, p := range patterns {
		f := func(t *testing.T) {
			ae = nil
			out, err := c.Collect(&p.in, collectOptions(&p.in)...)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, out)
			assert.Nil(t, ae)
		}
		t.Run(p.name, f)
	}
	c.Close()
	patterns = []struct {
		name string
		in   tpdu.TPDU
		out  []*tpdu.TPDU
		err  error
	}{
		{
			"closed single segment",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
			},
			nil,
			sms.ErrClosed,
		},
		{
			"closed concat",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{4, 3, 1}},
				},
			},
			nil,
			sms.ErrClosed,
		},
	}
	for _, p := range patterns {
		ae = nil
		out, err := c.Collect(&p.in, collectOptions(&p.in)...)
		assert.Equal(t, p.err, err, p.name)
		assert.Equal(t, p.out, out, p.name)
		assert.Nil(t, ae, p.name)
	}
}

func TestCollectorReassemblyTimeout(t *testing.T) {
	patterns := []struct {
		name string
		in   []tpdu.TPDU
	}{
		{
			"one",
			[]tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 1}},
					},
				},
			},
		},
		{
			"two",
			[]tpdu.TPDU{
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{1, 3, 2}},
					},
				},
				{
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{1, 3, 1}},
					},
				},
			},
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			var done = make(chan struct{})
			var aechan = make(chan []*tpdu.TPDU)
			exph := func(tt []*tpdu.TPDU) {
				close(done)
				aechan <- tt
			}
			c := sms.NewCollector(sms.WithReassemblyTimeout(time.Millisecond, exph))
			pexp := make([]*tpdu.TPDU, len(p.in)+1)
			for i, s := range p.in {
				ci, _ := s.ConcatInfo()
				pexp[ci.Seqno-1] = &p.in[i]
				m, err := c.Collect(&s)
				assert.Nil(t, err)
				assert.Nil(t, m)
			}
			select {
			case <-done:
			case <-time.After(50 * time.Millisecond):
				t.Fatalf("didn't expire")
			}
			pipes := c.Pipes()
			assert.Zero(t, len(pipes))
			texp := <-aechan
			assert.Equal(t, pexp, texp)
		}
		t.Run(p.name, f)
	}
}

func TestCollectorPipes(t *testing.T) {
	c := sms.NewCollector()
	patterns := []struct {
		name string
		in   tpdu.TPDU
		m    []*tpdu.TPDU
		out  []sms.Pipe
	}{
		{
			"deliver one a",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 1}},
				},
			},
			nil,
			[]sms.Pipe{
				{
					SmsType: tpdu.SmsDeliver,
					Address: tpdu.Address{Addr: "1234", TOA: 0x91},
					Ref:     1,
					Total:   2,
					Segments: []*tpdu.TPDU{
						{
							OA: tpdu.Address{Addr: "1234", TOA: 0x91},
							UDH: tpdu.UserDataHeader{
								tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 1}},
							},
						},
						nil,
					},
				},
			},
		},
		{
			"deliver two b",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{2, 2, 2}},
				},
			},
			nil,
			[]sms.Pipe{
				{
					SmsType: tpdu.SmsDeliver,
					Address: tpdu.Address{Addr: "1234", TOA: 0x91},
					Ref:     1,
					Total:   2,
					Segments: []*tpdu.TPDU{
						{
							OA: tpdu.Address{Addr: "1234", TOA: 0x91},
							UDH: tpdu.UserDataHeader{
								tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 1}},
							},
						},
						nil,
					},
				},
				{
					SmsType: tpdu.SmsDeliver,
					Address: tpdu.Address{Addr: "1234", TOA: 0x91},
					Ref:     2,
					Total:   2,
					Segments: []*tpdu.TPDU{
						nil,
						{
							OA: tpdu.Address{Addr: "1234", TOA: 0x91},
							UDH: tpdu.UserDataHeader{
								tpdu.InformationElement{ID: 0, Data: []byte{2, 2, 2}},
							},
						},
					},
				},
			},
		},
		{
			"deliver one b",
			tpdu.TPDU{
				OA: tpdu.Address{Addr: "1234", TOA: 0x91},
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 2}},
				},
			},
			[]*tpdu.TPDU{
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 1}},
					},
				},
				{
					OA: tpdu.Address{Addr: "1234", TOA: 0x91},
					UDH: tpdu.UserDataHeader{
						tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 2}},
					},
				},
			},
			[]sms.Pipe{
				{
					SmsType: tpdu.SmsDeliver,
					Address: tpdu.Address{Addr: "1234", TOA: 0x91},
					Ref:     2,
					Total:   2,
					Segments: []*tpdu.TPDU{
						nil,
						{
							OA: tpdu.Address{Addr: "1234", TOA: 0x91},
							UDH: tpdu.UserDataHeader{
								tpdu.InformationElement{ID: 0, Data: []byte{2, 2, 2}},
							},
						},
					},
				},
			},
		},
	}
	for _, p := range patterns {
		m, err := c.Collect(&p.in)
		assert.Nil(t, err, p.name)
		assert.Equal(t, p.m, m, p.name)
		out := c.Pipes()
		assert.Equal(t, p.out, out, p.name)
	}
}

// A concatenation IE whose total is 0, or whose sequence number is 0 or
// greater than the total, is ignored, as required by 3GPP TS 23.040 Sections
// 9.2.3.24.1 and 9.2.3.24.8, so the SM is delivered on its own, and every
// valid sequence number is collected. This covers every total and sequence
// number of both IEs.
func TestCollectorConcatIEValues(t *testing.T) {
	for _, ref16 := range []bool{false, true} {
		for total := 0; total < 256; total++ {
			c := sms.NewCollector()
			for seqno := 0; seqno < 256; seqno++ {
				ie := tpdu.InformationElement{ID: 0, Data: []byte{7, byte(total), byte(seqno)}}
				if ref16 {
					ie = tpdu.InformationElement{ID: 8, Data: []byte{0, 7, byte(total), byte(seqno)}}
				}
				p := tpdu.TPDU{OA: tpdu.Address{Addr: "1234", TOA: 0x91}}
				p.SetUDH(tpdu.UserDataHeader{ie})
				out, err := c.Collect(&p)
				require.NoError(t, err, "ref16 %t total %d seqno %d", ref16, total, seqno)
				switch {
				case seqno == 0 || seqno > total || total == 1:
					require.Len(t, out, 1, "ref16 %t total %d seqno %d", ref16, total, seqno)
					require.True(t, sms.IsCompleteMessage(out))
				case seqno == total:
					require.Len(t, out, total, "ref16 %t total %d seqno %d", ref16, total, seqno)
					require.True(t, sms.IsCompleteMessage(out))
				default:
					require.Nil(t, out, "ref16 %t total %d seqno %d", ref16, total, seqno)
				}
			}
			require.Empty(t, c.Pipes())
			c.Close()
		}
	}
}

// Segments with the same reference but different totals belong to different
// messages.
func TestCollectorKeyIncludesTotal(t *testing.T) {
	c := sms.NewCollector()
	defer c.Close()
	seg := func(total, seqno byte) *tpdu.TPDU {
		p := tpdu.TPDU{OA: tpdu.Address{Addr: "1234", TOA: 0x91}}
		p.SetUDH(tpdu.UserDataHeader{{ID: 0, Data: []byte{7, total, seqno}}})
		return &p
	}
	for _, p := range []*tpdu.TPDU{seg(2, 2), seg(3, 3), seg(3, 1)} {
		out, err := c.Collect(p)
		require.NoError(t, err)
		require.Nil(t, out)
	}
	out, err := c.Collect(seg(2, 1))
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.True(t, sms.IsCompleteMessage(out))
	out, err = c.Collect(seg(3, 2))
	require.NoError(t, err)
	require.Len(t, out, 3)
	assert.True(t, sms.IsCompleteMessage(out))
	assert.Empty(t, c.Pipes())
}

// The same from the wire: the TPDUs are unmarshalled, and an ignored IE does
// not hide a valid one.
func TestCollectorIgnoresInvalidConcatIE(t *testing.T) {
	c := sms.NewCollector()
	defer c.Close()
	// SMS-DELIVER from 1234 with UDHI and the UDH given, then "hello" in 8
	// bit.
	deliver := func(udh ...byte) []byte {
		b := []byte{0x44, 0x04, 0x91, 0x21, 0x43, 0x00, 0x04,
			0x62, 0x80, 0x92, 0x10, 0x00, 0x00, 0x00}
		b = append(b, byte(1+len(udh)+5), byte(len(udh)))
		b = append(b, udh...)
		return append(b, "hello"...)
	}
	for _, udh := range [][]byte{
		{0x00, 0x03, 0x09, 0x02, 0x00},             // seqno 0
		{0x00, 0x03, 0x09, 0x02, 0x03},             // seqno beyond total
		{0x00, 0x03, 0x09, 0x00, 0x01},             // total 0
		{0x08, 0x04, 0x00, 0x09, 0x02, 0x00},       // 16-bit seqno 0
		{0x08, 0x04, 0x00, 0x09, 0x02, 0x09},       // 16-bit seqno beyond total
		{0x00, 0x02, 0x09, 0x02},                   // wrong length
		{0x08, 0x03, 0x00, 0x09, 0x02},             // 16-bit wrong length
		{0x00, 0x03, 0x09, 0x01, 0x01, 0x05, 0x00}, // total 1, and an empty IE
	} {
		pdu, err := sms.Unmarshal(deliver(udh...))
		require.NoError(t, err, "% x", udh)
		out, err := c.Collect(pdu)
		require.NoError(t, err, "% x", udh)
		require.Len(t, out, 1, "% x", udh)
		assert.True(t, sms.IsCompleteMessage(out), "% x", udh)
		msg, err := sms.Decode(out)
		require.NoError(t, err, "% x", udh)
		assert.Equal(t, "hello", string(msg), "% x", udh)
	}
	// An invalid 8-bit IE before a valid 16-bit one: the 16-bit one is used.
	for seqno := byte(1); seqno <= 2; seqno++ {
		pdu, err := sms.Unmarshal(deliver(0x00, 0x03, 0x09, 0x02, 0x00, 0x08, 0x04, 0x01, 0x09, 0x02, seqno))
		require.NoError(t, err)
		out, err := c.Collect(pdu)
		require.NoError(t, err)
		if seqno == 1 {
			assert.Nil(t, out)
			continue
		}
		require.Len(t, out, 2)
		assert.True(t, sms.IsCompleteMessage(out))
		msg, err := sms.Decode(out)
		require.NoError(t, err)
		assert.Equal(t, "hellohello", string(msg))
	}
	assert.Empty(t, c.Pipes())
}

// Pipes returns a snapshot: a later Collect does not change it, and changing
// it does not change the reassembly.
func TestCollectorPipesSnapshot(t *testing.T) {
	c := sms.NewCollector()
	defer c.Close()
	seg := func(seqno byte) *tpdu.TPDU {
		p := tpdu.TPDU{OA: tpdu.Address{Addr: "1234", TOA: 0x91}, UD: []byte{'a' + seqno}}
		p.SetUDH(tpdu.UserDataHeader{{ID: 0, Data: []byte{7, 3, seqno}}})
		return &p
	}
	_, err := c.Collect(seg(1))
	require.NoError(t, err)
	snap := c.Pipes()
	require.Len(t, snap, 1)
	segs := snap[0].Segments
	require.Len(t, segs, 3)
	require.Nil(t, segs[1])
	// a later Collect does not change the snapshot
	_, err = c.Collect(seg(3))
	require.NoError(t, err)
	assert.Nil(t, segs[2])
	// changing the snapshot does not change the reassembly
	segs[1] = &tpdu.TPDU{}
	segs[0].UD[0] = 'X'
	segs[0].UDH[0].Data[2] = 9
	segs[0].OA.Addr = "9999"
	out, err := c.Collect(seg(2))
	require.NoError(t, err)
	require.Len(t, out, 3)
	assert.True(t, sms.IsCompleteMessage(out))
	msg, err := sms.Decode(out)
	require.NoError(t, err)
	assert.Equal(t, "bcd", string(msg))
	assert.Equal(t, "1234", out[0].OA.Addr)
}

// Pipes may be called while other goroutines Collect.
func TestCollectorPipesConcurrent(t *testing.T) {
	c := sms.NewCollector()
	defer c.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		// segments 1 and 2 of 3, so each message stays in a pipe
		for ref := 0; ref < 200; ref++ {
			for seqno := byte(1); seqno <= 2; seqno++ {
				p := tpdu.TPDU{OA: tpdu.Address{Addr: "1234", TOA: 0x91}, UD: []byte{'a'}}
				p.SetUDH(tpdu.UserDataHeader{{ID: 0, Data: []byte{byte(ref), 3, seqno}}})
				_, err := c.Collect(&p)
				assert.NoError(t, err)
			}
		}
	}()
	for finished := false; !finished; {
		select {
		case <-done:
			finished = true
		default:
		}
		for _, pipe := range c.Pipes() {
			for _, s := range pipe.Segments {
				if s != nil {
					assert.Equal(t, tpdu.UserData("a"), s.UD)
					assert.Len(t, s.UDH, 1)
				}
			}
		}
	}
	assert.Len(t, c.Pipes(), 200)
}

// The Collector does not export its lock, which a caller could otherwise
// hold to stall it.
func TestCollectorLockNotExported(t *testing.T) {
	typ := reflect.TypeOf(&sms.Collector{})
	for _, m := range []string{"Lock", "Unlock", "TryLock"} {
		_, ok := typ.MethodByName(m)
		assert.False(t, ok, m)
	}
	for i := 0; i < typ.Elem().NumField(); i++ {
		assert.False(t, typ.Elem().Field(i).IsExported(), typ.Elem().Field(i).Name)
	}
}

// Only SMS-SUBMIT and SMS-DELIVER are reassembled. Any other type is
// rejected before the Collector stores anything, so segments of reports from
// different parties are never merged, and nothing expires.
func TestCollectorRejectsOtherTypes(t *testing.T) {
	for _, st := range []tpdu.SmsType{
		tpdu.SmsDeliverReport, tpdu.SmsSubmitReport, tpdu.SmsStatusReport, tpdu.SmsCommand,
	} {
		expired := make(chan []*tpdu.TPDU, 2)
		c := sms.NewCollector(sms.WithReassemblyTimeout(10*time.Millisecond,
			func(s []*tpdu.TPDU) { expired <- s }))
		seg := func(ra string, seqno byte) tpdu.TPDU {
			p := tpdu.TPDU{}
			require.NoError(t, p.SetSmsType(st))
			p.RA = tpdu.Address{Addr: ra, TOA: 0x91}
			p.DA = tpdu.Address{Addr: ra, TOA: 0x91}
			p.SetUDH(tpdu.UserDataHeader{tpdu.InformationElement{ID: 0, Data: []byte{7, 2, seqno}}})
			return p
		}
		single := tpdu.TPDU{}
		require.NoError(t, single.SetSmsType(st))
		for _, p := range []tpdu.TPDU{seg("111", 1), seg("222", 2), seg("111", 1), single} {
			out, err := c.Collect(&p)
			assert.Equal(t, tpdu.ErrUnsupportedSmsType(st), err, "%s", st)
			assert.Nil(t, out, "%s", st)
			assert.Empty(t, c.Pipes(), "%s", st)
		}
		select {
		case s := <-expired:
			t.Errorf("%s: expired %v", st, s)
		case <-time.After(50 * time.Millisecond):
		}
		c.Close()
	}
}
