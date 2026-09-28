// SPDX-License-Identifier: MIT

package tpdu_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gomaja/go-sms/encoding/bcd"
	"github.com/gomaja/go-sms/encoding/gsm7"
	"github.com/gomaja/go-sms/encoding/semioctet"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/gomaja/go-sms/encoding/ucs2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type BadOption struct {
	err error
}

func (o BadOption) ApplyTPDUOption(t *tpdu.TPDU) error {
	return o.err
}

func TestNew(t *testing.T) {
	s, err := tpdu.New()
	require.Nil(t, err)
	addr := tpdu.NewAddress()
	assert.Equal(t, addr, s.OA)
	assert.Equal(t, addr, s.DA)
	assert.Equal(t, addr, s.RA)
	assert.Equal(t, tpdu.SmsDeliver, s.SmsType())

	s, err = tpdu.New(tpdu.SmsCommand)
	require.Nil(t, err)
	assert.Equal(t, tpdu.SmsCommand, s.SmsType())

	inerr := errors.New("failed TPDU option")
	s, err = tpdu.New(BadOption{inerr})
	assert.Equal(t, inerr, err)
	assert.Nil(t, s)
}

func TestNewDeliver(t *testing.T) {
	s, err := tpdu.NewDeliver()
	require.Nil(t, err)
	addr := tpdu.NewAddress()
	assert.Equal(t, addr, s.OA)
	assert.Equal(t, addr, s.DA)
	assert.Equal(t, addr, s.RA)
	assert.Equal(t, tpdu.SmsDeliver, s.SmsType())

	inerr := errors.New("failed TPDU option")
	s, err = tpdu.NewDeliver(BadOption{inerr})
	assert.Equal(t, inerr, err)
	assert.Nil(t, s)
}

func TestNewSubmit(t *testing.T) {
	s, err := tpdu.NewSubmit()
	require.Nil(t, err)
	addr := tpdu.NewAddress()
	assert.Equal(t, addr, s.OA)
	assert.Equal(t, addr, s.DA)
	assert.Equal(t, addr, s.RA)
	assert.Equal(t, tpdu.SmsSubmit, s.SmsType())

	inerr := errors.New("failed TPDU option")
	s, err = tpdu.NewSubmit(BadOption{inerr})
	assert.Equal(t, inerr, err)
	assert.Nil(t, s)
}

func TestAlphabet(t *testing.T) {
	patterns := []dcsAlphabetPattern{
		{0x00, tpdu.Alpha7Bit},
		{0x04, tpdu.Alpha8Bit},
		{0x08, tpdu.AlphaUCS2},
		{0x0c, tpdu.Alpha7Bit},
		{0x80, tpdu.Alpha7Bit},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			d := tpdu.TPDU{}
			d.DCS = tpdu.DCS(p.in)
			c := d.Alphabet()
			assert.Equal(t, p.out, c)
			// TS 23.040 9.2.3.20: an SMS-COMMAND has no TP-DCS, and its
			// TP-CD is octets.
			d.Direction = tpdu.MO
			d.FirstOctet = tpdu.FirstOctet(tpdu.MtCommand)
			assert.Equal(t, tpdu.Alpha8Bit, d.Alphabet())
		}
		t.Run(fmt.Sprintf("%02x", p.in), f)
	}
}

func TestConcat(t *testing.T) {
	for _, p := range concatTestPatterns {
		f := func(t *testing.T) {
			s, err := tpdu.New(tpdu.WithUDH(p.udh))
			require.Nil(t, err)
			require.NotNil(t, s)
			ci, ok := s.ConcatInfo()
			assert.Equal(t, p.ok, ok)
			assert.Equal(t, p.ci, ci)
			// a concatenated message of 1 segment is a single segment.
			assert.Equal(t, !p.ok || p.ci.Total == 1, s.IsSingleSegment())
		}
		t.Run(p.name, f)
	}
}

func TestMarshalBinary(t *testing.T) {
	patterns := []struct {
		name string
		in   tpdu.TPDU
		out  []byte
		err  error
	}{
		{
			"unsupported SMS type",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x03,
			},
			nil,
			tpdu.ErrUnsupportedSmsType(7),
		},
		{
			"SmsCommand full",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x02,
				PID:        0xab,
				UD:         []byte("a command"),
				MR:         0x42,
				CT:         0x89,
				MN:         0x34,
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			[]byte{
				0x02, 0x42, 0xab, 0x89, 0x34, 0x04, 0x91, 0x36, 0x19, 0x09, 0x61,
				0x20, 0x63, 0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64,
			},
			nil,
		},
		{
			"SmsCommand bad da",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x02,
				PID:        0xab,
				UD:         []byte("a command"),
				MR:         0x42,
				CT:         0x89,
				MN:         0x34,
				DA:         tpdu.Address{Addr: "d391", TOA: 0x91},
			},
			nil,
			tpdu.NewEncodeError("SmsCommand.da.addr", semioctet.ErrInvalidDigit('d')),
		},
		{
			"SmsDeliver haha",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 4,
				UD:         []byte("Hahahaha"),
				OA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600))},
			},
			[]byte{
				0x04, 0x04, 0x91, 0x36, 0x19, 0x00, 0x00, 0x51, 0x50, 0x71, 0x32,
				0x20, 0x05, 0x23, 0x08, 0xC8, 0x30, 0x3A, 0x8C, 0x0E, 0xA3, 0xC3,
			},
			nil,
		},
		{
			"SmsDeliver bad oa",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 4,
				UD:         []byte("Hahahaha"),
				OA:         tpdu.Address{Addr: "d391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			nil,
			tpdu.NewEncodeError("SmsDeliver.oa.addr", semioctet.ErrInvalidDigit('d')),
		},
		{
			"SmsDeliver bad scts",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 4,
				UD:         []byte("Hahahaha"),
				OA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 24*3600)),
				},
			},
			nil,
			tpdu.NewEncodeError("SmsDeliver.scts", bcd.ErrInvalidInteger(96)),
		},
		{
			"SmsDeliver bad ud",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 4,
				DCS:        0x08,
				UD:         []byte("Hahahah"),
				OA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			nil,
			tpdu.NewEncodeError("SmsDeliver.ud.sm", tpdu.ErrOddUCS2Length),
		},
		{
			"SmsDeliverReport minimal",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0,
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
			},
			[]byte{0x00, 0xd0, 0x00},
			nil,
		},
		{
			"SmsDeliverReport pid",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0,
				PID:        0xab,
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
				PI:         0x01,
			},
			[]byte{
				0x00, 0xd0, 0x01, 0xab,
			},
			nil,
		},
		{
			"SmsDeliverReport dcs",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0,
				DCS:        0x04,
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
				PI:         0x02,
			},
			[]byte{
				0x00, 0xd0, 0x02, 0x04,
			},
			nil,
		},
		{
			"SmsDeliverReport ud",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0,
				UD:         []byte("report"),
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
				PI:         0x04,
			},
			[]byte{
				0x00, 0xd0, 0x04, 0x06, 0xf2, 0x32, 0xfc, 0x2d, 0xa7, 0x03,
			},
			nil,
		},
		{
			// TS 23.040 9.2.3.27: the PI bits are derived from the fields,
			// so the UD is not encoded with a DCS the receiver cannot see.
			"SmsDeliverReport pi derived",
			tpdu.TPDU{
				Direction: tpdu.MO,
				PID:       0xab,
				DCS:       0x04,
				UD:        []byte("report"),
				PI:        0x01,
			},
			[]byte{
				0x00, 0x07, 0xab, 0x04, 0x06, 0x72, 0x65, 0x70, 0x6f, 0x72, 0x74,
			},
			nil,
		},
		{
			"SmsDeliverReport full",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0,
				PID:        0xab,
				DCS:        0x04,
				UD:         []byte("report"),
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
				PI:         0x07,
			},
			[]byte{
				0x00, 0xd0, 0x07, 0xab, 0x04, 0x06, 0x72, 0x65, 0x70, 0x6f, 0x72,
				0x74,
			},
			nil,
		},
		{
			"SmsDeliverReport bad ud",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0,
				DCS:        0x08,
				UD:         []byte("report!"),
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
				PI:         0x06,
			},
			nil,
			tpdu.NewEncodeError("SmsDeliverReport.ud.sm", tpdu.ErrOddUCS2Length),
		},
		{
			"SmsStatusReport minimal",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
				ST: 0xab,
			},
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab,
			},
			nil,
		},
		{
			"SmsStatusReport full",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x06,
				PID:        0x89,
				DCS:        0x04,
				UD:         []byte("report"),
				MR:         0x42,
				PI:         0x07,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
				ST: 0xab,
			},
			[]byte{
				0x06, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab, 0x07,
				0x89, 0x04, 0x06, 0x72, 0x65, 0x70, 0x6f, 0x72, 0x74,
			},
			nil,
		},
		{
			"SmsStatusReport pidless",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x06,
				DCS:        0x04,
				UD:         []byte("report"),
				MR:         0x42,
				PI:         0x06, // no PID set
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
				ST: 0xab,
			},
			[]byte{
				0x06, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab, 0x06,
				0x04, 0x06, 0x72, 0x65, 0x70, 0x6f, 0x72, 0x74,
			},
			nil,
		},
		{
			"SmsStatusReport dcsless",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x06,
				PID:        0x89,
				UD:         []byte("report"),
				MR:         0x42,
				PI:         0x05, // no DCS set
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
				ST: 0xab,
			},
			[]byte{
				0x06, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab, 0x05,
				0x89, 0x06, 0xf2, 0x32, 0xfc, 0x2d, 0xa7, 0x03,
			},
			nil,
		},
		{
			"SmsStatusReport bad ra",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				RA:         tpdu.Address{Addr: "63d1", TOA: 0x91},
			},
			nil,
			tpdu.NewEncodeError("SmsStatusReport.ra.addr", semioctet.ErrInvalidDigit('d')),
		},
		{
			"SmsStatusReport bad scts",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 24*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
				ST: 0xab,
			},
			nil,
			tpdu.NewEncodeError("SmsStatusReport.scts", bcd.ErrInvalidInteger(96)),
		},
		{
			"SmsStatusReport bad dt",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 24*3600)),
				},
				ST: 0xab,
			},
			nil,
			tpdu.NewEncodeError("SmsStatusReport.dt", bcd.ErrInvalidInteger(96)),
		},
		{
			"SmsStatusReport bad ud",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				DCS:        0x08,
				UD:         []byte("report!"),
				PI:         0x06,
			},
			nil,
			tpdu.NewEncodeError("SmsStatusReport.ud.sm", tpdu.ErrOddUCS2Length),
		},
		{
			"SmsSubmit haha",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
				UD:         []byte("Hahahaha"),
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			[]byte{
				0x01, 0x00, 0x04, 0x91, 0x36, 0x19, 0x00, 0x00, 0x08, 0xC8, 0x30,
				0x3A, 0x8C, 0x0E, 0xA3, 0xC3,
			},
			nil,
		},
		{
			"SmsSubmit vp",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
				UD:         []byte("Hahahaha"),
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				VP: tpdu.ValidityPeriod{
					Format:   tpdu.VpfRelative,
					Duration: time.Duration(6000000000000),
				},
			},
			[]byte{
				0x11, 0x00, 0x04, 0x91, 0x36, 0x19, 0x00, 0x00, 0x13, 0x08, 0xC8,
				0x30, 0x3A, 0x8C, 0x0E, 0xA3, 0xC3,
			},
			nil,
		},
		{
			"SmsSubmit bad da",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
				UD:         []byte("Hahahaha"),
				DA:         tpdu.Address{Addr: "d391", TOA: 0x91},
			},
			nil,
			tpdu.NewEncodeError("SmsSubmit.da.addr", semioctet.ErrInvalidDigit('d')),
		},
		{
			"SmsSubmit bad vp",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
				UD:         []byte("Hahahaha"),
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				VP:         tpdu.ValidityPeriod{Format: 6},
			},
			nil,
			tpdu.NewEncodeError("SmsSubmit.vp.vpf", tpdu.ErrInvalid),
		},
		{
			"SmsSubmit bad ud",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
				DCS:        0x08,
				UD:         []byte("Hahahah"),
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			nil,
			tpdu.NewEncodeError("SmsSubmit.ud.sm", tpdu.ErrOddUCS2Length),
		},
		{
			"SmsSubmitReport minimal",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			[]byte{0x01, 0xc0, 0x00, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23},
			nil,
		},
		{
			"SmsSubmitReport pi",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				PID:        0xab,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         0x01,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			[]byte{
				0x01, 0xc0, 0x01, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23, 0xab,
			},
			nil,
		},
		{
			"SmsSubmitReport dcs",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				DCS:        0x04,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         0x02,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			[]byte{
				0x01, 0xc0, 0x02, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23, 0x04,
			},
			nil,
		},
		{
			"SmsSubmitReport ud",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				UD:         []byte("report"),
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         0x04,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			[]byte{
				0x01, 0xc0, 0x04, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23,
				0x06, 0xf2, 0x32, 0xfc, 0x2d, 0xa7, 0x03,
			},
			nil,
		},
		{
			"SmsSubmitReport full",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				PID:        0xab,
				DCS:        0x04,
				UD:         []byte("report"),
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         0x07,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			[]byte{
				0x01, 0xc0, 0x07, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23, 0xab,
				0x04, 0x06, 0x72, 0x65, 0x70, 0x6f, 0x72, 0x74,
			},
			nil,
		},
		{
			"SmsSubmitReport bad scts",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				DCS:        0x80,
				UD:         []byte("report"),
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         0x07,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 24*3600))},
			},
			nil,
			tpdu.NewEncodeError("SmsSubmitReport.scts", bcd.ErrInvalidInteger(96)),
		},
		{
			"SmsSubmitReport bad ud",
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				DCS:        0x08,
				UD:         []byte("report!"),
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         0x06,
			},
			nil,
			tpdu.NewEncodeError("SmsSubmitReport.ud.sm", tpdu.ErrOddUCS2Length),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			b, err := p.in.MarshalBinary()
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, b)
		}
		t.Run(p.name, f)
	}
}

func TestIsSingleSegment(t *testing.T) {
	patterns := []struct {
		name string
		in   tpdu.TPDU
		out  bool
	}{
		{
			"single segment",
			tpdu.TPDU{},
			true,
		},
		{
			"concat8",
			tpdu.TPDU{
				UDH: []tpdu.InformationElement{
					{
						ID:   0,
						Data: []byte{1, 2, 1},
					},
				},
			},
			false,
		},
		{
			"concat16",
			tpdu.TPDU{
				UDH: []tpdu.InformationElement{
					{
						ID:   8,
						Data: []byte{0, 1, 2, 1},
					},
				},
			},
			false,
		},
		{
			"no concat",
			tpdu.TPDU{
				UDH: []tpdu.InformationElement{
					{
						ID:   3,
						Data: []byte{1, 2, 1},
					},
				},
			},
			true,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out := p.in.IsSingleSegment()
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

func TestMTI(t *testing.T) {
	b := tpdu.TPDU{}
	m := b.MTI()
	assert.Equal(t, tpdu.MtDeliver, m)
	for _, p := range []tpdu.FirstOctet{0x00, 0xab, 0x00, 0xff} {
		b.FirstOctet = p
		m = b.MTI()
		assert.Equal(t, tpdu.MessageType(p&0x3), m)
	}
}

func TestSetDCS(t *testing.T) {
	b := tpdu.TPDU{}
	assert.False(t, b.PI.DCS())
	for _, p := range []byte{0x00, 0xab, 0x00, 0xff} {
		b.SetDCS(p)
		assert.Equal(t, p, byte(b.DCS))
		assert.True(t, b.PI.DCS())
	}
}

func TestSegment(t *testing.T) {
	patterns := []struct {
		name    string
		in      tpdu.TPDU
		msg     []byte
		options []tpdu.SegmentationOption
		out     []tpdu.TPDU
	}{
		{
			// an empty message is carried by one TPDU with a TP-UDL of 0.
			"nil msg",
			tpdu.TPDU{},
			nil,
			nil,
			[]tpdu.TPDU{{}},
		},
		{
			"empty msg",
			tpdu.TPDU{MR: 3},
			[]byte{},
			nil,
			[]tpdu.TPDU{{MR: 3}},
		},
		{
			"single segment",
			tpdu.TPDU{},
			[]byte("hello"),
			nil,
			[]tpdu.TPDU{
				{
					UD: []byte("hello"),
				},
			},
		},
		{
			"two segment 7bit",
			tpdu.TPDU{},
			[]byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you might think"),
			nil,
			[]tpdu.TPDU{
				{
					FirstOctet: tpdu.FoUDHI,
					PI:         tpdu.PiUDL,
					UDH: []tpdu.InformationElement{
						{
							ID:   0,
							Data: []byte{1, 2, 1},
						},
					},
					UD: []byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you mi"),
				},
				{
					FirstOctet: tpdu.FoUDHI,
					PI:         tpdu.PiUDL,
					// TS 23.040 9.2.3.24.1: "TP-MR must be incremented for
					// every segment of a concatenated message"
					MR: 1,
					UDH: []tpdu.InformationElement{
						{
							ID:   0,
							Data: []byte{1, 2, 2},
						},
					},
					UD: []byte("ght think"),
				},
			},
		},
		{
			"three segment 7bit withMR",
			tpdu.TPDU{},
			[]byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you might think, but wait, then we also need a really really long message to trigger a three segment concatenation which requires even more characters than I care to count"),
			[]tpdu.SegmentationOption{
				tpdu.WithMR(&counter{10}),
				tpdu.WithConcatRef(&counter{6}),
			},
			[]tpdu.TPDU{
				{
					FirstOctet: tpdu.FoUDHI,
					PI:         tpdu.PiUDL,
					MR:         11,
					UDH: []tpdu.InformationElement{
						{
							ID:   0,
							Data: []byte{7, 3, 1},
						},
					},
					UD: []byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you mi"),
				},
				{
					FirstOctet: tpdu.FoUDHI,
					PI:         tpdu.PiUDL,
					MR:         12,
					UDH: []tpdu.InformationElement{
						{
							ID:   0,
							Data: []byte{7, 3, 2},
						},
					},
					UD: []byte("ght think, but wait, then we also need a really really long message to trigger a three segment concatenation which requires even more characters than I c"),
				},
				{
					FirstOctet: tpdu.FoUDHI,
					PI:         tpdu.PiUDL,
					MR:         13,
					UDH: []tpdu.InformationElement{
						{
							ID:   0,
							Data: []byte{7, 3, 3},
						},
					},
					UD: []byte("are to count"),
				},
			},
		},
		{
			"three segment 7bit with16BitConcatRef",
			tpdu.TPDU{},
			[]byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you might think, but wait, then we also need a really really long message to trigger a three segment concatenation which requires even more characters than I care to count"),
			[]tpdu.SegmentationOption{
				tpdu.WithMR(&counter{20}),
				tpdu.WithConcatRef(&counter{0x507}),
				tpdu.With16BitConcatRef,
			},
			[]tpdu.TPDU{
				{
					FirstOctet: tpdu.FoUDHI,
					PI:         tpdu.PiUDL,
					MR:         21,
					UDH: []tpdu.InformationElement{
						{
							ID:   8,
							Data: []byte{5, 8, 3, 1},
						},
					},
					UD: []byte("this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you m"),
				},
				{
					FirstOctet: tpdu.FoUDHI,
					PI:         tpdu.PiUDL,
					MR:         22,
					UDH: []tpdu.InformationElement{
						{
							ID:   8,
							Data: []byte{5, 8, 3, 2},
						},
					},
					UD: []byte("ight think, but wait, then we also need a really really long message to trigger a three segment concatenation which requires even more characters than I"),
				},
				{
					FirstOctet: tpdu.FoUDHI,
					PI:         tpdu.PiUDL,
					MR:         23,
					UDH: []tpdu.InformationElement{
						{
							ID:   8,
							Data: []byte{5, 8, 3, 3},
						},
					},
					UD: []byte(" care to count"),
				},
			},
		},
		{
			"8bit",
			tpdu.TPDU{
				DCS: tpdu.Dcs8BitData,
			},
			[]byte("hello"),
			nil,
			[]tpdu.TPDU{
				{
					DCS: tpdu.Dcs8BitData,
					UD:  []byte("hello"),
				},
			},
		},
		{
			"ucs2",
			tpdu.TPDU{
				DCS: tpdu.DcsUCS2Data,
			},
			[]byte{0x00, 'h', 0x00, 'i'},
			nil,
			[]tpdu.TPDU{
				{
					DCS: tpdu.DcsUCS2Data,
					UD:  []byte{0x00, 'h', 0x00, 'i'},
				},
			},
		},
		{
			"single segment withMR",
			tpdu.TPDU{},
			[]byte("hello"),
			[]tpdu.SegmentationOption{
				tpdu.WithMR(&counter{42}),
			},
			[]tpdu.TPDU{
				{
					MR: 43,
					UD: []byte("hello"),
				},
			},
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out, err := p.in.Segment(p.msg, p.options...)
			require.NoError(t, err)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

func TestSegmentUCS2DoesNotSplitSurrogatePair(t *testing.T) {
	// Concatenated SMS TP-UD is limited by 3GPP TS 23.040 Section 9.2.3.24.
	msg := ucs2.Encode([]rune(strings.Repeat("a", 66) + "😁bbb"))
	in := tpdu.TPDU{DCS: tpdu.DcsUCS2Data}

	segments, err := in.Segment(msg)
	require.NoError(t, err)

	require.Len(t, segments, 2)
	require.Len(t, segments[0].UD, 132)
	require.GreaterOrEqual(t, len(segments[1].UD), 4)
	assert.Equal(t, []byte{0x00, 'a'}, []byte(segments[0].UD[len(segments[0].UD)-2:]))
	assert.Equal(t, []byte{0xd8, 0x3d, 0xde, 0x01}, []byte(segments[1].UD[:4]))
}

func TestSetPID(t *testing.T) {
	b := tpdu.TPDU{}
	assert.Zero(t, b.PI)
	for _, p := range []byte{0x00, 0xab, 0x00, 0xff} {
		b.SetPID(p)
		assert.Equal(t, p, b.PID)
		assert.True(t, b.PI.PID())
	}
}

func TestSetSmsType(t *testing.T) {
	patterns := []struct {
		in  tpdu.SmsType
		err error
	}{
		{tpdu.SmsDeliver, nil},
		{tpdu.SmsSubmit, nil},
		{7, tpdu.ErrInvalid},
	}
	for _, p := range patterns {
		s, err := tpdu.New()
		require.Nil(t, err)
		err = s.SetSmsType(p.in)
		assert.Equal(t, p.err, err)
		if err == nil {
			assert.Equal(t, p.in, s.SmsType())
		} else {
			assert.Equal(t, tpdu.SmsType(0), s.SmsType())
		}
	}
}

func TestSetUD(t *testing.T) {
	b := tpdu.TPDU{}
	assert.Zero(t, b.PI)
	for _, p := range []byte{0x00, 0xab, 0x00, 0xff} {
		b.SetUD([]byte{p})
		assert.Equal(t, []byte{p}, []byte(b.UD))
		assert.True(t, b.PI.UDL())
	}
	// Reset
	b.SetUD(nil)
	assert.Nil(t, b.UD)
	assert.False(t, b.PI.UDL())
}

func TestSetUDH(t *testing.T) {
	// also tests tpdu.TPDU.UDH
	b := tpdu.TPDU{}
	udh := b.UDH
	assert.Zero(t, len(udh))
	for _, p := range []tpdu.UserDataHeader{
		nil,
		{
			tpdu.InformationElement{ID: 1, Data: []byte{5, 6, 7}},
		},
		{
			tpdu.InformationElement{ID: 1, Data: []byte{1, 2, 3}},
			tpdu.InformationElement{ID: 1, Data: []byte{5, 6, 7}},
		},
		nil,
	} {
		b.SetUDH(p)
		udh = b.UDH
		assert.Equal(t, len(p) != 0, b.UDHI())
		assert.Equal(t, udh, p)
	}
}

func TestSetValidityPeriod(t *testing.T) {
	// also tests Submit.VP
	s := tpdu.TPDU{}
	vp := s.VP
	assert.Equal(t, tpdu.VpfNotPresent, vp.Format)
	pvp := tpdu.ValidityPeriod{}
	pvp.SetRelative(time.Duration(100000000))
	for _, p := range []struct {
		vp tpdu.ValidityPeriod
		fo tpdu.FirstOctet
	}{
		{tpdu.ValidityPeriod{}, 0x00},
		{pvp, 0x10},
		{tpdu.ValidityPeriod{}, 0x00},
	} {
		s.SetVP(p.vp)
		vp = s.VP
		assert.Equal(t, p.fo, s.FirstOctet)
		assert.Equal(t, vp, p.vp)
	}
}

func TestMessageTypeString(t *testing.T) {
	patterns := []struct {
		mti tpdu.MessageType
		out string
	}{
		{0, "Deliver"},
		{1, "Submit"},
		{2, "Command"},
		{3, "Unknown"},
	}
	for _, p := range patterns {
		assert.Equal(t, p.out, p.mti.String())
	}
}

func TestSmsType(t *testing.T) {
	patterns := []struct {
		dirn tpdu.Direction
		mt   tpdu.MessageType
		st   tpdu.SmsType
	}{
		{tpdu.MT, tpdu.MtDeliver, tpdu.SmsDeliver},
		{tpdu.MT, tpdu.MtSubmit, tpdu.SmsSubmitReport},
		{tpdu.MT, tpdu.MtCommand, tpdu.SmsStatusReport},
		{tpdu.MO, tpdu.MtDeliver, tpdu.SmsDeliverReport},
		{tpdu.MO, tpdu.MtSubmit, tpdu.SmsSubmit},
		{tpdu.MO, tpdu.MtCommand, tpdu.SmsCommand},
		// TS 23.040 9.2.3.1
		{tpdu.MT, tpdu.MtReserved, tpdu.SmsDeliver},
		{tpdu.MO, tpdu.MtReserved, tpdu.SmsType(7)},
	}
	s := tpdu.TPDU{}
	for _, p := range patterns {
		s.Direction = p.dirn
		s.FirstOctet = tpdu.FirstOctet(p.mt)
		assert.Equal(t, p.st, s.SmsType())
	}
}

// TestInvalidDirection checks that a TPDU with a Direction other than MT or
// MO, as set on the exported field, is of no type, so every method that
// depends on the type rejects it, rather than the Direction being ORed into
// the TP-MTI, which would make an SMS-DELIVER an SMS-SUBMIT-REPORT or an
// SMS-SUBMIT.
func TestInvalidDirection(t *testing.T) {
	deliver := unhex(t, "04 04 91 3619 00 00 51507132200523 01 41")
	for _, d := range []tpdu.Direction{-1, 2, 3, 4, 7} {
		for _, mt := range []tpdu.MessageType{tpdu.MtDeliver, tpdu.MtSubmit, tpdu.MtCommand, tpdu.MtReserved} {
			name := fmt.Sprintf("%d %s", d, mt)
			p := tpdu.TPDU{Direction: d, FirstOctet: tpdu.FirstOctet(mt), OA: tpdu.Address{Addr: "6391", TOA: 0x91}}
			st := p.SmsType()
			assert.Equal(t, "Unknown", st.String(), name)
			b, err := p.MarshalBinary()
			assert.Equal(t, tpdu.ErrUnsupportedSmsType(st), err, name)
			assert.Nil(t, b, name)
			pdus, err := p.Segment([]byte("hi"))
			assert.Equal(t, tpdu.ErrUnsupportedSmsType(st), err, name)
			assert.Nil(t, pdus, name)
			assert.Error(t, p.SetSmsType(st), name)
			in := append([]byte{byte(mt)}, deliver[1:]...)
			u := tpdu.TPDU{Direction: d}
			err = u.UnmarshalBinary(in)
			assert.Equal(t, tpdu.NewDecodeError("tpdu.firstOctet", 0, tpdu.ErrUnsupportedSmsType(st)), err, name)
			assert.Equal(t, tpdu.TPDU{Direction: d, FirstOctet: tpdu.FirstOctet(mt)}, u, name)
		}
	}
}

func TestSmsTypeString(t *testing.T) {
	patterns := []struct {
		st  tpdu.SmsType
		out string
	}{
		{0, "SmsDeliver"},
		{1, "SmsDeliverReport"},
		{2, "SmsSubmitReport"},
		{3, "SmsSubmit"},
		{4, "SmsStatusReport"},
		{5, "SmsCommand"},
		{6, "Unknown"},
	}
	for _, p := range patterns {
		assert.Equal(t, p.out, p.st.String())
	}
}

func TestUDBlockSize(t *testing.T) {
	patterns := []struct {
		name string
		pdu  tpdu.TPDU
		out  int
	}{
		{
			"command dcs 0",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtCommand),
			},
			// TS 23.040 9.2.3.20: TP-CDL counts octets, and 9.2.2.4 gives
			// 156 with a TP-DA of 2 octets.
			156,
		},
		{
			"command 8bit",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtCommand),
				DCS:        0xf4,
			},
			156,
		},
		{
			"deliver 7bit",
			tpdu.TPDU{},
			160,
		},
		{
			"deliver UDH 7bit",
			tpdu.TPDU{
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 1, Data: []byte{5, 6, 7}},
				},
			},
			153,
		},
		{
			"deliver 8bit",
			tpdu.TPDU{
				DCS: 0xf4,
			},
			140,
		},
		{
			"deliver UDH 8bit",
			tpdu.TPDU{
				DCS: 0xf4,
				UDH: tpdu.UserDataHeader{
					tpdu.InformationElement{ID: 1, Data: []byte{5, 6, 7}},
				},
			},
			134,
		},
		{
			"deliver UCS2",
			tpdu.TPDU{
				DCS: 0xe0,
			},
			140,
		},
		{
			"deliverreport RP-ACK 7bit",
			tpdu.TPDU{
				Direction: tpdu.MO,
			},
			181,
		},
		{
			"deliverreport RP-ERROR 7bit",
			tpdu.TPDU{
				Direction: tpdu.MO,
				RPMessage: tpdu.RPError,
			},
			180,
		},
		{
			"deliverreport RP-ACK 8bit",
			tpdu.TPDU{
				Direction: tpdu.MO,
				DCS:       0xf4,
			},
			159,
		},
		{
			"deliverreport RP-ERROR 8bit",
			tpdu.TPDU{
				Direction: tpdu.MO,
				DCS:       0xf4,
				RPMessage: tpdu.RPError,
			},
			158,
		},
		{
			"statusreport 7bit",
			tpdu.TPDU{
				FirstOctet: tpdu.FirstOctet(tpdu.MtCommand),
			},
			// TS 23.040 9.2.2.3: 143 octets with a TP-RA of 2 octets and no
			// TP-PID or TP-DCS.
			163,
		},
		{
			"statusreport 8bit",
			tpdu.TPDU{
				FirstOctet: tpdu.FirstOctet(tpdu.MtCommand),
				DCS:        0xf4,
			},
			142, // the TP-DCS takes an octet
		},
		{
			"statusreport UCS2",
			tpdu.TPDU{
				FirstOctet: tpdu.FirstOctet(tpdu.MtCommand),
				DCS:        0xe0,
			},
			142,
		},
		{
			"submit 7bit",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
			},
			160,
		},
		{
			"submit 8bit",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				DCS:        0xf4,
			},
			140,
		},
		{
			"submit UCS2",
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				DCS:        0xe0,
			},
			140,
		},
		{
			"submitreport RP-ACK 7bit",
			tpdu.TPDU{
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
			},
			173,
		},
		{
			"submitreport RP-ERROR 7bit",
			tpdu.TPDU{
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				RPMessage:  tpdu.RPError,
			},
			172,
		},
		{
			"submitreport RP-ACK 8bit",
			tpdu.TPDU{
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				DCS:        0xf4,
			},
			152,
		},
		{
			"submitreport RP-ERROR 8bit",
			tpdu.TPDU{
				FirstOctet: tpdu.FirstOctet(tpdu.MtSubmit),
				DCS:        0xf4,
				RPMessage:  tpdu.RPError,
			},
			151,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			assert.Equal(t, p.out, p.pdu.UDBlockSize())
		}
		t.Run(p.name, f)
	}
}

func TestUDHI(t *testing.T) {
	// also tests tpdu.TPDU.SetUDH
	b := tpdu.TPDU{}
	assert.False(t, b.UDHI())
	for _, p := range []tpdu.UserDataHeader{
		nil,
		{
			tpdu.InformationElement{ID: 1, Data: []byte{5, 6, 7}},
		},
		{
			tpdu.InformationElement{ID: 1, Data: []byte{1, 2, 3}},
			tpdu.InformationElement{ID: 1, Data: []byte{5, 6, 7}},
		},
		nil,
	} {
		b.SetUDH(p)
		assert.Equal(t, len(p) != 0, b.UDHI())
	}
}

// TestUDHIWithoutUD checks that a TPDU received with the TP-UDHI set but no
// TP-UD has no UDH, as there is no header to decode, and keeps the bit, so
// it marshals back to the octets it came from.
//
// TS 23.040 9.2.3.16: "If this field is zero, the TP-User-Data field shall
// not be present."
func TestUDHIWithoutUD(t *testing.T) {
	for _, p := range []struct {
		name string
		dirn tpdu.Direction
		in   string
	}{
		{"deliver udl 0", tpdu.MT, "44 04 91 2143 00 00 99202150750321 00"},
		{"command cdl 0", tpdu.MO, "42 42 00 00 34 04 91 3619 00"},
		{"status report without pi", tpdu.MT, "42 42 04 91 3619 51507132200523 51408132200542 ab"},
		{"status report without udl", tpdu.MT, "42 42 04 91 3619 51507132200523 51408132200542 ab 01 00"},
		{"deliver report without udl", tpdu.MO, "40 00"},
	} {
		f := func(t *testing.T) {
			in := unhex(t, p.in)
			d := tpdu.TPDU{Direction: p.dirn}
			require.NoError(t, d.UnmarshalBinary(in))
			assert.True(t, d.UDHI())
			assert.Nil(t, d.UDH)
			assert.Equal(t, 0, d.UDHL())
			out, err := d.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, in, out)
		}
		t.Run(p.name, f)
	}
}

func TestUnmarshalBinary(t *testing.T) {
	patterns := []struct {
		name string
		in   []byte
		dirn tpdu.Direction
		out  tpdu.TPDU
		err  error
	}{
		{
			"underflow fo",
			[]byte{},
			tpdu.MT,
			tpdu.TPDU{},
			tpdu.NewDecodeError("tpdu.firstOctet", 0, tpdu.ErrUnderflow),
		},
		{
			// TS 23.040 9.2.3.1: processed as an SMS-DELIVER
			"reserved MTI MT",
			[]byte{0x03},
			tpdu.MT,
			tpdu.TPDU{FirstOctet: 0x03},
			tpdu.NewDecodeError("SmsDeliver.oa.addr", 1, tpdu.ErrUnderflow),
		},
		{
			"unsupported SMS type",
			[]byte{0x03},
			tpdu.MO,
			tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x03},
			tpdu.NewDecodeError("tpdu.firstOctet", 0, tpdu.ErrUnsupportedSmsType(7)),
		},
		{
			"SmsCommand",
			[]byte{
				0x02, 0x42, 0xab, 0x89, 0x34, 0x04, 0x91, 0x36, 0x19, 0x09, 0x61,
				0x20, 0x63, 0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x02,
				PID:        0xab,
				UD:         []byte("a command"),
				MR:         0x42,
				CT:         0x89,
				MN:         0x34,
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			nil,
		},
		{
			"SmsCommand underflow mr",
			[]byte{0x02},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x02,
			},
			tpdu.NewDecodeError("SmsCommand.mr", 1, tpdu.ErrUnderflow),
		},
		{
			"SmsCommand underflow pid",
			[]byte{0x02, 0x42},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x02,
				MR:         0x42,
			},
			tpdu.NewDecodeError("SmsCommand.pid", 2, tpdu.ErrUnderflow),
		},
		{
			"SmsCommand underflow ct",
			[]byte{0x02, 0x42, 0xab},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x02,
				PID:        0xab,
				MR:         0x42,
			},
			tpdu.NewDecodeError("SmsCommand.ct", 3, tpdu.ErrUnderflow),
		},
		{
			"SmsCommand underflow mn",
			[]byte{0x02, 0x42, 0xab, 0x89},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x02,
				PID:        0xab,
				MR:         0x42,
				CT:         0x89,
			},
			tpdu.NewDecodeError("SmsCommand.mn", 4, tpdu.ErrUnderflow),
		},
		{
			"SmsCommand underflow da",
			[]byte{0x02, 0x42, 0xab, 0x89, 0x34, 0x04, 0x91, 0x36, 0xF9, 0x09},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x02,
				PID:        0xab,
				MR:         0x42,
				CT:         0x89,
				MN:         0x34,
			},
			tpdu.NewDecodeError("SmsCommand.da.addr", 7, tpdu.ErrUnderflow),
		},
		{
			"SmsCommand underflow ud",
			[]byte{0x02, 0x42, 0xab, 0x89, 0x34, 0x04, 0x91, 0x36, 0x19},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x02,
				PID:        0xab,
				MR:         0x42,
				CT:         0x89,
				MN:         0x34,
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			tpdu.NewDecodeError("SmsCommand.ud.udl", 9, tpdu.ErrUnderflow),
		},
		{
			"SmsDeliver haha",
			[]byte{
				0x04, 0x04, 0x91, 0x36, 0x19, 0x00, 0x00, 0x51, 0x50, 0x71, 0x32,
				0x20, 0x05, 0x23, 0x08, 0xC8, 0x30, 0x3A, 0x8C, 0x0E, 0xA3, 0xC3,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x04,
				UD:         []byte("Hahahaha"),
				OA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			nil,
		},
		{
			"SmsDeliver underflow oa",
			[]byte{0x04, 0x04, 0x91, 0x36, 0xF9, 0x00, 0x00},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x04,
			},
			tpdu.NewDecodeError("SmsDeliver.oa.addr", 3, tpdu.ErrUnderflow),
		},
		{
			"SmsDeliver underflow pid",
			[]byte{0x04, 0x04, 0x91, 0x36, 0x19},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x04,
				OA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			tpdu.NewDecodeError("SmsDeliver.pid", 5, tpdu.ErrUnderflow),
		},
		{
			"SmsDeliver underflow dcs",
			[]byte{0x04, 0x04, 0x91, 0x36, 0x19, 0x00},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x04,
				OA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			tpdu.NewDecodeError("SmsDeliver.dcs", 6, tpdu.ErrUnderflow),
		},
		{
			"SmsDeliver underflow scts",
			[]byte{0x04, 0x04, 0x91, 0x36, 0x19, 0x00, 0x00, 0x51},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x04,
				OA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			tpdu.NewDecodeError("SmsDeliver.scts", 7, tpdu.ErrUnderflow),
		},
		{
			"SmsDeliver non-integer scts",
			[]byte{
				0x04, 0x04, 0x91, 0x36, 0x19, 0x00, 0x00, 0x51, 0x50, 0xf1, 0x32,
				0x20, 0x05, 0x23,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x04,
				OA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS:       timestampFrom(0x51, 0x50, 0xf1, 0x32, 0x20, 0x05, 0x23),
			},
			tpdu.NewDecodeError("SmsDeliver.ud.udl", 14, tpdu.ErrUnderflow),
		},
		{
			"SmsDeliver underflow ud",
			[]byte{
				0x04, 0x04, 0x91, 0x36, 0x19, 0x00, 0x00, 0x51, 0x50, 0x71, 0x32,
				0x20, 0x05, 0x23, 0x08,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x04,
				OA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			tpdu.NewDecodeError("SmsDeliver.ud.sm", 15, tpdu.ErrUnderflow),
		},
		{
			"SmsDeliverReport minimal",
			[]byte{0x00, 0xd0, 0x00},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x00,
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
			},
			nil,
		},
		{
			"SmsDeliverReport pid",
			[]byte{0x00, 0xd0, 0x01, 0xab},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x00,
				PID:        0xab,
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
				PI:         0x01,
			},
			nil,
		},
		{
			"SmsDeliverReport dcs",
			[]byte{0x00, 0xd0, 0x02, 0x04},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x00,
				DCS:        0x04,
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
				PI:         0x02,
			},
			nil,
		},
		{
			"SmsDeliverReport ud",
			[]byte{
				0x00, 0xd0, 0x06, 0x04, 0x06, 0x72, 0x65, 0x70, 0x6f, 0x72, 0x74,
			},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x00,
				DCS:        0x04,
				UD:         []byte("report"),
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
				PI:         0x06,
			},
			nil,
		},
		{
			"SmsDeliverReport underflow fcs",
			[]byte{0x00},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				RPMessage:  tpdu.RPError,
				FirstOctet: 0x00,
			},
			tpdu.NewDecodeError("SmsDeliverReport.fcs", 1, tpdu.ErrUnderflow),
		},
		{
			"SmsDeliverReport underflow pi",
			[]byte{0x00, 0xd0},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x00,
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
			},
			tpdu.NewDecodeError("SmsDeliverReport.pi", 2, tpdu.ErrUnderflow),
		},
		{
			"SmsDeliverReport underflow pid",
			[]byte{0x00, 0xd0, 0x01},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x00,
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
				PI:         0x01,
			},
			tpdu.NewDecodeError("SmsDeliverReport.pid", 3, tpdu.ErrUnderflow),
		},
		{
			"SmsDeliverReport underflow dcs",
			[]byte{0x00, 0xd0, 0x02},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x00,
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
				PI:         0x02,
			},
			tpdu.NewDecodeError("SmsDeliverReport.dcs", 3, tpdu.ErrUnderflow),
		},
		{
			"SmsDeliverReport underflow ud",
			[]byte{0x00, 0xd0, 0x04},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x00,
				RPMessage:  tpdu.RPError,
				FCS:        0xd0,
				PI:         0x04,
			},
			tpdu.NewDecodeError("SmsDeliverReport.ud.udl", 3, tpdu.ErrUnderflow),
		},
		{
			"SmsStatusReport minimal",
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
				ST: 0xab,
			},
			nil,
		},
		{
			"SmsStatusReport pid",
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab, 0x01,
				0x89,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				PID:        0x89,
				MR:         0x42,
				PI:         0x01,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
				ST: 0xab,
			},
			nil,
		},
		{
			"SmsStatusReport dcs",
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab, 0x02,
				0x04,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				DCS:        0x04,
				MR:         0x42,
				PI:         0x02,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
				ST: 0xab,
			},
			nil,
		},
		{
			"SmsStatusReport ud",
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab, 0x06,
				0x04, 0x06, 0x72, 0x65, 0x70, 0x6f, 0x72, 0x74,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				DCS:        0x04,
				UD:         []byte("report"),
				MR:         0x42,
				PI:         0x06,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
				ST: 0xab,
			},
			nil,
		},
		{
			"SmsStatusReport underflow mr",
			[]byte{0x02},
			tpdu.MT,
			tpdu.TPDU{
				FirstOctet: 0x02,
			},
			tpdu.NewDecodeError("SmsStatusReport.mr", 1, tpdu.ErrUnderflow),
		},
		{
			"SmsStatusReport underflow ra",
			[]byte{0x02, 0x42},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
			},
			tpdu.NewDecodeError("SmsStatusReport.ra.addr", 2, tpdu.ErrUnderflow),
		},
		{
			"SmsStatusReport underflow scts",
			[]byte{0x02, 0x42, 0x04, 0x91, 0x36, 0x19},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			tpdu.NewDecodeError("SmsStatusReport.scts", 6, tpdu.ErrUnderflow),
		},
		{
			"SmsStatusReport underflow dt",
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			tpdu.NewDecodeError("SmsStatusReport.dt", 13, tpdu.ErrUnderflow),
		},
		{
			"SmsStatusReport non-integer scts",
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0xf1, 0x32, 0x20,
				0x05, 0x23,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS:       timestampFrom(0x51, 0x50, 0xf1, 0x32, 0x20, 0x05, 0x23),
			},
			tpdu.NewDecodeError("SmsStatusReport.dt", 13, tpdu.ErrUnderflow),
		},
		{
			"SmsStatusReport non-integer dt",
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0xc1, 0x32, 0x20, 0x05, 0x42,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: timestampFrom(0x51, 0x40, 0xc1, 0x32, 0x20, 0x05, 0x42),
			},
			tpdu.NewDecodeError("SmsStatusReport.st", 20, tpdu.ErrUnderflow),
		},
		{
			"SmsStatusReport underflow st",
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
			},
			tpdu.NewDecodeError("SmsStatusReport.st", 20, tpdu.ErrUnderflow),
		},
		{
			"SmsStatusReport underflow pid",
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab, 0x01,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				PI:         0x01,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
					time.FixedZone("SCTS", 8*3600))},
				DT: tpdu.Timestamp{Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
					time.FixedZone("SCTS", 6*3600))},
				ST: 0xab,
			},
			tpdu.NewDecodeError("SmsStatusReport.pid", 22, tpdu.ErrUnderflow),
		},
		{
			"SmsStatusReport underflow dcs",
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab, 0x02,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				PI:         0x02,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
				ST: 0xab,
			},
			tpdu.NewDecodeError("SmsStatusReport.dcs", 22, tpdu.ErrUnderflow),
		},
		{
			"SmsStatusReport underflow ud",
			[]byte{
				0x02, 0x42, 0x04, 0x91, 0x36, 0x19, 0x51, 0x50, 0x71, 0x32, 0x20,
				0x05, 0x23, 0x51, 0x40, 0x81, 0x32, 0x20, 0x05, 0x42, 0xab, 0x04,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x02,
				MR:         0x42,
				PI:         0x04,
				RA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
				DT: tpdu.Timestamp{
					Time: time.Date(2015, time.April, 18, 23, 02, 50, 0,
						time.FixedZone("SCTS", 6*3600)),
				},
				ST: 0xab,
			},
			tpdu.NewDecodeError("SmsStatusReport.ud.udl", 22, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmit haha",
			[]byte{
				0x01, 0x23, 0x04, 0x91, 0x36, 0x19, 0x34, 0x00, 0x08, 0xC8, 0x30,
				0x3A, 0x8C, 0x0E, 0xA3, 0xC3,
			},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
				PID:        0x34,
				UD:         []byte("Hahahaha"),
				MR:         0x23,
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			nil},
		{
			"SmsSubmit vp",
			[]byte{
				0x11, 0x23, 0x04, 0x91, 0x36, 0x19, 0x34, 0x00, 0x45, 0x08, 0xC8,
				0x30, 0x3A, 0x8C, 0x0E, 0xA3, 0xC3,
			},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x11,
				PID:        0x34,
				UD:         []byte("Hahahaha"),
				MR:         0x23,
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				VP: tpdu.ValidityPeriod{
					Format:   tpdu.VpfRelative,
					Duration: time.Duration(60 * 350 * 1000000000),
				},
			},
			nil,
		},
		{
			"SmsSubmit underflow mr",
			[]byte{0x01},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
			},
			tpdu.NewDecodeError("SmsSubmit.mr", 1, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmit underflow da",
			[]byte{0x01, 0x23},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
				MR:         0x23,
			},
			tpdu.NewDecodeError("SmsSubmit.da.addr", 2, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmit bad da",
			[]byte{0x01, 0x23, 0x04, 0x91, 0x36, 0xF9, 0x00},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
				MR:         0x23,
			},
			tpdu.NewDecodeError("SmsSubmit.da.addr", 4, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmit underflow pid",
			[]byte{0x01, 0x23, 0x04, 0x91, 0x36, 0x19},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
				MR:         0x23,
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			tpdu.NewDecodeError("SmsSubmit.pid", 6, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmit underflow dcs",
			[]byte{0x01, 0x23, 0x04, 0x91, 0x36, 0x19, 0x00},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
				MR:         0x23,
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			tpdu.NewDecodeError("SmsSubmit.dcs", 7, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmit underflow vp",
			[]byte{0x11, 0x23, 0x04, 0x91, 0x36, 0x19, 0x34, 0x00},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x11,
				PID:        0x34,
				MR:         0x23,
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			tpdu.NewDecodeError("SmsSubmit.vp", 8, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmit non-integer vp",
			[]byte{
				0x19, 0x23, 0x04, 0x91, 0x36, 0x19, 0x34, 0x00, 0x45, 0x08, 0xC8,
				0x30, 0x3A, 0x8C, 0x0E, 0xA3, 0xC3,
			},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x19,
				PID:        0x34,
				MR:         0x23,
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
				VP: tpdu.ValidityPeriod{
					Format: tpdu.VpfAbsolute,
					Time:   timestampFrom(0x45, 0x08, 0xC8, 0x30, 0x3A, 0x8C, 0x0E),
				},
			},
			// a TP-UDL of 163 septets exceeds the 140 octets of TS 23.040 3.1
			tpdu.NewDecodeError("SmsSubmit.ud.udl", 15, tpdu.ErrOverlength),
		},
		{
			"SmsSubmit underflow ud",
			[]byte{
				0x01, 0x23, 0x04, 0x91, 0x36, 0x19, 0x00, 0x00, 0x51, 0x50, 0x71,
				0x32, 0x20, 0x05, 0x23, 0x08,
			},
			tpdu.MO,
			tpdu.TPDU{
				Direction:  tpdu.MO,
				FirstOctet: 0x01,
				MR:         0x23,
				DA:         tpdu.Address{Addr: "6391", TOA: 0x91},
			},
			tpdu.NewDecodeError("SmsSubmit.ud.sm", 9, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmitReport minimal",
			[]byte{0x01, 0xc0, 0x00, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			nil,
		},
		{
			"SmsSubmitReport pid",
			[]byte{0x01, 0xc0, 0x01, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23, 0xab},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				PID:        0xab,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         tpdu.PiPID,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			nil,
		},
		{
			"SmsSubmitReport dcs",
			[]byte{0x01, 0xc0, 0x02, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23, 0x04},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				DCS:        0x04,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         tpdu.PiDCS,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			nil,
		},
		{
			"SmsSubmitReport ud",
			[]byte{
				0x01, 0xc0, 0x06, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23, 0x04,
				0x06, 0x72, 0x65, 0x70, 0x6f, 0x72, 0x74,
			},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				DCS:        0x04,
				UD:         []byte("report"),
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         0x06,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			nil,
		},
		{
			"SmsSubmitReport underflow fcs",
			[]byte{0x01},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				RPMessage:  tpdu.RPError,
				FirstOctet: 0x01,
			},
			tpdu.NewDecodeError("SmsSubmitReport.fcs", 1, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmitReport underflow pi",
			[]byte{0x01, 0xc0},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
			},
			tpdu.NewDecodeError("SmsSubmitReport.pi", 2, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmitReport underflow scts",
			[]byte{0x01, 0xc0, 0x00},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
			},
			tpdu.NewDecodeError("SmsSubmitReport.scts", 3, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmitReport non-integer scts",
			[]byte{0x01, 0xc0, 0x00, 0x51, 0x50, 0xf1, 0x32, 0x20, 0x05, 0x23},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				SCTS:       timestampFrom(0x51, 0x50, 0xf1, 0x32, 0x20, 0x05, 0x23),
			},
			nil,
		},
		{
			"SmsSubmitReport underflow pid",
			[]byte{0x01, 0xc0, 0x01, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         0x01,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600))},
			},
			tpdu.NewDecodeError("SmsSubmitReport.pid", 10, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmitReport underflow dcs",
			[]byte{0x01, 0xc0, 0x02, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         0x02,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600))},
			},
			tpdu.NewDecodeError("SmsSubmitReport.dcs", 10, tpdu.ErrUnderflow),
		},
		{
			"SmsSubmitReport underflow ud",
			[]byte{0x01, 0xc0, 0x06, 0x51, 0x50, 0x71, 0x32, 0x20, 0x05, 0x23, 0x04},
			tpdu.MT,
			tpdu.TPDU{
				Direction:  tpdu.MT,
				FirstOctet: 0x01,
				DCS:        0x04,
				RPMessage:  tpdu.RPError,
				FCS:        0xc0,
				PI:         0x06,
				SCTS: tpdu.Timestamp{
					Time: time.Date(2015, time.May, 17, 23, 02, 50, 0,
						time.FixedZone("SCTS", 8*3600)),
				},
			},
			tpdu.NewDecodeError("SmsSubmitReport.ud.udl", 11, tpdu.ErrUnderflow),
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			d := tpdu.TPDU{Direction: p.dirn, RPMessage: p.out.RPMessage}
			err := d.UnmarshalBinary(p.in)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, d)
		}
		t.Run(p.name, f)
	}
}

// unhex decodes a hex string, ignoring spaces.
func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	require.NoError(t, err)
	return b
}

// TestUnmarshalBinaryResetsReceiver checks that decoding into a TPDU that
// already holds a decoded TPDU gives the same result as decoding into a new
// TPDU with the same Direction, so no field of the earlier TPDU survives.
func TestUnmarshalBinaryResetsReceiver(t *testing.T) {
	patterns := []struct {
		name   string
		dirn   tpdu.Direction
		first  string
		second string
	}{
		{
			// TS 23.040 9.2.3.16: "If this field is zero, the TP-User-Data
			// field shall not be present."
			"deliver udl 0 after concatenated ucs2",
			tpdu.MT,
			"44 04 91 3619 00 08 51507132200523 08 050003070201 0041",
			"04 04 91 3619 00 00 51507132200523 00",
		},
		{
			"submit without vp after submit with vp",
			tpdu.MO,
			"11 23 04 91 3619 34 00 45 08 c8303a8c0ea3c3",
			"01 23 04 91 3619 00 00 00",
		},
		{
			"status report without pi after one with pi",
			tpdu.MT,
			"02 42 04 91 3619 51507132200523 51408132200542 ab 07 89 04 02 6869",
			"02 42 04 91 3619 51507132200523 51408132200542 ab",
		},
		{
			"partial decode after full decode",
			tpdu.MT,
			"44 04 91 3619 00 08 51507132200523 08 050003070201 0041",
			"04 04 91 3619 00",
		},
		{
			"unsupported type after full decode",
			tpdu.MO,
			"01 23 04 91 3619 34 00 08 c8303a8c0ea3c3",
			"03",
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			reused := tpdu.TPDU{Direction: p.dirn}
			require.NoError(t, reused.UnmarshalBinary(unhex(t, p.first)))
			rerr := reused.UnmarshalBinary(unhex(t, p.second))
			fresh := tpdu.TPDU{Direction: p.dirn}
			ferr := fresh.UnmarshalBinary(unhex(t, p.second))
			assert.Equal(t, ferr, rerr)
			assert.Equal(t, fresh, reused)
		}
		t.Run(p.name, f)
	}
}

// TestReservedMTI checks the handling of the Reserved TP-MTI value 11.
//
// TS 23.040 9.2.3.1: "If an MS receives a TPDU with a "Reserved" value in the
// TP-MTI it shall process the message as if it were an "SMS-DELIVER" but
// store the message exactly as received."
func TestReservedMTI(t *testing.T) {
	in := unhex(t, "07 04 91 3619 00 00 51507132200523 01 41")
	d := tpdu.TPDU{Direction: tpdu.MT}
	require.NoError(t, d.UnmarshalBinary(in))
	assert.Equal(t, tpdu.SmsDeliver, d.SmsType())
	assert.Equal(t, tpdu.MtReserved, d.MTI())
	assert.Equal(t, tpdu.FirstOctet(0x07), d.FirstOctet)
	assert.Equal(t, tpdu.Address{Addr: "6391", TOA: 0x91}, d.OA)
	assert.Equal(t, []byte{0x41}, []byte(d.UD))
	assert.Equal(t, 160, d.UDBlockSize())
	out, err := d.MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, in, out)

	// There is no such rule for the MO direction, where the SC receives it.
	m := tpdu.TPDU{Direction: tpdu.MO}
	err = m.UnmarshalBinary(in)
	assert.Equal(t, tpdu.NewDecodeError("tpdu.firstOctet", 0, tpdu.ErrUnsupportedSmsType(7)), err)
	// the TPDU is left partially decoded, so the first octet is kept.
	assert.Equal(t, tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x07}, m)
}

// TestReportRPAckDecode checks reports carried by an RP-ACK, which have no
// TP-FCS, decode with the default RP message, and marshal back to the same
// octets.
//
// TS 23.040 9.2.2.1a (ii) "SMS-DELIVER-REPORT for RP-ACK" and 9.2.2.2a (ii)
// "SMS-SUBMIT-REPORT for RP-ACK": TP-MTI, TP-UDHI; TP-PI; ... with no TP-FCS.
func TestReportRPAckDecode(t *testing.T) {
	scts := tpdu.Timestamp{
		Time: time.Date(2015, time.May, 17, 23, 02, 50, 0, time.FixedZone("SCTS", 8*3600)),
	}
	patterns := []struct {
		name string
		dirn tpdu.Direction
		in   string
		out  tpdu.TPDU
	}{
		{
			"deliver report pi 0",
			tpdu.MO,
			"00 00",
			tpdu.TPDU{Direction: tpdu.MO},
		},
		{
			"deliver report pid",
			tpdu.MO,
			"00 01 7f",
			tpdu.TPDU{Direction: tpdu.MO, PI: tpdu.PiPID, PID: 0x7f},
		},
		{
			"deliver report pid and dcs",
			tpdu.MO,
			"00 03 00 00",
			tpdu.TPDU{Direction: tpdu.MO, PI: tpdu.PiPID | tpdu.PiDCS},
		},
		{
			"deliver report ud",
			tpdu.MO,
			"00 07 00 00 02 e834",
			tpdu.TPDU{Direction: tpdu.MO, PI: tpdu.PiPID | tpdu.PiDCS | tpdu.PiUDL, UD: []byte("hi")},
		},
		{
			"submit report pi 0",
			tpdu.MT,
			"01 00 51507132200523",
			tpdu.TPDU{FirstOctet: 0x01, SCTS: scts},
		},
		{
			"submit report ud",
			tpdu.MT,
			"01 04 51507132200523 02 e834",
			tpdu.TPDU{FirstOctet: 0x01, PI: tpdu.PiUDL, SCTS: scts, UD: []byte("hi")},
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			in := unhex(t, p.in)
			d := tpdu.TPDU{Direction: p.dirn}
			require.NoError(t, d.UnmarshalBinary(in))
			assert.Equal(t, p.out, d)
			b, err := d.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, in, b)
		}
		t.Run(p.name, f)
	}
}

// TestReportRPErrorDecode checks reports carried by an RP-ERROR, which have a
// TP-FCS, round trip.
//
// TS 23.040 9.2.2.1a (i) and 9.2.2.2a (i): TP-MTI, TP-UDHI; TP-FCS; TP-PI...
func TestReportRPErrorDecode(t *testing.T) {
	patterns := []struct {
		name string
		dirn tpdu.Direction
		in   string
		fcs  byte
		pi   tpdu.PI
	}{
		{"deliver report", tpdu.MO, "00 d0 00", 0xd0, 0},
		{"deliver report pid", tpdu.MO, "00 ff 01 7f", 0xff, tpdu.PiPID},
		// a reserved FCS is decoded, and treated as "Unspecified error
		// cause" by the application, as per TS 23.040 9.2.3.22.
		{"deliver report reserved fcs", tpdu.MO, "00 00 00", 0x00, 0},
		{"submit report", tpdu.MT, "01 c0 00 51507132200523", 0xc0, 0},
		{"submit report ud", tpdu.MT, "01 c5 04 51507132200523 02 e834", 0xc5, tpdu.PiUDL},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			in := unhex(t, p.in)
			d := tpdu.TPDU{Direction: p.dirn, RPMessage: tpdu.RPError}
			require.NoError(t, d.UnmarshalBinary(in))
			assert.Equal(t, tpdu.RPError, d.RPMessage)
			assert.Equal(t, p.fcs, d.FCS)
			assert.Equal(t, p.pi, d.PI)
			b, err := d.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, in, b)
		}
		t.Run(p.name, f)
	}
}

func TestReportFCSMarshal(t *testing.T) {
	// An FCS cannot be carried in a report for RP-ACK.
	for _, st := range []tpdu.SmsType{tpdu.SmsDeliverReport, tpdu.SmsSubmitReport} {
		r, err := tpdu.New(st)
		require.NoError(t, err)
		r.FCS = 0xd0
		_, err = r.MarshalBinary()
		assert.Equal(t, tpdu.NewEncodeError(st.String()+".fcs", tpdu.ErrInvalid), err)
		// but it can in a report for RP-ERROR
		r.RPMessage = tpdu.RPError
		b, err := r.MarshalBinary()
		require.NoError(t, err)
		assert.Equal(t, byte(0xd0), b[1])
		// even if it is zero
		r.FCS = 0
		b, err = r.MarshalBinary()
		require.NoError(t, err)
		assert.Equal(t, byte(0x00), b[1])
		assert.Equal(t, byte(0x00), b[2])
	}
	// An invalid RPMessage is rejected, rather than guessed.
	r := tpdu.TPDU{Direction: tpdu.MO, RPMessage: 2}
	_, err := r.MarshalBinary()
	assert.Equal(t, tpdu.NewEncodeError("SmsDeliverReport.rp", tpdu.ErrInvalid), err)
	err = r.UnmarshalBinary([]byte{0x00, 0x00})
	assert.Equal(t, tpdu.NewDecodeError("SmsDeliverReport.rp", 1, tpdu.ErrInvalid), err)
	// but only matters for the reports
	r.FirstOctet = 0x01
	_, err = r.MarshalBinary()
	assert.NoError(t, err)
}

func TestRPMessageOption(t *testing.T) {
	r, err := tpdu.New(tpdu.SmsDeliverReport, tpdu.RPError)
	require.NoError(t, err)
	assert.Equal(t, tpdu.RPError, r.RPMessage)
	r, err = tpdu.New(tpdu.SmsDeliverReport, tpdu.RPError, tpdu.RPAck)
	require.NoError(t, err)
	assert.Equal(t, tpdu.RPAck, r.RPMessage)
	_, err = tpdu.New(tpdu.RPMessage(2))
	assert.Equal(t, tpdu.ErrInvalid, err)
	_, err = tpdu.New(tpdu.RPMessage(-1))
	assert.Equal(t, tpdu.ErrInvalid, err)
	assert.Equal(t, "RP-ACK", tpdu.RPAck.String())
	assert.Equal(t, "RP-ERROR", tpdu.RPError.String())
	assert.Equal(t, "Unknown", tpdu.RPMessage(2).String())
}

// srHead is the part of an SMS-STATUS-REPORT before the TP-PI.
const srHead = "02 42 04 91 3619 51507132200523 51408132200542 ab"

// TestPIExtension checks the TP-PI extension octets are read, and are not
// mistaken for the optional fields that follow the TP-PI.
//
// TS 23.040 9.2.3.27: "The most significant bit in octet 1 and any other
// TP-PI octets which may be added later is reserved as an extension bit which
// when set to a 1 shall indicate that another TP-PI octet follows immediately
// afterwards."
func TestPIExtension(t *testing.T) {
	patterns := []struct {
		name  string
		dirn  tpdu.Direction
		in    string
		pi    tpdu.PI
		piext []byte
		pid   byte
		ud    []byte
	}{
		{"deliver report", tpdu.MO, "00 81 00 7f", 0x81, []byte{0x00}, 0x7f, nil},
		{"deliver report chain", tpdu.MO, "00 81 80 80 00 7f", 0x81, []byte{0x80, 0x80, 0x00}, 0x7f, nil},
		{"submit report", tpdu.MT, "01 84 00 51507132200523 01 41", 0x84, []byte{0x00}, 0, []byte("A")},
		{"status report", tpdu.MT, srHead + " 84 00 01 41", 0x84, []byte{0x00}, 0, []byte("A")},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			in := unhex(t, p.in)
			d := tpdu.TPDU{Direction: p.dirn}
			require.NoError(t, d.UnmarshalBinary(in))
			assert.Equal(t, p.pi, d.PI)
			assert.Equal(t, p.piext, d.PIExt)
			assert.Equal(t, p.pid, d.PID)
			assert.Equal(t, p.ud, []byte(d.UD))
			b, err := d.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, in, b)
		}
		t.Run(p.name, f)
	}

	truncated := []struct {
		name string
		dirn tpdu.Direction
		in   string
		err  error
	}{
		{"deliver report", tpdu.MO, "00 81",
			tpdu.NewDecodeError("SmsDeliverReport.pi", 2, tpdu.ErrUnderflow)},
		{"deliver report chain", tpdu.MO, "00 81 80",
			tpdu.NewDecodeError("SmsDeliverReport.pi", 3, tpdu.ErrUnderflow)},
		{"submit report", tpdu.MT, "01 80",
			tpdu.NewDecodeError("SmsSubmitReport.pi", 2, tpdu.ErrUnderflow)},
		{"status report", tpdu.MT, srHead + " 84",
			tpdu.NewDecodeError("SmsStatusReport.pi", 22, tpdu.ErrUnderflow)},
	}
	for _, p := range truncated {
		f := func(t *testing.T) {
			d := tpdu.TPDU{Direction: p.dirn}
			err := d.UnmarshalBinary(unhex(t, p.in))
			assert.Equal(t, p.err, err)
		}
		t.Run("truncated "+p.name, f)
	}

	// The extension bits are derived from the PIExt on marshal.
	marshal := []struct {
		name string
		in   tpdu.TPDU
		out  string
	}{
		{"ext bit without ext octets",
			tpdu.TPDU{Direction: tpdu.MO, PI: 0x81, PID: 0x7f}, "00 01 7f"},
		{"ext octet",
			tpdu.TPDU{Direction: tpdu.MO, PI: 0x01, PIExt: []byte{0x00}, PID: 0x7f}, "00 81 00 7f"},
		{"ext octets",
			tpdu.TPDU{Direction: tpdu.MO, PI: 0x01, PIExt: []byte{0x80, 0x00}, PID: 0x7f}, "00 81 80 00 7f"},
		{"ext octets without ext bits",
			tpdu.TPDU{Direction: tpdu.MO, PI: 0x01, PIExt: []byte{0x00, 0x00}, PID: 0x7f}, "00 81 80 00 7f"},
		{"last ext octet with ext bit",
			tpdu.TPDU{Direction: tpdu.MO, PI: 0x01, PIExt: []byte{0x80}, PID: 0x7f}, "00 81 00 7f"},
		{"status report pi omitted",
			tpdu.TPDU{FirstOctet: 0x02, PI: 0x80, RA: tpdu.Address{Addr: "6391", TOA: 0x91}},
			"02 00 04 91 3619 00000000000000 00000000000000 00"},
		{"status report ext octet only",
			tpdu.TPDU{FirstOctet: 0x02, PIExt: []byte{0x00}, RA: tpdu.Address{Addr: "6391", TOA: 0x91}},
			"02 00 04 91 3619 00000000000000 00000000000000 00 80 00"},
	}
	for _, p := range marshal {
		f := func(t *testing.T) {
			b, err := p.in.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, unhex(t, p.out), b)
		}
		t.Run("marshal "+p.name, f)
	}
}

// TestStatusReportExplicitPI checks an SMS-STATUS-REPORT marshals its TP-PI
// if it was received with one, even one announcing no field, so it marshals
// back to the octets it came from, while one built without a TP-PI, or
// received without one, is marshalled without it.
//
// TS 23.040 9.2.2.3: the TP-PI is "Mandatory if any of the optional
// parameters following TP-PI is present, otherwise optional."
func TestStatusReportExplicitPI(t *testing.T) {
	for _, in := range []string{
		srHead,
		srHead + " 00",
		srHead + " 78",       // reserved bits only
		srHead + " 80 00",    // an extension octet announcing nothing
		srHead + " 04 01 41", // a TP-UD
	} {
		b := unhex(t, in)
		d := tpdu.TPDU{}
		require.NoError(t, d.UnmarshalBinary(b), in)
		out, err := d.MarshalBinary()
		require.NoError(t, err, in)
		assert.Equal(t, b, out, in)
		// Keeping the TP-PI does not keep the fields it announced.
		d.UD = []byte("B")
		out, err = d.MarshalBinary()
		require.NoError(t, err, in)
		r := tpdu.TPDU{}
		require.NoError(t, r.UnmarshalBinary(out), in)
		assert.Equal(t, []byte("B"), []byte(r.UD), in)
	}

	// The TP-PI is omitted from a status report built without one, and
	// from one decoded without one, whatever the TPDU held before.
	built := tpdu.TPDU{FirstOctet: 0x02, MR: 0x42, RA: tpdu.Address{Addr: "6391", TOA: 0x91}, ST: 0xab}
	out, err := built.MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, unhex(t, "02 42 04 91 3619 00000000000000 00000000000000 ab"), out)
	d := tpdu.TPDU{}
	require.NoError(t, d.UnmarshalBinary(unhex(t, srHead+" 00")))
	require.NoError(t, d.UnmarshalBinary(unhex(t, srHead)))
	out, err = d.MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, unhex(t, srHead), out)
}

// TestPIReservedBits checks that octets following the TP-UD are discarded
// when a reserved TP-PI bit is set, and rejected otherwise.
//
// TS 23.040 9.2.3.27: "If a Reserved bit is set to "1" then the receiving
// entity shall ignore the setting. The setting of this bit shall mean that
// additional information will follow the TP-User-Data, so a receiving entity
// shall discard any octets following the TP-User-Data."
func TestPIReservedBits(t *testing.T) {
	patterns := []struct {
		name string
		dirn tpdu.Direction
		in   string
		out  string // the octets without the discarded ones
		ud   []byte
	}{
		{"status report", tpdu.MT, srHead + " 0c 01 41 aa bb", srHead + " 0c 01 41", []byte("A")},
		{"status report bit 6", tpdu.MT, srHead + " 44 01 41 aa", srHead + " 44 01 41", []byte("A")},
		{"deliver report", tpdu.MO, "00 14 01 41 de ad", "00 14 01 41", []byte("A")},
		{"deliver report udl 0", tpdu.MO, "00 24 00 de ad", "00 24 00", nil},
		{"deliver report no ud", tpdu.MO, "00 08 de ad", "00 08", nil},
		{"submit report", tpdu.MT, "01 44 51507132200523 01 41 ff", "01 44 51507132200523 01 41", []byte("A")},
		{"reserved bit in ext octet", tpdu.MO, "00 80 01 de ad", "00 80 01", nil},
		{"reserved bit in ext octet with ud", tpdu.MO, "00 84 40 01 41 de ad", "00 84 40 01 41", []byte("A")},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			d := tpdu.TPDU{Direction: p.dirn}
			require.NoError(t, d.UnmarshalBinary(unhex(t, p.in)))
			assert.Equal(t, p.ud, []byte(d.UD))
			// the reserved bits are kept, but not the discarded octets.
			b, err := d.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, unhex(t, p.out), b)
		}
		t.Run(p.name, f)
	}

	// Without a reserved bit set, trailing octets are not expected.
	rejected := []struct {
		name string
		dirn tpdu.Direction
		in   string
		err  error
	}{
		{"deliver report after ud", tpdu.MO, "00 04 01 41 de",
			tpdu.NewDecodeError("SmsDeliverReport.ud", 4, tpdu.ErrOverlength)},
		{"deliver report after udl 0", tpdu.MO, "00 04 00 de",
			tpdu.NewDecodeError("SmsDeliverReport.ud", 3, tpdu.ErrOverlength)},
		{"deliver report without ud", tpdu.MO, "00 00 de",
			tpdu.NewDecodeError("SmsDeliverReport.ud", 2, tpdu.ErrOverlength)},
		{"deliver report after ext octet", tpdu.MO, "00 81 00 7f de",
			tpdu.NewDecodeError("SmsDeliverReport.ud", 4, tpdu.ErrOverlength)},
		{"submit report", tpdu.MT, "01 00 51507132200523 de",
			tpdu.NewDecodeError("SmsSubmitReport.ud", 9, tpdu.ErrOverlength)},
		{"status report", tpdu.MT, srHead + " 00 de",
			tpdu.NewDecodeError("SmsStatusReport.ud", 22, tpdu.ErrOverlength)},
		// TS 23.040 9.2.3.16: "If this field is zero, the TP-User-Data
		// field shall not be present."
		{"deliver udl 0", tpdu.MT, "04 04 91 3619 00 00 51507132200523 00 41",
			tpdu.NewDecodeError("SmsDeliver.ud", 15, tpdu.ErrOverlength)},
		{"submit udl 0", tpdu.MO, "01 23 04 91 3619 00 00 00 aa bb",
			tpdu.NewDecodeError("SmsSubmit.ud", 9, tpdu.ErrOverlength)},
		{"submit after ud", tpdu.MO, "01 23 04 91 3619 00 04 01 41 42",
			tpdu.NewDecodeError("SmsSubmit.ud", 10, tpdu.ErrOverlength)},
		{"command cdl 0", tpdu.MO, "02 42 00 00 00 00 00 00 aa",
			tpdu.NewDecodeError("SmsCommand.ud", 8, tpdu.ErrOverlength)},
	}
	for _, p := range rejected {
		f := func(t *testing.T) {
			d := tpdu.TPDU{Direction: p.dirn}
			err := d.UnmarshalBinary(unhex(t, p.in))
			assert.Equal(t, p.err, err)
			assert.ErrorIs(t, err, tpdu.ErrOverlength)
		}
		t.Run("reject "+p.name, f)
	}
}

// TestPIDCSAbsent checks the DCS is 0x00 when the TP-PI announces a TP-UDL
// but not a TP-DCS, whatever the TPDU held before.
//
// TS 23.040 9.2.3.27: "If the TP-UDL bit is set to "1" but the TP-DCS bit is
// set to "0" then the receiving entity shall for TP-DCS assume a value of
// 0x00, i.e. the 7bit default alphabet."
func TestPIDCSAbsent(t *testing.T) {
	patterns := []struct {
		name string
		dirn tpdu.Direction
		in   string
	}{
		{"deliver report", tpdu.MO, "00 04 02 c1 20"},
		{"submit report", tpdu.MT, "01 04 51507132200523 02 c1 20"},
		{"status report", tpdu.MT, srHead + " 04 02 c1 20"},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			d := tpdu.TPDU{Direction: p.dirn, DCS: tpdu.DcsUCS2Data, PID: 0x7f}
			require.NoError(t, d.UnmarshalBinary(unhex(t, p.in)))
			assert.Equal(t, tpdu.DCS(0), d.DCS)
			assert.Equal(t, byte(0), d.PID)
			assert.Equal(t, []byte("AA"), []byte(d.UD))
		}
		t.Run(p.name, f)
	}
}

// TestMarshalDerivesFlags checks MarshalBinary derives the flag bits from the
// fields they describe, so the octets are always consistent, and that
// unmarshalling them gives back the fields.
func TestMarshalDerivesFlags(t *testing.T) {
	da := tpdu.Address{Addr: "6391", TOA: 0x91}
	port := tpdu.UserDataHeader{{ID: 5, Data: []byte{0x0b, 0x84, 0x23, 0xf0}}}
	vp := tpdu.ValidityPeriod{}
	vp.SetRelative(100 * time.Minute)
	patterns := []struct {
		name string
		in   tpdu.TPDU
		out  string
	}{
		// TS 23.040 9.2.3.3: TP-VPF "0 0 TP-VP field not present, 1 0
		// TP-VP field present - relative format"
		{"submit vp without vpf",
			tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DA: da, VP: vp, UD: []byte("hi")},
			"11 00 04 91 3619 00 00 13 02 e834"},
		{"submit vpf without vp",
			tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x19, DA: da, UD: []byte("hi")},
			"01 00 04 91 3619 00 00 02 e834"},
		// TS 23.040 9.2.3.23: TP-UDHI "1 The beginning of the TP-UD field
		// contains a Header in addition to the short message."
		{"submit udh without udhi",
			tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DA: da, DCS: 0x04, UDH: port, UD: []byte("hi")},
			"41 00 04 91 3619 00 04 09 0605040b8423f0 6869"},
		{"submit udhi without udh",
			tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x41, DA: da, DCS: 0x04, UD: []byte("hi")},
			"01 00 04 91 3619 00 04 02 6869"},
		{"submit empty udh",
			tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DA: da, DCS: 0x04, UDH: tpdu.UserDataHeader{}, UD: []byte("ab")},
			"41 00 04 91 3619 00 04 03 00 6162"},
		{"submit empty udh 7bit",
			tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DA: da, UDH: tpdu.UserDataHeader{}, UD: []byte("hi")},
			"41 00 04 91 3619 00 00 04 00 00 3a 0d"},
		{"submit udhi without udh or ud is kept",
			tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x41, DA: da},
			"41 00 04 91 3619 00 00 00"},
		{"deliver udh without udhi",
			tpdu.TPDU{FirstOctet: 0x04, OA: da, DCS: 0x04, UDH: port},
			"44 04 91 3619 00 04 00000000000000 07 0605040b8423f0"},
		// TS 23.040 9.2.3.27: "If the TP-UDL bit is set to "1" but the
		// TP-DCS bit is set to "0" then the receiving entity shall for
		// TP-DCS assume a value of 0x00"
		{"deliver report dcs without pi dcs",
			tpdu.TPDU{Direction: tpdu.MO, PI: tpdu.PiUDL, DCS: 0x04, UD: []byte("hi")},
			"00 06 04 02 6869"},
		{"deliver report ud without pi udl",
			tpdu.TPDU{Direction: tpdu.MO, UD: []byte("hi")},
			"00 04 02 e834"},
		{"deliver report udh without pi udl or udhi",
			tpdu.TPDU{Direction: tpdu.MO, DCS: 0x04, UDH: port},
			"40 06 04 07 0605040b8423f0"},
		{"deliver report pid without pi pid",
			tpdu.TPDU{Direction: tpdu.MO, PID: 0x7f},
			"00 01 7f"},
		{"deliver report pi bits with zero fields are kept",
			tpdu.TPDU{Direction: tpdu.MO, PI: tpdu.PiPID | tpdu.PiDCS | tpdu.PiUDL},
			"00 07 00 00 00"},
		{"submit report ucs2 without pi dcs",
			tpdu.TPDU{FirstOctet: 0x01, DCS: 0x08, UD: []byte{0x00, 0x41}},
			"01 06 00000000000000 08 02 0041"},
		{"status report dcs without pi",
			tpdu.TPDU{FirstOctet: 0x02, RA: da, DCS: 0x04, UD: []byte("hi")},
			"02 00 04 91 3619 00000000000000 00000000000000 00 06 04 02 6869"},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			in := p.in
			b, err := in.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, unhex(t, p.out), b)
			// the TPDU itself is unchanged
			assert.Equal(t, p.in, in)
			d := tpdu.TPDU{Direction: p.in.Direction}
			require.NoError(t, d.UnmarshalBinary(b))
			assert.Equal(t, p.in.UDH, d.UDH)
			assert.Equal(t, p.in.UD, d.UD)
			assert.Equal(t, p.in.DCS, d.DCS)
			assert.Equal(t, p.in.PID, d.PID)
			assert.Equal(t, p.in.VP, d.VP)
		}
		t.Run(p.name, f)
	}
}

// TestSegmentReportUD checks the UD of a report built by Segment is
// marshalled.
func TestSegmentReportUD(t *testing.T) {
	tmpl := tpdu.TPDU{Direction: tpdu.MO, RPMessage: tpdu.RPError, FCS: 0xd0}
	pdus, err := tmpl.Segment([]byte("hi"))
	require.NoError(t, err)
	require.Len(t, pdus, 1)
	b, err := pdus[0].MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, unhex(t, "00 d0 04 02 e834"), b)

	tmpl.DCS = tpdu.DcsUCS2Data
	pdus, err = tmpl.Segment([]byte{0x00, 0x41})
	require.NoError(t, err)
	require.Len(t, pdus, 1)
	b, err = pdus[0].MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, unhex(t, "00 d0 06 08 02 0041"), b)
}

// TestUDHIRoundTrip checks a received TPDU with an empty UDH, a TP-UDHL of 0,
// marshals back to the octets it came from.
//
// TS 23.040 9.2.3.24: "Length of User Data Header 1 octet"
func TestUDHIRoundTrip(t *testing.T) {
	for _, in := range []string{
		"41 00 04 91 3619 00 04 03 00 6162",        // 8-bit
		"41 00 04 91 3619 00 08 03 00 0041",        // UCS2
		"41 00 04 91 3619 00 00 04 00 00 3a 0d",    // 7-bit, 6 fill bits
		"41 00 04 91 3619 00 04 00",                // TP-UDHI with no TP-UD
		"41 00 04 91 3619 00 00 02 00 00",          // 7-bit, header only
		"40 04 91 3619 00 04 51507132200523 01 00", // header only deliver
	} {
		b := unhex(t, in)
		d := tpdu.TPDU{Direction: tpdu.MO}
		if b[0]&0x03 == 0 {
			d.Direction = tpdu.MT
		}
		require.NoError(t, d.UnmarshalBinary(b), in)
		out, err := d.MarshalBinary()
		require.NoError(t, err, in)
		assert.Equal(t, b, out, in)
	}
}

// TestCommandData checks the TP-CD of an SMS-COMMAND, which counts octets
// whatever the DCS, which a command does not have, and may hold a header.
//
// TS 23.040 9.2.2.4: TP-UDHI "Parameter indicating that the TP-CD field
// contains a Header"; 9.2.3.20: "The TP-Command-Data-Length field is used to
// indicate the number of octets contained within the TP-Command-Data field."
func TestCommandData(t *testing.T) {
	patterns := []struct {
		name string
		in   string
		udh  tpdu.UserDataHeader
		ud   []byte
	}{
		{"concat header", "42 42 00 00 34 04 91 3619 07 05 00 03 01 02 01 41",
			tpdu.UserDataHeader{{ID: 0, Data: []byte{1, 2, 1}}}, []byte("A")},
		{"port header", "42 01 00 01 02 04 91 2143 08 06 05 04 0b 84 23 f0 78",
			tpdu.UserDataHeader{{ID: 5, Data: []byte{0x0b, 0x84, 0x23, 0xf0}}}, []byte{0x78}},
		{"header only", "42 01 00 01 02 04 91 2143 06 05 00 03 01 02 01",
			tpdu.UserDataHeader{{ID: 0, Data: []byte{1, 2, 1}}}, nil},
		{"empty header", "42 01 00 01 02 04 91 2143 02 00 78",
			tpdu.UserDataHeader{}, []byte{0x78}},
		{"udhi without cd", "42 01 00 01 02 04 91 2143 00", nil, nil},
		{"octets above 7f", "02 01 00 01 02 04 91 2143 03 80 ff 7f",
			nil, []byte{0x80, 0xff, 0x7f}},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			in := unhex(t, p.in)
			d := tpdu.TPDU{Direction: tpdu.MO}
			require.NoError(t, d.UnmarshalBinary(in))
			assert.Equal(t, tpdu.SmsCommand, d.SmsType())
			assert.Equal(t, p.udh, d.UDH)
			assert.Equal(t, p.ud, []byte(d.UD))
			// a command has no TP-DCS
			assert.Equal(t, tpdu.DCS(0), d.DCS)
			b, err := d.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, in, b)
		}
		t.Run(p.name, f)
	}

	// A command built with SetUDH, whose DCS is 0, is not packed as septets.
	c, err := tpdu.New(tpdu.SmsCommand)
	require.NoError(t, err)
	c.SetUDH(tpdu.UserDataHeader{{ID: 0, Data: []byte{1, 2, 1}}})
	c.UD = []byte("xy")
	b, err := c.MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, unhex(t, "42 00 00 00 00 00 80 08 05 00 03 01 02 01 7879"), b)
	d := tpdu.TPDU{Direction: tpdu.MO}
	require.NoError(t, d.UnmarshalBinary(b))
	assert.Equal(t, c.UDH, d.UDH)
	assert.Equal(t, c.UD, d.UD)

	// The TP-UDHI is derived from the UDH, as for the other types.
	c = &tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x02,
		UDH: tpdu.UserDataHeader{{ID: 0, Data: []byte{1, 2, 1}}}, UD: []byte("xy")}
	b, err = c.MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, unhex(t, "42 00 00 00 00 00 00 08 05 00 03 01 02 01 7879"), b)
	c = &tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x42, UD: []byte("xy")}
	b, err = c.MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, unhex(t, "02 00 00 00 00 00 00 02 7879"), b)

	// UDBlockSize counts octets.
	c, err = tpdu.New(tpdu.SmsCommand)
	require.NoError(t, err)
	bs := c.UDBlockSize()
	c.SetUDH(tpdu.UserDataHeader{{ID: 0, Data: []byte{1, 2, 1}}})
	assert.Equal(t, bs-6, c.UDBlockSize())
	c.DCS = tpdu.DcsUCS2Data
	assert.Equal(t, bs-6, c.UDBlockSize())
}

// dlvHead is the part of an SMS-DELIVER before the TP-DCS.
const dlvHead = "04 04 91 3619 00"

// dlvHeadUDHI is dlvHead with the TP-UDHI set.
const dlvHeadUDHI = "44 04 91 3619 00"

// scts is an SCTS for the test vectors.
const scts = "51507132200523"

// TestCompressedUserData checks the TP-UDL of compressed user data counts
// octets, and the octets are kept as they are, whatever the alphabet.
//
// TS 23.040 9.2.3.16: "If the TP-User-Data is coded using compressed GSM 7
// bit default alphabet or compressed 8 bit data or compressed UCS2 [24] data,
// the TP-User-Data-Length field gives an integer representation of the number
// of octets after compression within the TP-User-Data field to follow. If a
// TP-User-Data-Header field is present, then the TP-User-Data-Length value is
// the sum of the number of uncompressed octets in the TP-User-Data-Header
// field and the number of octets in the compressed TP-User-Data field which
// follows."
func TestCompressedUserData(t *testing.T) {
	patterns := []struct {
		name string
		in   string
		udh  tpdu.UserDataHeader
		ud   []byte
	}{
		{"gsm7", dlvHead + " 20 " + scts + " 08 0102030405060708", nil,
			[]byte{1, 2, 3, 4, 5, 6, 7, 8}},
		{"gsm7 octets above 7f", dlvHead + " 20 " + scts + " 03 80ff7f", nil,
			[]byte{0x80, 0xff, 0x7f}},
		{"ucs2 odd length", dlvHead + " 28 " + scts + " 03 010203", nil,
			[]byte{1, 2, 3}},
		{"8bit", dlvHead + " 24 " + scts + " 02 0102", nil, []byte{1, 2}},
		{"marked for deletion gsm7", dlvHead + " 60 " + scts + " 02 0102", nil, []byte{1, 2}},
		{"class 1", dlvHead + " 31 " + scts + " 02 0102", nil, []byte{1, 2}},
		{"with udh", dlvHeadUDHI + " 20 " + scts + " 09 050003010201 81ff01",
			tpdu.UserDataHeader{{ID: 0, Data: []byte{1, 2, 1}}}, []byte{0x81, 0xff, 0x01}},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			in := unhex(t, p.in)
			d := tpdu.TPDU{}
			require.NoError(t, d.UnmarshalBinary(in))
			require.True(t, d.DCS.Compressed())
			assert.Equal(t, p.udh, d.UDH)
			assert.Equal(t, p.ud, []byte(d.UD))
			b, err := d.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, in, b)
		}
		t.Run(p.name, f)
	}
	// a UDL of 8 octets, not septets, so 7 octets is too few.
	d := tpdu.TPDU{}
	err := d.UnmarshalBinary(unhex(t, dlvHead+" 20 "+scts+" 08 01020304050607"))
	assert.ErrorIs(t, err, tpdu.ErrUnderflow)

	// the block size is in octets
	s := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: 0x20}
	assert.Equal(t, 140, s.UDBlockSize())
	s.DCS = 0x28
	assert.Equal(t, 140, s.UDBlockSize())
}

// TestHeaderOnly7BitUDL checks the TP-UDL of 7-bit user data with a header
// and no text counts the septets of the header, including its fill bits, and
// that the octets it announces are present.
//
// TS 23.040 9.2.3.16: "If a TP-User-Data-Header field is present, then the
// TP-User-Data-Length value is the sum of the number of septets in the
// TP-User-Data-Header field (including any padding) and the number of septets
// in the TP-User-Data field which follows."
func TestHeaderOnly7BitUDL(t *testing.T) {
	patterns := []struct {
		name string
		udh  tpdu.UserDataHeader
		ud   string // TP-UDL and TP-UD
	}{
		{"empty udh", tpdu.UserDataHeader{}, "02 00 00"},
		{"4 octets", tpdu.UserDataHeader{{ID: 1, Data: []byte{1}}}, "05 03 01 01 01 00"},
		{"5 octets", tpdu.UserDataHeader{{ID: 1, Data: []byte{0x80, 1}}}, "06 04 01 02 80 01 00"},
		{"6 octets", tpdu.UserDataHeader{{ID: 0, Data: []byte{1, 2, 1}}}, "07 05 00 03 01 02 01 00"},
		{"7 octets", tpdu.UserDataHeader{{ID: 8, Data: []byte{0, 1, 2, 1}}}, "08 06 08 04 00 01 02 01"},
		{"8 octets", tpdu.UserDataHeader{{ID: 1, Data: []byte{1, 2, 3, 4, 5}}}, "0a 07 01 05 01 02 03 04 05 00"},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			s := tpdu.TPDU{FirstOctet: 0x04, OA: tpdu.Address{Addr: "6391", TOA: 0x91}}
			s.SetUDH(p.udh)
			b, err := s.MarshalBinary()
			require.NoError(t, err)
			want := unhex(t, dlvHeadUDHI+" 00 00000000000000 "+p.ud)
			assert.Equal(t, want, b)
			d := tpdu.TPDU{}
			require.NoError(t, d.UnmarshalBinary(b))
			assert.Equal(t, p.udh, d.UDH)
			assert.Nil(t, d.UD)
			out, err := d.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, want, out)
		}
		t.Run(p.name, f)
	}

	// The undercounted form, UDL=6 for a 6 octet header, which the header
	// alone overruns, is still accepted, but marshals to the counted form.
	in := unhex(t, "41 00 04 91 3619 00 00 06 05 00 03 01 02 01")
	d := tpdu.TPDU{Direction: tpdu.MO}
	require.NoError(t, d.UnmarshalBinary(in))
	assert.Equal(t, tpdu.UserDataHeader{{ID: 0, Data: []byte{1, 2, 1}}}, d.UDH)
	assert.Nil(t, d.UD)
	b, err := d.MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, unhex(t, "41 00 04 91 3619 00 00 07 05 00 03 01 02 01 00"), b)
}

// TestSpareBits7Bit checks non-zero bits after the last septet are ignored,
// whether 1 to 6 bits, or a whole spare septet, are left in the last octet.
//
// TS 23.040 9.2.2.1: "Any unused bits shall be set to zero by the sending
// entity and shall be ignored by the receiving entity."
func TestSpareBits7Bit(t *testing.T) {
	patterns := []struct {
		name string
		in   string
		ud   string
		udh  tpdu.UserDataHeader
	}{
		// 7 septets leave 7 spare bits, here CR as some senders pad
		{"7 spare bits", dlvHead + " 00 " + scts + " 07 edf27c1e3e971b", "message", nil},
		{"7 spare bits all set", dlvHead + " 00 " + scts + " 07 edf27c1e3e97ff", "message", nil},
		{"2 spare bits", dlvHead + " 00 " + scts + " 02 41e1", "AB", nil},
		{"after header", dlvHeadUDHI + " 00 " + scts + " 0e 050003010203 dae5f93c7c2eff", "message",
			tpdu.UserDataHeader{{ID: 0, Data: []byte{1, 2, 3}}}},
		{"spare septet after header", dlvHeadUDHI + " 00 " + scts + " 0f 050003010203 dae5f93c7c2ecfff", "messages",
			tpdu.UserDataHeader{{ID: 0, Data: []byte{1, 2, 3}}}},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			d := tpdu.TPDU{}
			require.NoError(t, d.UnmarshalBinary(unhex(t, p.in)))
			assert.Equal(t, []byte(p.ud), []byte(d.UD))
			assert.Equal(t, p.udh, d.UDH)
		}
		t.Run(p.name, f)
	}
}

// TestHighBitSeptets checks UD holding a byte above 0x7f, which is not a
// septet, cannot be marshalled with a 7-bit DCS, rather than its eighth bit
// corrupting the next septet.
//
// TS 23.038 6.1.2.1.1: characters are 7 bit, packed "by completing the
// octets with zeros on the left".
func TestHighBitSeptets(t *testing.T) {
	ud := []byte("caf\xc3\xa9")
	for _, p := range []tpdu.TPDU{
		{Direction: tpdu.MO, FirstOctet: 0x01, UD: ud},
		{FirstOctet: 0x00, UD: ud},
		{Direction: tpdu.MO, UD: ud},
		{FirstOctet: 0x02, UD: ud},
		{FirstOctet: 0x01, UD: ud, DCS: 0x80}, // reserved, so 7 bit
	} {
		_, err := p.MarshalBinary()
		var ise gsm7.ErrInvalidSeptet
		require.ErrorAs(t, err, &ise, p.SmsType().String())
		assert.Equal(t, gsm7.ErrInvalidSeptet{Offset: 3, Septet: 0xc3}, ise)
		assert.Equal(t, p.SmsType().String()+".ud.sm", err.(tpdu.EncodeError).Field)
	}
	// but they are octets in 8 bit data
	p := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, UD: ud, DCS: tpdu.Dcs8BitData}
	_, err := p.MarshalBinary()
	assert.NoError(t, err)
}

// TestReservedDCS checks a TPDU with a reserved coding group in its DCS
// decodes as GSM 7 bit, and marshals back as received.
//
// TS 23.038 4: "Any reserved codings shall be assumed to be the GSM 7 bit
// default alphabet (the same as codepoint 00000000) by a receiving entity."
func TestReservedDCS(t *testing.T) {
	for _, in := range []string{
		dlvHead + " 80 " + scts + " 08 c8303a8c0ea3c3",
		dlvHead + " 9f " + scts + " 08 c8303a8c0ea3c3",
		dlvHead + " a0 " + scts + " 08 c8303a8c0ea3c3",
		dlvHead + " bf " + scts + " 08 c8303a8c0ea3c3",
		dlvHead + " 0c " + scts + " 08 c8303a8c0ea3c3", // reserved alphabet
		dlvHeadUDHI + " 80 " + scts + " 0f 050003010203 906174181d468701",
	} {
		b := unhex(t, in)
		d := tpdu.TPDU{}
		require.NoError(t, d.UnmarshalBinary(b), in)
		assert.Equal(t, []byte("Hahahaha"), []byte(d.UD), in)
		out, err := d.MarshalBinary()
		require.NoError(t, err, in)
		assert.Equal(t, b, out, in)
	}
}

// udMaxPattern is a TPDU and the most octets of TP-UD, or TP-CD, it can hold.
type udMaxPattern struct {
	name string
	pdu  tpdu.TPDU
	max  int
}

// udMaxPatterns returns the maximum TP-UD octets of each TPDU type.
//
// TS 23.040 3.1: "The text messages to be transferred by means of the SM MT
// or SM MO contain up to 140 octets." 9.2.2.1a: TP-UD "0 to 158" for RP-ERROR
// and "0 to 159" for RP-ACK. 9.2.2.2a: "0 to 151" and "0 to 152". 9.2.2.3:
// "0 to 143 ... The maximum guaranteed length of TP-UD is 131 octets. In
// order to achieve the maximum stated above (143 octets), the TP-RA field
// must have a length of 2 octets and TP-PID and TP-DCS must not be present."
// 9.2.2.4: TP-CD "0 to 156 ... The maximum guaranteed length of TP-CD is 146
// octets. In order to achieve the maximum stated above (156 octets), the
// TP-DA field must have a length of 2 octets."
func udMaxPatterns() []udMaxPattern {
	addr4 := tpdu.Address{Addr: "6391", TOA: 0x91}                  // 4 octets
	addr12 := tpdu.Address{Addr: "12345678901234567890", TOA: 0x91} // 12 octets
	return []udMaxPattern{
		{"submit", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DA: addr12}, 140},
		{"deliver", tpdu.TPDU{FirstOctet: 0x00, OA: addr12}, 140},
		{"deliver report rp-ack", tpdu.TPDU{Direction: tpdu.MO}, 159},
		{"deliver report rp-error", tpdu.TPDU{Direction: tpdu.MO, RPMessage: tpdu.RPError}, 158},
		{"deliver report pid", tpdu.TPDU{Direction: tpdu.MO, PID: 0x7f}, 159},
		{"deliver report ext pi", tpdu.TPDU{Direction: tpdu.MO, PIExt: []byte{0}}, 158},
		{"submit report rp-ack", tpdu.TPDU{FirstOctet: 0x01}, 152},
		{"submit report rp-error", tpdu.TPDU{FirstOctet: 0x01, RPMessage: tpdu.RPError}, 151},
		{"status report", tpdu.TPDU{FirstOctet: 0x02}, 143},
		{"status report ra 4", tpdu.TPDU{FirstOctet: 0x02, RA: addr4}, 141},
		{"status report ra 12", tpdu.TPDU{FirstOctet: 0x02, RA: addr12}, 133},
		{"status report pid", tpdu.TPDU{FirstOctet: 0x02, PID: 0x7f}, 142},
		{"status report pi pid", tpdu.TPDU{FirstOctet: 0x02, PI: tpdu.PiPID}, 142},
		{"status report ra 12 pid", tpdu.TPDU{FirstOctet: 0x02, RA: addr12, PID: 1}, 132},
		{"command", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x02}, 156},
		{"command da 4", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x02, DA: addr4}, 154},
		{"command da 12", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x02, DA: addr12}, 146},
	}
}

// TestUDMaximaOnMarshal checks MarshalBinary rejects TP-UD, or TP-CD, longer
// than the TPDU type allows, rather than writing an oversized TPDU or
// wrapping the length octet, and that UDBlockSize gives exactly the room left.
func TestUDMaximaOnMarshal(t *testing.T) {
	codings := []struct {
		name string
		dcs  tpdu.DCS
		unit int // octets per UD byte, as 7-bit UD is septets
	}{
		{"7bit", 0x00, 7},
		{"8bit", 0x04, 8},
		{"ucs2", 0x08, 8},
		{"compressed", 0x20, 8},
	}
	headers := []tpdu.UserDataHeader{
		nil,
		{},
		{{ID: 0, Data: []byte{1, 2, 1}}},
		{{ID: 1, Data: []byte{1, 2, 3, 4, 5, 6}}},
	}
	for _, p := range udMaxPatterns() {
		for _, c := range codings {
			for _, udh := range headers {
				pdu := p.pdu
				pdu.UDH = udh
				if pdu.SmsType() == tpdu.SmsCommand {
					if c.dcs != 0 {
						continue // a command has no DCS
					}
				} else {
					pdu.DCS = c.dcs
				}
				name := fmt.Sprintf("%s %s udh %d", p.name, c.name, len(udh))
				bs := pdu.UDBlockSize()
				pdu.UD = make([]byte, bs)
				b, err := pdu.MarshalBinary()
				require.NoError(t, err, name)
				assert.LessOrEqual(t, len(b), 164, name)
				// the TP-UD takes at most the maximum, and no more than a
				// unit less, other than for a UCS2 character.
				d := tpdu.TPDU{Direction: pdu.Direction, RPMessage: pdu.RPMessage}
				require.NoError(t, d.UnmarshalBinary(b), name)
				udo := udOctets(pdu)
				max := p.max
				if pdu.SmsType() == tpdu.SmsStatusReport && pdu.DCS != 0 {
					max-- // the TP-DCS is present
				}
				assert.LessOrEqual(t, udo, max, name)
				slack := 1
				if pdu.SmsType() != tpdu.SmsCommand && pdu.DCS == 0x08 {
					slack = 2
				}
				assert.Greater(t, udo, max-slack, name)
				// one more is too many
				pdu.UD = make([]byte, bs+1)
				if c.dcs == 0x08 && pdu.SmsType() != tpdu.SmsCommand {
					pdu.UD = make([]byte, bs+2)
				}
				_, err = pdu.MarshalBinary()
				assert.ErrorIs(t, err, tpdu.ErrOverlength, name)
			}
		}
	}

	// An address that cannot be marshalled is taken to be the longest.
	bad := tpdu.TPDU{FirstOctet: 0x02, RA: tpdu.Address{Addr: "12d4", TOA: 0x91}}
	long := tpdu.TPDU{FirstOctet: 0x02, RA: tpdu.Address{Addr: "12345678901234567890", TOA: 0x91}}
	assert.Equal(t, long.UDBlockSize(), bad.UDBlockSize())

	// A length that does not fit in the length octet is rejected rather
	// than wrapped.
	s := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: 0x04, UD: make([]byte, 300)}
	_, err := s.MarshalBinary()
	assert.Equal(t, tpdu.NewEncodeError("SmsSubmit.ud", tpdu.ErrOverlength), err)
	s = tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, UD: make([]byte, 300)}
	_, err = s.MarshalBinary()
	assert.Equal(t, tpdu.NewEncodeError("SmsSubmit.ud", tpdu.ErrOverlength), err)
	c := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x02, UD: make([]byte, 300)}
	_, err = c.MarshalBinary()
	assert.Equal(t, tpdu.NewEncodeError("SmsCommand.ud", tpdu.ErrOverlength), err)
}

// TestLongPIExtension checks the TP-PI extension octets of a report are
// bounded by the TPDU: "The Short Message is of variable length, 6-164
// octets" (3GPP TS 27.005 Section 2.5.2.6), so those of an RP-ERROR
// SMS-DELIVER-REPORT can take at most its 158 octets of TP-UD room.
func TestLongPIExtension(t *testing.T) {
	// first octet, TP-FCS, a TP-PI with every bit set, n extension octets
	// of which the last has no extension bit, then TP-PID, TP-DCS and a
	// TP-UDL of 0.
	report := func(n int) []byte {
		b := []byte{0x00, 0x80, 0xff}
		b = append(b, bytes.Repeat([]byte{0xff}, n-1)...)
		b = append(b, 0x7f)
		return append(b, 0x00, 0x04, 0x00)
	}
	in := report(158)
	require.Len(t, in, 164)
	d := tpdu.TPDU{Direction: tpdu.MO, RPMessage: tpdu.RPError}
	require.NoError(t, d.UnmarshalBinary(in))
	assert.Len(t, d.PIExt, 158)
	b, err := d.MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, in, b)
	assert.Equal(t, 0, d.UDBlockSize())
	// but not a TP-UD with an octet
	d.UD = []byte{1}
	_, err = d.MarshalBinary()
	assert.ErrorIs(t, err, tpdu.ErrOverlength)
	err = d.UnmarshalBinary(append(in[:len(in)-1], 0x01, 0x41))
	assert.ErrorIs(t, err, tpdu.ErrOverlength)

	// nor one more extension octet, even with no TP-UD
	in = report(159)
	err = d.UnmarshalBinary(in)
	assert.ErrorIs(t, err, tpdu.ErrOverlength)
	d = tpdu.TPDU{Direction: tpdu.MO, RPMessage: tpdu.RPError, FCS: 0x80, PIExt: bytes.Repeat([]byte{0x80}, 159)}
	d.PIExt[len(d.PIExt)-1] = 0
	_ = d.SetSmsType(tpdu.SmsDeliverReport)
	_, err = d.MarshalBinary()
	assert.ErrorIs(t, err, tpdu.ErrOverlength)
	d.PIExt = d.PIExt[1:]
	_, err = d.MarshalBinary()
	assert.NoError(t, err)
}

// udOctets returns the number of octets of the TP-UD, or TP-CD, of the TPDU,
// or -1 if it cannot be marshalled.
func udOctets(t tpdu.TPDU) int {
	// with PiUDL a report has a TP-UDL, and a TP-PI, even with no UD.
	t.PI |= tpdu.PiUDL
	b, err := t.MarshalBinary()
	if err != nil {
		return -1
	}
	t.UD, t.UDH = nil, nil
	e, err := t.MarshalBinary()
	if err != nil {
		return -1
	}
	return len(b) - len(e)
}

// TestUDMaximaOnUnmarshal checks UnmarshalBinary rejects TP-UD, or TP-CD,
// longer than the TPDU type allows, so every TPDU it accepts can be
// marshalled again.
func TestUDMaximaOnUnmarshal(t *testing.T) {
	patterns := []struct {
		name string
		dirn tpdu.Direction
		ok   string
		bad  string
	}{
		{"deliver 8bit", tpdu.MT,
			dlvHead + " 04 " + scts + " 8c " + strings.Repeat("41", 140),
			dlvHead + " 04 " + scts + " 8d " + strings.Repeat("41", 141)},
		{"deliver 7bit", tpdu.MT,
			dlvHead + " 00 " + scts + " a0 " + strings.Repeat("00", 140),
			dlvHead + " 00 " + scts + " a1 " + strings.Repeat("00", 141)},
		{"submit", tpdu.MO,
			"01 00 00 91 00 04 8c " + strings.Repeat("41", 140),
			"01 00 00 91 00 04 8d " + strings.Repeat("41", 141)},
		{"status report", tpdu.MT,
			"02 00 00 91 00000000000000 00000000000000 00 04 a3 " + strings.Repeat("00", 143),
			"02 00 00 91 00000000000000 00000000000000 00 04 a4 " + strings.Repeat("00", 144)},
		{"status report dcs", tpdu.MT,
			"02 00 00 91 00000000000000 00000000000000 00 06 04 8e " + strings.Repeat("00", 142),
			"02 00 00 91 00000000000000 00000000000000 00 06 04 8f " + strings.Repeat("00", 143)},
		{"command", tpdu.MO,
			"02 00 00 00 00 00 91 9c " + strings.Repeat("00", 156),
			"02 00 00 00 00 00 91 9d " + strings.Repeat("00", 157)},
		{"deliver report", tpdu.MO,
			"00 04 b5 " + strings.Repeat("00", 159),
			"00 04 b6 " + strings.Repeat("00", 160)},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			d := tpdu.TPDU{Direction: p.dirn}
			in := unhex(t, p.ok)
			require.NoError(t, d.UnmarshalBinary(in))
			b, err := d.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, in, b)
			err = d.UnmarshalBinary(unhex(t, p.bad))
			assert.ErrorIs(t, err, tpdu.ErrOverlength)
		}
		t.Run(p.name, f)
	}
}

// segmentWithin calls Segment, and fails the test if it panics or does not
// return within a few seconds, rather than hanging the test run.
func segmentWithin(t *testing.T, p tpdu.TPDU, msg []byte, options ...tpdu.SegmentationOption) ([]tpdu.TPDU, error) {
	t.Helper()
	type result struct {
		pdus  []tpdu.TPDU
		err   error
		panic interface{}
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{panic: r}
			}
		}()
		pdus, err := p.Segment(msg, options...)
		done <- result{pdus: pdus, err: err}
	}()
	select {
	case r := <-done:
		require.Nil(t, r.panic, "Segment panicked")
		return r.pdus, r.err
	case <-time.After(5 * time.Second):
		t.Fatal("Segment did not return")
		return nil, nil
	}
}

// checkSegments checks each segment marshals, and holds no more UD than its
// UDBlockSize, and that the segments carry consistent concatenation IEs.
func checkSegments(t *testing.T, pdus []tpdu.TPDU, name string) {
	t.Helper()
	for i, p := range pdus {
		_, err := p.MarshalBinary()
		require.NoError(t, err, "%s segment %d", name, i)
		assert.LessOrEqual(t, len(p.UD), p.UDBlockSize(), "%s segment %d", name, i)
		if len(pdus) == 1 {
			continue
		}
		ci, ok := p.ConcatInfo()
		require.True(t, ok, "%s segment %d", name, i)
		assert.Equal(t, len(pdus), ci.Total, "%s segment %d", name, i)
		assert.Equal(t, i+1, ci.Seqno, "%s segment %d", name, i)
	}
}

// TestSegmentLargeTemplateUDH checks Segment returns an error, rather than
// panicking, looping or producing oversized TPDUs, when the template UDH
// leaves too little room, whatever its size and the coding.
func TestSegmentLargeTemplateUDH(t *testing.T) {
	msgs := map[string][]byte{
		"7bit": append([]byte{0x1b, 0x65}, bytes.Repeat([]byte("a"), 300)...),
		"8bit": bytes.Repeat([]byte{0xa5}, 300),
		"ucs2": ucs2.Encode([]rune(strings.Repeat("😁a", 100))),
	}
	dcs := map[string]tpdu.DCS{"7bit": 0x00, "8bit": 0x04, "ucs2": 0x08}
	for name, msg := range msgs {
		for n := 100; n <= 160; n++ {
			p := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: dcs[name],
				UDH: tpdu.UserDataHeader{{ID: 1, Data: make([]byte, n)}}}
			for _, l := range []int{0, 2, 10, len(msg)} {
				pdus, err := segmentWithin(t, p, msg[:l])
				label := fmt.Sprintf("%s ie %d len %d", name, n, l)
				if err != nil {
					// no room, or blocks so small the message needs
					// more than 255 of them.
					if !errors.Is(err, tpdu.ErrTooManySegments) {
						assert.ErrorIs(t, err, tpdu.ErrOverlength, label)
					}
					assert.Nil(t, pdus, label)
					continue
				}
				checkSegments(t, pdus, label)
			}
		}
	}

	// the cases that used to panic or hang, from the least room each coding
	// can use to none.
	for _, p := range []struct {
		name string
		pdu  tpdu.TPDU
		msg  []byte
		ok   bool
	}{
		{"8bit 1 octet blocks", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: 0x04,
			UDH: tpdu.UserDataHeader{{ID: 1, Data: make([]byte, 131)}}}, make([]byte, 10), true},
		{"8bit no room", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: 0x04,
			UDH: tpdu.UserDataHeader{{ID: 1, Data: make([]byte, 132)}}}, make([]byte, 10), false},
		{"7bit 2 septet blocks", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01,
			UDH: tpdu.UserDataHeader{{ID: 1, Data: make([]byte, 130)}}}, []byte("\x1be\x1bea\x1bebcdef"), true},
		{"7bit 1 septet blocks", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01,
			UDH: tpdu.UserDataHeader{{ID: 1, Data: make([]byte, 131)}}}, []byte("\x1beabcdefghij"), false},
		{"ucs2 4 octet blocks", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: 0x08,
			UDH: tpdu.UserDataHeader{{ID: 1, Data: make([]byte, 128)}}}, ucs2.Encode([]rune("😁😁😁")), true},
		{"ucs2 2 octet blocks", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: 0x08,
			UDH: tpdu.UserDataHeader{{ID: 1, Data: make([]byte, 130)}}}, ucs2.Encode([]rune("😁😁😁")), false},
		{"udh alone too long", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: 0x04,
			UDH: tpdu.UserDataHeader{{ID: 1, Data: make([]byte, 138)}}}, nil, false},
		{"udh alone fits", tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: 0x04,
			UDH: tpdu.UserDataHeader{{ID: 1, Data: make([]byte, 137)}}}, nil, true},
	} {
		pdus, err := segmentWithin(t, p.pdu, p.msg)
		if !p.ok {
			assert.Equal(t, tpdu.NewEncodeError("udh", tpdu.ErrOverlength), err, p.name)
			continue
		}
		require.NoError(t, err, p.name)
		checkSegments(t, pdus, p.name)
		var ud []byte
		for _, s := range pdus {
			ud = append(ud, s.UD...)
		}
		assert.Equal(t, p.msg, ud, p.name)
	}
}

// TestSegmentCount checks a message of 255 segments is segmented, and a
// longer one is rejected, rather than wrapping the count and sequence
// numbers.
//
// TS 23.040 9.2.3.24.1: "The maximum length of an uncompressed concatenated
// short message is 39015 (255*153) default alphabet characters, 34170
// (255*134) octets or 17085 (255*67) UCS2 characters."
func TestSegmentCount(t *testing.T) {
	sixteen := []tpdu.SegmentationOption{tpdu.With16BitConcatRef}
	patterns := []struct {
		name string
		dcs  tpdu.DCS
		msg  []byte
		opts []tpdu.SegmentationOption
	}{
		{"7bit", 0x00, bytes.Repeat([]byte("a"), 153*255), nil},
		{"8bit", 0x04, bytes.Repeat([]byte{0xff}, 134*255), nil},
		{"ucs2", 0x08, bytes.Repeat([]byte{0x00, 0x61}, 67*255), nil},
		{"8bit 16 bit ref", 0x04, bytes.Repeat([]byte{0xff}, 133*255), sixteen},
		{"7bit 16 bit ref", 0x00, bytes.Repeat([]byte("a"), 152*255), sixteen},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			mr := &counter{}
			cr := &counter{}
			opts := append([]tpdu.SegmentationOption{tpdu.WithMR(mr), tpdu.WithConcatRef(cr)}, p.opts...)
			tmpl := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: p.dcs}
			pdus, err := tmpl.Segment(p.msg, opts...)
			require.NoError(t, err)
			require.Len(t, pdus, 255)
			checkSegments(t, pdus, p.name)
			assert.Equal(t, 255, mr.c)
			assert.Equal(t, 1, cr.c)

			// one more unit needs a 256th segment
			unit := 1
			if p.dcs == 0x08 {
				unit = 2
			}
			msg := append(p.msg, p.msg[:unit]...)
			pdus, err = tmpl.Segment(msg, opts...)
			assert.Equal(t, tpdu.NewEncodeError("sm", tpdu.ErrTooManySegments), err)
			assert.Nil(t, pdus)
			// and no counter was drawn from
			assert.Equal(t, 255, mr.c)
			assert.Equal(t, 1, cr.c)
		}
		t.Run(p.name, f)
	}

	// Escapes that are not split make blocks shorter, so a message of 39015
	// septets with escapes on the block boundaries needs 256 segments.
	msg := bytes.Repeat([]byte("a"), 153*255)
	msg[152] = 0x1b
	msg[153] = 0x65
	tmpl := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01}
	_, err := tmpl.Segment(msg)
	assert.ErrorIs(t, err, tpdu.ErrTooManySegments)
}

// TestSegmentMR checks the TP-MR is incremented for each segment, from that
// of the template, when no MR generator is provided.
//
// TS 23.040 9.2.3.24.1: "TP-MR must be incremented for every segment of a
// concatenated message as defined in clause 9.2.3.6."
func TestSegmentMR(t *testing.T) {
	msg := bytes.Repeat([]byte("a"), 153*3)
	for _, mr := range []byte{0, 7, 254} {
		tmpl := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x05, MR: mr}
		pdus, err := tmpl.Segment(msg)
		require.NoError(t, err)
		require.Len(t, pdus, 3)
		for i, p := range pdus {
			assert.Equal(t, mr+byte(i), p.MR) // modulo 256, as 9.2.3.6
			b, err := p.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, mr+byte(i), b[1])
		}
	}
	// a single segment keeps the template TP-MR
	tmpl := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, MR: 9}
	pdus, err := tmpl.Segment([]byte("hi"))
	require.NoError(t, err)
	assert.Equal(t, byte(9), pdus[0].MR)
}

// TestSegmentTemplateUnchanged checks Segment does not write to the template,
// including the spare capacity of its UDH, so it can be shared.
func TestSegmentTemplateUnchanged(t *testing.T) {
	sentinel := tpdu.InformationElement{ID: 0x70, Data: []byte{0x70}}
	udh := make(tpdu.UserDataHeader, 2, 4)
	udh[0] = tpdu.InformationElement{ID: 1, Data: []byte{1}}
	udh[1] = sentinel
	udh = udh[:1]
	tmpl := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, MR: 3, UDH: udh}
	orig := tmpl
	msg := bytes.Repeat([]byte("a"), 400)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var opts []tpdu.SegmentationOption
			if i%2 == 1 {
				opts = append(opts, tpdu.With16BitConcatRef)
			}
			pdus, err := tmpl.Segment(msg, opts...)
			assert.NoError(t, err)
			assert.Len(t, pdus, 3)
		}(i)
	}
	wg.Wait()
	assert.Equal(t, orig, tmpl)
	assert.Equal(t, sentinel, udh[:2][1])
}

// TestSegmentEscapes checks Segment does not split an escape sequence,
// including one that follows an escape sequence of two escapes.
//
// TS 23.040 9.2.3.24.1: "A character represented by an escape-sequence shall
// not be split in the middle."
func TestSegmentEscapes(t *testing.T) {
	// 150 'a' then ESC ESC, ESC 'e', which would end the 153 septet block
	// between the last ESC and the 'e'.
	msg := append(bytes.Repeat([]byte("a"), 150), 0x1b, 0x1b, 0x1b, 0x65)
	msg = append(msg, bytes.Repeat([]byte("b"), 20)...)
	tmpl := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01}
	pdus, err := tmpl.Segment(msg)
	require.NoError(t, err)
	require.Len(t, pdus, 2)
	assert.Equal(t, []byte{0x1b, 0x1b}, []byte(pdus[0].UD[len(pdus[0].UD)-2:]))
	assert.Equal(t, []byte{0x1b, 0x65}, []byte(pdus[1].UD[:2]))
	text, err := tpdu.DecodeUserData(pdus[1].UD, nil, tpdu.Alpha7Bit)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(text), "€b"))
}

// TestSegmentInvalidMessage checks a message that is not valid for the coding
// of the template is rejected.
func TestSegmentInvalidMessage(t *testing.T) {
	tmpl := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01}
	_, err := tmpl.Segment([]byte("caf\xc3\xa9"))
	assert.Equal(t, tpdu.NewEncodeError("sm", gsm7.ErrInvalidSeptet{Offset: 3, Septet: 0xc3}), err)
	tmpl.DCS = tpdu.DcsUCS2Data
	_, err = tmpl.Segment([]byte{0x00, 0x61, 0x00})
	assert.Equal(t, tpdu.NewEncodeError("sm", tpdu.ErrOddUCS2Length), err)
	// but any octets are valid for 8 bit and compressed data
	for _, dcs := range []tpdu.DCS{0x04, 0x20, 0x28} {
		tmpl.DCS = dcs
		pdus, err := tmpl.Segment([]byte{0xc3, 0xa9, 0x00})
		require.NoError(t, err)
		assert.Len(t, pdus, 1)
	}
	// and the type must be known
	tmpl = tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x03}
	_, err = tmpl.Segment([]byte("a"))
	assert.Equal(t, tpdu.ErrUnsupportedSmsType(7), err)
}

// TestSegmentEmptyMessage checks an empty message is carried by one TPDU.
func TestSegmentEmptyMessage(t *testing.T) {
	for _, p := range []struct {
		tmpl tpdu.TPDU
		out  string
	}{
		{tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DA: tpdu.Address{Addr: "6391", TOA: 0x91}},
			"01 01 04 91 3619 00 00 00"},
		{tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DA: tpdu.Address{Addr: "6391", TOA: 0x91},
			UDH: tpdu.UserDataHeader{{ID: 5, Data: []byte{0x0b, 0x84, 0x23, 0xf0}}}, DCS: 0x04},
			"41 01 04 91 3619 00 04 07 0605040b8423f0"},
		{tpdu.TPDU{Direction: tpdu.MO, RPMessage: tpdu.RPError, FCS: 0xd0}, "00 d0 00"},
	} {
		pdus, err := p.tmpl.Segment(nil, tpdu.WithMR(&counter{}))
		require.NoError(t, err)
		require.Len(t, pdus, 1)
		b, err := pdus[0].MarshalBinary()
		require.NoError(t, err)
		assert.Equal(t, unhex(t, p.out), b)
	}
}

// TestSegmentEmptyUDH checks the UDHL octet of an empty, but not nil, UDH is
// counted, so the segments are not overfilled.
func TestSegmentEmptyUDH(t *testing.T) {
	tmpl := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: 0x04, UDH: tpdu.UserDataHeader{}}
	assert.Equal(t, 139, tmpl.UDBlockSize())
	pdus, err := tmpl.Segment(bytes.Repeat([]byte{1}, 140))
	require.NoError(t, err)
	require.Len(t, pdus, 2)
	checkSegments(t, pdus, "8bit")
	tmpl.DCS = 0
	assert.Equal(t, 158, tmpl.UDBlockSize())
	pdus, err = tmpl.Segment(bytes.Repeat([]byte("a"), 159))
	require.NoError(t, err)
	require.Len(t, pdus, 2)
	checkSegments(t, pdus, "7bit")
}

// unmarshalConfigs are the configurations every fuzzed TPDU is decoded in.
var unmarshalConfigs = []tpdu.TPDU{
	{Direction: tpdu.MT, RPMessage: tpdu.RPAck},
	{Direction: tpdu.MT, RPMessage: tpdu.RPError},
	{Direction: tpdu.MO, RPMessage: tpdu.RPAck},
	{Direction: tpdu.MO, RPMessage: tpdu.RPError},
}

// FuzzUnmarshalBinary checks that UnmarshalBinary never panics, in either
// direction and for either RP message, and that every TPDU it accepts
// marshals, decodes back to an equal TPDU, and marshals to the octets it came
// from, other than where 3GPP TS 23.040 lets them differ, as checked by
// checkRemarshal.
func FuzzUnmarshalBinary(f *testing.F) {
	for _, seed := range []string{
		"",
		"00",
		"03",
		"00 00 00",
		"01 00 05 91 2143f5 00 00 00",
		// deliver, submit, command, status report
		"04 04 91 3619 00 00 51507132200523 08 c8303a8c0ea3c3",
		"44 04 91 3619 00 08 51507132200523 08 050003070201 0041",
		"11 23 04 91 3619 34 00 45 08 c8303a8c0ea3c3",
		"19 23 04 91 3619 34 00 45 08 c8 30 3a 8c 0e a3 c3 08 c8303a8c0ea3c3",
		"09 23 04 91 3619 34 00 01 05 00 00 00 00 00 08 c8303a8c0ea3c3",
		"42 42 00 00 34 04 91 3619 07 05 00 03 01 02 01 41",
		"02 42 04 91 3619 51507132200523 51408132200542 ab 07 89 04 02 6869",
		"02 42 04 91 3619 51507132200523 51408132200542 ab 00",
		srHead,
		// reports for RP-ACK and RP-ERROR
		"00 07 00 00 02 e834",
		"00 d0 07 00 00 02 e834",
		"01 04 51507132200523 02 e834",
		"01 c0 04 51507132200523 02 e834",
		// TP-PI extension and reserved bits
		"00 81 80 00 7f",
		"00 14 01 41 de ad",
		srHead + " 84 00 01 41",
		srHead + " 0c 01 41 aa bb",
		// 7 bit spare and fill bits, header only, compressed, reserved
		dlvHead + " 00 " + scts + " 07 edf27c1e3e971b",
		dlvHeadUDHI + " 00 " + scts + " 0f 050003010203 dae5f93c7c2ecfff",
		"41 00 04 91 3619 00 00 07 05 00 03 01 02 01 00",
		"41 00 04 91 3619 00 00 06 05 00 03 01 02 01",
		dlvHeadUDHI + " 20 " + scts + " 09 050003010201 81ff01",
		dlvHead + " 80 " + scts + " 08 c8303a8c0ea3c3",
		"07 04 91 3619 00 00 51507132200523 01 41",
		// ignored UDH, alphanumeric address
		"41 00 04 91 3619 00 04 06 05 04 01 03 01 02",
		"04 07 d0 e1f1d8 00 00 51507132200523 00",
	} {
		f.Add(unhex(f, seed))
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		for _, cfg := range unmarshalConfigs {
			d := cfg
			if err := d.UnmarshalBinary(src); err != nil {
				continue
			}
			label := fmt.Sprintf("%s %s % x", d.SmsType(), d.RPMessage, src)
			out, err := d.MarshalBinary()
			require.NoError(t, err, label)
			d2 := cfg
			require.NoError(t, d2.UnmarshalBinary(out), label)
			require.Equal(t, d, d2, label)
			out2, err := d2.MarshalBinary()
			require.NoError(t, err, label)
			require.Equal(t, out, out2, label)
			checkRemarshal(t, d, src, out, label)
		}
	})
}

// piReserved reports whether a reserved bit of the TP-PI is set.
func piReserved(d tpdu.TPDU) bool {
	if d.PI&tpdu.PiReserved != 0 {
		return true
	}
	for _, o := range d.PIExt {
		if o&^tpdu.PiExt != 0 {
			return true
		}
	}
	return false
}

// checkRemarshal checks out, the marshalled d, reproduces src, the octets d
// was unmarshalled from, other than in these cases, where 3GPP TS 23.040
// lets them differ:
//
//   - a reserved TP-PI bit is set, as the octets that follow the TP-UD are
//     then discarded (9.2.3.27).
//   - an address is not in the form it marshals to, such as one with a fill
//     semi-octet other than the last (9.1.2.3).
//   - a 7 bit UDH with no text has a TP-UDL of fewer septets than the UDH
//     and its fill bits, which is marshalled with the count 9.2.3.16 gives.
//   - 7 bit fill bits after the UDH, or spare bits after the last septet, are
//     not zero, as the receiver ignores them and they are marshalled as zero
//     (9.2.2.1, 9.2.3.24). Only those bits may differ, and they must be zero
//     in out.
func checkRemarshal(t *testing.T, d tpdu.TPDU, src, out []byte, label string) {
	t.Helper()
	if piReserved(d) {
		return
	}
	addrLen := func(i int) int { return 2 + (int(src[i])+1)/2 }
	opts := func() int {
		n := 1 + len(d.PIExt)
		if d.PI.PID() {
			n++
		}
		if d.PI.DCS() {
			n++
		}
		return n
	}
	fcs := 0
	if d.RPMessage == tpdu.RPError {
		fcs = 1
	}
	udl := -1 // index of the TP-UDL
	var addrs []int
	var addr []tpdu.Address
	switch d.SmsType() {
	case tpdu.SmsDeliver:
		addrs, addr = []int{1}, []tpdu.Address{d.OA}
		udl = 1 + addrLen(1) + 2 + 7
	case tpdu.SmsSubmit:
		addrs, addr = []int{2}, []tpdu.Address{d.DA}
		vp := map[tpdu.ValidityPeriodFormat]int{
			tpdu.VpfNotPresent: 0, tpdu.VpfRelative: 1, tpdu.VpfEnhanced: 7, tpdu.VpfAbsolute: 7,
		}[d.FirstOctet.VPF()]
		udl = 2 + addrLen(2) + 2 + vp
	case tpdu.SmsCommand:
		addrs, addr = []int{5}, []tpdu.Address{d.DA}
		udl = 5 + addrLen(5)
	case tpdu.SmsDeliverReport:
		if d.PI.UDL() {
			udl = 1 + fcs + opts()
		}
	case tpdu.SmsSubmitReport:
		if d.PI.UDL() {
			udl = 1 + fcs + opts() + 7
		}
	case tpdu.SmsStatusReport:
		addrs, addr = []int{2}, []tpdu.Address{d.RA}
		st := 2 + addrLen(2) + 7 + 7
		if d.PI.UDL() {
			udl = st + 1 + opts()
		}
	}
	for i, pos := range addrs {
		b, err := addr[i].MarshalBinary()
		require.NoError(t, err, label)
		if !bytes.Equal(b, src[pos:pos+len(b)]) || len(b) != addrLen(pos) {
			return // an address in a form it does not marshal to
		}
	}
	mask := make([]byte, len(src))
	if udl >= 0 {
		require.Less(t, udl, len(src), label)
		n := int(src[udl])
		udhl := -1 // the UDHL of the received UDH
		if d.UDH != nil {
			udhl = int(src[udl+1])
		}
		if d.SmsType() != tpdu.SmsCommand && !d.DCS.Compressed() && d.DCS.Alphabet() == tpdu.Alpha7Bit {
			octs := (n*7 + 7) / 8
			if udhl >= 0 {
				h := udhl + 1
				fill := (7 - h%7) % 7
				if (h*8+fill)/7 > n {
					return // an undercounted UDH
				}
				if fill > 0 && h < octs {
					mask[udl+1+h] |= byte(1<<fill) - 1
				}
			}
			if spare := octs*8 - n*7; spare > 0 {
				mask[udl+octs] |= ^byte(0) << (8 - spare)
			}
		}
	}
	require.Equal(t, len(src), len(out), label)
	for i := range src {
		require.Zero(t, (src[i]^out[i])&^mask[i], "%s: octet %d % x", label, i, out)
		// "Any unused bits shall be set to zero by the sending entity"
		require.Zero(t, out[i]&mask[i], "%s: unused bits of octet %d % x", label, i, out)
	}
}

// segmentTemplate returns a template of the type, coding and UDH size chosen
// by the fuzzer.
func segmentTemplate(typ, coding, udhLen, mr byte) tpdu.TPDU {
	addr := tpdu.Address{Addr: "6391", TOA: 0x91}
	var t tpdu.TPDU
	switch typ % 7 {
	case 0:
		t = tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DA: addr}
	case 1:
		t = tpdu.TPDU{FirstOctet: 0x00, OA: addr}
	case 2:
		t = tpdu.TPDU{Direction: tpdu.MO}
	case 3:
		t = tpdu.TPDU{Direction: tpdu.MO, RPMessage: tpdu.RPError, FCS: 0xd0}
	case 4:
		t = tpdu.TPDU{FirstOctet: 0x01}
	case 5:
		t = tpdu.TPDU{FirstOctet: 0x02, RA: addr}
	case 6:
		t = tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x02, DA: addr}
	}
	t.MR = mr
	t.DCS = []tpdu.DCS{0x00, 0x04, 0x08, 0x20}[coding%4]
	switch udhLen {
	case 0:
	case 1:
		t.UDH = tpdu.UserDataHeader{}
	default:
		t.UDH = tpdu.UserDataHeader{{ID: 0x70, Data: bytes.Repeat([]byte{udhLen}, int(udhLen)-2)}}
	}
	return t
}

// FuzzSegment checks that Segment, for any template and message, either
// returns an error that the template and message justify, or returns
// segments that each marshal and hold no more than their block size, that do
// not split an escape sequence or surrogate pair, whose concatenation IEs
// are consistent, and whose UD reassembles to the message. The template is
// never changed.
func FuzzSegment(f *testing.F) {
	long := bytes.Repeat([]byte("abcdefghij"), 50)
	escapes := bytes.Repeat([]byte{'a', 0x1b, 0x65, 0x1b, 0x1b, 0x1b, 0x3c}, 60)
	emoji := ucs2.Encode([]rune(strings.Repeat("a😁", 90)))
	for _, seed := range []struct {
		typ, coding, udhLen byte
		ref16               bool
		mr                  byte
		msg                 []byte
	}{
		{0, 0x80, 0, false, 0, []byte("hello")},
		{0, 0x80, 0, false, 7, long},
		{0, 0x80, 6, true, 255, escapes},
		{1, 0x82, 1, false, 0, emoji},
		{2, 0x81, 130, false, 0, long},
		{3, 0x82, 128, true, 0, emoji},
		{4, 0x83, 20, false, 0, long},
		{5, 0x80, 50, false, 0, escapes},
		{6, 0x81, 140, false, 0, long},
		{0, 0x80, 131, false, 0, escapes},
		{0, 0x00, 0, false, 0, []byte("caf\xc3\xa9")},
		{0, 0x02, 0, false, 0, []byte{0, 0x61, 0}},
		{0, 0x80, 0, false, 0, nil},
	} {
		f.Add(seed.typ, seed.coding, seed.udhLen, seed.ref16, seed.mr, seed.msg)
	}

	f.Fuzz(func(t *testing.T, typ, coding, udhLen byte, ref16 bool, mr byte, msg []byte) {
		tmpl := segmentTemplate(typ, coding, udhLen, mr)
		cdg := tmpl.DCS
		sevenBit := tmpl.SmsType() != tpdu.SmsCommand && cdg == 0x00
		ucs := tmpl.SmsType() != tpdu.SmsCommand && cdg == 0x08
		if coding&0x80 != 0 { // make the message valid for the coding
			msg = append([]byte(nil), msg...)
			if sevenBit {
				for i := range msg {
					msg[i] &= 0x7f
				}
			}
			if ucs && len(msg)%2 == 1 {
				msg = msg[:len(msg)-1]
			}
		}
		var tudh tpdu.UserDataHeader
		if tmpl.UDH != nil {
			tudh = append(tpdu.UserDataHeader{}, tmpl.UDH...)
		}
		orig := tmpl
		var opts []tpdu.SegmentationOption
		if ref16 {
			opts = append(opts, tpdu.With16BitConcatRef)
		}
		pdus, err := tmpl.Segment(msg, opts...)
		require.Equal(t, orig, tmpl)
		if tudh != nil {
			require.Equal(t, tudh, tmpl.UDH[:len(tmpl.UDH):len(tmpl.UDH)])
		}

		if err != nil {
			require.Nil(t, pdus)
			var ise gsm7.ErrInvalidSeptet
			switch {
			case errors.As(err, &ise):
				require.True(t, sevenBit)
				require.Greater(t, msg[ise.Offset], byte(0x7f))
			case errors.Is(err, tpdu.ErrOddUCS2Length):
				require.True(t, ucs)
				require.Equal(t, 1, len(msg)%2)
			case errors.Is(err, tpdu.ErrOverlength), errors.Is(err, tpdu.ErrTooManySegments):
				// the room left by the template UDH and a concatenation
				// IE, of 5 or 6 octets.
				c := tmpl
				ie := tpdu.InformationElement{ID: 0, Data: []byte{1, 2, 3}}
				if ref16 {
					ie = tpdu.InformationElement{ID: 8, Data: []byte{1, 2, 3, 4}}
				}
				c.UDH = append(append(tpdu.UserDataHeader{}, tmpl.UDH...), ie)
				bs := c.UDBlockSize()
				require.Greater(t, len(msg), tmpl.UDBlockSize())
				// a segment must hold the longest character, which is an
				// escape sequence or a surrogate pair only if the message
				// has one, and may then be a unit short of the room.
				unit, shrink := 1, 0
				if sevenBit && bytes.IndexByte(msg, 0x1b) >= 0 {
					unit, shrink = 2, 1
				}
				if ucs {
					unit = 2
					for i := 0; i+1 < len(msg); i += 2 {
						if msg[i]&0xfc == 0xd8 {
							unit, shrink = 4, 2
						}
					}
				}
				if errors.Is(err, tpdu.ErrOverlength) {
					require.Less(t, bs, unit)
				} else {
					require.GreaterOrEqual(t, bs, unit)
					require.Greater(t, len(msg), 255*(bs-shrink))
				}
			default:
				t.Fatalf("unexpected error %v", err)
			}
			return
		}

		require.NotEmpty(t, pdus)
		require.LessOrEqual(t, len(pdus), 255)
		var ud []byte
		ref := -1
		for i, p := range pdus {
			ud = append(ud, p.UD...)
			require.LessOrEqual(t, len(p.UD), p.UDBlockSize())
			require.Equal(t, mr+byte(i), p.MR)
			b, err := p.MarshalBinary()
			require.NoError(t, err)
			d := tpdu.TPDU{Direction: p.Direction, RPMessage: p.RPMessage}
			require.NoError(t, d.UnmarshalBinary(b))
			require.Equal(t, len(p.UD), len(d.UD))
			if len(p.UD) > 0 {
				require.Equal(t, p.UD, d.UD)
			}
			if len(pdus) == 1 {
				require.Equal(t, tudh, p.UDH)
				continue
			}
			require.Len(t, p.UDH, len(tudh)+1)
			// an IE with no data may be nil or empty
			want, err := p.UDH.MarshalBinary()
			require.NoError(t, err)
			got, err := d.UDH.MarshalBinary()
			require.NoError(t, err)
			require.Equal(t, want, got)
			ci, ok := p.ConcatInfo()
			require.True(t, ok)
			require.Equal(t, len(pdus), ci.Total)
			require.Equal(t, i+1, ci.Seqno)
			require.Equal(t, ref16, ci.Ref16Bit)
			if ref < 0 {
				ref = ci.Ref
			}
			require.Equal(t, ref, ci.Ref)
		}
		require.Equal(t, len(msg), len(ud))
		if len(msg) > 0 {
			require.Equal(t, msg, ud)
		}
		// no segment boundary splits an escape sequence or surrogate pair
		cont := make([]bool, len(msg)+1)
		if sevenBit {
			for i := 0; i < len(msg); i++ {
				if msg[i] == 0x1b && i+1 < len(msg) {
					cont[i+1] = true
					i++
				}
			}
		}
		if ucs {
			for i := 0; i+4 <= len(msg); i += 2 {
				hi := int(msg[i])<<8 | int(msg[i+1])
				lo := int(msg[i+2])<<8 | int(msg[i+3])
				if hi >= 0xd800 && hi < 0xdc00 && lo >= 0xdc00 && lo < 0xe000 {
					cont[i+2] = true
				}
			}
		}
		at := 0
		for _, p := range pdus[:len(pdus)-1] {
			at += len(p.UD)
			require.False(t, cont[at], "segment boundary at %d splits a character", at)
		}
	})
}

// counter is an implementation of the tpdu.Counter interface.
//
// It is never used in a multi-threaded setting and so is not MT safe.
type counter struct {
	c int
}

// Count increments and returns the counter.
func (c *counter) Count() int {
	c.c++
	return c.c
}

// TestSegmentMinimalRoom checks Segment only needs room for the characters
// the message has: 3GPP TS 23.040 Section 9.2.3.24.1 says "A character
// represented by an escape-sequence shall not be split in the middle" and "A
// UCS2 character shall not be split in the middle", so a segment must hold an
// escape sequence, or a surrogate pair, only if the message contains one.
func TestSegmentMinimalRoom(t *testing.T) {
	// a template UDH that, with the concatenation IE, leaves room for one
	// septet, or one UCS2 code unit, in each segment.
	template := func(dcs tpdu.DCS, head int) tpdu.TPDU {
		d, err := tpdu.NewSubmit(tpdu.WithDA(tpdu.NewAddress(tpdu.FromNumber("123"))))
		require.NoError(t, err)
		d.SetDCS(byte(dcs))
		d.SetUDH(tpdu.UserDataHeader{{ID: 0x70, Data: make([]byte, head)}})
		return *d
	}
	patterns := []struct {
		name  string
		t     tpdu.TPDU
		msg   []byte
		count int // 0 for an error
	}{
		// without the concatenation IE, the template leaves room for 6
		// septets, or 3 UCS2 code units, so these need segmenting.
		{"7bit", template(0x00, 131), []byte("AAAAAAAAAA"), 10},
		{"7bit escape", template(0x00, 131), []byte("AAAAAAAA\x1b\x65"), 0},
		{"ucs2", template(tpdu.DcsUCS2Data, 130), []byte("\x00A\x00B\x00C\x00D"), 4},
		{"ucs2 surrogate pair", template(tpdu.DcsUCS2Data, 130), []byte("\x00A\x00B\x00C\xd8\x3d\xde\x01"), 0},
		{"ucs2 lone high surrogate", template(tpdu.DcsUCS2Data, 130), []byte("\x00A\x00B\x00C\xd8\x3d\x00D"), 0},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			pdus, err := p.t.Segment(p.msg)
			if p.count == 0 {
				assert.ErrorIs(t, err, tpdu.ErrOverlength)
				assert.Nil(t, pdus)
				return
			}
			require.NoError(t, err)
			require.Len(t, pdus, p.count)
			var got []byte
			for i := range pdus {
				b, err := pdus[i].MarshalBinary()
				require.NoError(t, err)
				assert.LessOrEqual(t, len(b), 164)
				got = append(got, pdus[i].UD...)
			}
			assert.Equal(t, p.msg, got)
		}
		t.Run(p.name, f)
	}
}
