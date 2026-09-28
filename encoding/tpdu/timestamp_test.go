// SPDX-License-Identifier: MIT

package tpdu_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/gomaja/go-sms/encoding/bcd"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type marshalTimestampPattern struct {
	name string
	in   tpdu.Timestamp
	out  []byte
	err  error
}

func TestMarhalBinary(t *testing.T) {
	patterns := []marshalTimestampPattern{
		{
			"19700101",
			tpdu.Timestamp{
				Time: time.Date(1970, time.January, 1, 1, 2, 3, 0, time.UTC),
			},
			[]byte{0x07, 0x10, 0x10, 0x10, 0x20, 0x30, 0x00},
			nil,
		},
		{
			"19991231",
			tpdu.Timestamp{
				Time: time.Date(1999, time.December, 31, 23, 59, 59, 0, time.FixedZone("SCTS", -15*60)),
			},
			[]byte{0x99, 0x21, 0x13, 0x32, 0x95, 0x95, 0x18},
			nil,
		},
		{
			"20001231",
			tpdu.Timestamp{Time: time.Date(2000, time.December, 31, 23, 59, 59, 0, time.FixedZone("SCTS", 15*60))},
			[]byte{0x00, 0x21, 0x13, 0x32, 0x95, 0x95, 0x10},
			nil,
		},
		{
			"20170831",
			tpdu.Timestamp{
				Time: time.Date(2017, time.August, 31, 11, 21, 54, 0, time.FixedZone("any", 8*3600)),
			},
			[]byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23},
			nil,
		},
		{
			"20691231",
			tpdu.Timestamp{
				Time: time.Date(2069, time.December, 31, 23, 59, 59, 0, time.FixedZone("SCTS", -79*15*60)),
			},
			[]byte{0x96, 0x21, 0x13, 0x32, 0x95, 0x95, 0x9f},
			nil,
		},
		{
			"subsecond",
			tpdu.Timestamp{
				Time: time.Date(2017, time.August, 31, 11, 21, 54, 999999999, time.FixedZone("any", 8*3600)),
			},
			[]byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23},
			nil,
		},
		// the zero Timestamp is the all-zero SCTS
		{
			"zero",
			tpdu.Timestamp{},
			[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			nil,
		},
		// 3GPP TS 23.040 Section 9.2.3.11 only provides a two digit year,
		// which is decoded as 1970 to 2069.
		{
			"20700101",
			tpdu.Timestamp{
				Time: time.Date(2070, time.January, 1, 1, 2, 3, 0, time.UTC),
			},
			nil,
			tpdu.EncodeError("year", tpdu.ErrInvalid),
		},
		{
			"19691231",
			tpdu.Timestamp{
				Time: time.Date(1969, time.December, 31, 23, 59, 59, 0, time.UTC),
			},
			nil,
			tpdu.EncodeError("year", tpdu.ErrInvalid),
		},
		{
			"21001231",
			tpdu.Timestamp{
				Time: time.Date(2100, time.December, 31, 23, 59, 59, 0, time.FixedZone("SCTS", 15*60)),
			},
			nil,
			tpdu.EncodeError("year", tpdu.ErrInvalid),
		},
		// "The Time Zone indicates the difference, expressed in quarters of
		// an hour, between the local time and GMT."
		{
			"offset not in quarter hours",
			tpdu.Timestamp{
				Time: time.Date(2017, time.August, 31, 11, 21, 54, 0, time.FixedZone("any", 5*3600+50*60)),
			},
			nil,
			tpdu.EncodeError("tz", tpdu.ErrInvalid),
		},
		{
			"tz beyond 79 quarters",
			tpdu.Timestamp{
				Time: time.Date(2017, time.December, 31, 23, 59, 59, 0, time.FixedZone("SCTS", 24*3600)),
			},
			nil,
			bcd.ErrInvalidInteger(96),
		},
		// how to trigger invalid integer in date (other than tz)??
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			b, err := p.in.MarshalBinary()
			require.Equal(t, p.err, err)
			assert.Equal(t, p.out, b)
		}
		t.Run(p.name, f)
	}
}

func TestUnmarhalBinary(t *testing.T) {
	patterns := []struct {
		name string
		in   []byte
		out  tpdu.Timestamp
		err  error
	}{
		{
			"19700101",
			[]byte{0x07, 0x10, 0x10, 0x10, 0x20, 0x30, 0x00},
			tpdu.Timestamp{
				Time: time.Date(1970, time.January, 1, 1, 2, 3, 0, time.UTC),
			},
			nil,
		},
		{
			"19991231",
			[]byte{0x99, 0x21, 0x13, 0x32, 0x95, 0x95, 0x18},
			tpdu.Timestamp{
				Time: time.Date(1999, time.December, 31, 23, 59, 59, 0, time.FixedZone("SCTS", -15*60)),
			},
			nil,
		},
		{
			"20001231",
			[]byte{0x00, 0x21, 0x13, 0x32, 0x95, 0x95, 0x10},
			tpdu.Timestamp{
				Time: time.Date(2000, time.December, 31, 23, 59, 59, 0, time.FixedZone("SCTS", 15*60)),
			},
			nil,
		},
		{
			"20170831",
			[]byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23},
			tpdu.Timestamp{
				Time: time.Date(2017, time.August, 31, 11, 21, 54, 0, time.FixedZone("SCTS", 8*3600)),
			},
			nil,
		},
		{
			"short",
			[]byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45},
			tpdu.Timestamp{},
			tpdu.ErrUnderflow,
		},
		// 3GPP TS 23.040 Section 9.2.3.11: "If the MS receives a non-integer
		// value in the SCTS, it shall assume that the digit is set to 0".
		{
			"non-integer digit",
			[]byte{0xa1, 0x80, 0x13, 0x11, 0x12, 0x45, 0x00},
			tpdu.Timestamp{
				Time: time.Date(2010, time.August, 31, 11, 21, 54, 0, time.UTC),
			},
			nil,
		},
		{
			"non-integer tz digit",
			[]byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0xa0},
			tpdu.Timestamp{
				Time: time.Date(2017, time.August, 31, 11, 21, 54, 0, time.UTC),
			},
			nil,
		},
		{
			"non-integer tens digit",
			[]byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x4f, 0x23},
			tpdu.Timestamp{
				Time: time.Date(2017, time.August, 31, 11, 21, 4, 0, time.FixedZone("SCTS", 8*3600)),
			},
			nil,
		},
		// Fields out of range do not form a time, so Time is left zero.
		{
			"all zero",
			[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			tpdu.Timestamp{},
			nil,
		},
		{
			"month 13",
			[]byte{0x71, 0x31, 0x13, 0x11, 0x12, 0x45, 0x23},
			tpdu.Timestamp{},
			nil,
		},
		{
			"30 February",
			[]byte{0x71, 0x20, 0x03, 0x11, 0x12, 0x45, 0x23},
			tpdu.Timestamp{},
			nil,
		},
		{
			"hour 24",
			[]byte{0x71, 0x80, 0x13, 0x42, 0x12, 0x45, 0x23},
			tpdu.Timestamp{},
			nil,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			s := tpdu.Timestamp{}
			err := s.UnmarshalBinary(p.in)
			if err != p.err {
				t.Fatalf("error unmarshalling %v: %v", p.in, err)
			}
			if !s.Equal(p.out.Time) {
				t.Fatalf("failed to unmarshal %v: expected %v, got %v", p.in, p.out, s)
			}
			szn, szo := s.Zone()
			ozn, ozo := p.out.Zone()
			assert.Equal(t, ozn, szn)
			assert.Equal(t, ozo, szo)
		}
		t.Run(p.name, f)
	}
}

func TestTimestampString(t *testing.T) {
	patterns := []struct {
		in  time.Time
		out string
	}{
		{
			time.Date(2000, 11, 2, 3, 4, 5, 65, time.FixedZone("ABC", 22800)),
			"2000-11-02 03:04:05 +0620",
		},
		{
			time.Date(2000, 11, 2, 3, 4, 5, 0, time.FixedZone("ABC", 22800)),
			"2000-11-02 03:04:05 +0620",
		},
		{
			time.Date(2000, 11, 2, 3, 4, 5, 0, time.FixedZone("TEST", 0)),
			"2000-11-02 03:04:05 +0000",
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out := tpdu.Timestamp{Time: p.in}.String()
			assert.Equal(t, p.out, out)
		}
		t.Run(fmt.Sprintf("%02x", p.in), f)
	}
}

// timestampFrom returns the Timestamp unmarshalled from the SCTS octets.
func timestampFrom(b ...byte) tpdu.Timestamp {
	var ts tpdu.Timestamp
	if err := ts.UnmarshalBinary(b); err != nil {
		panic(err)
	}
	return ts
}

func TestTimestampRoundTrip(t *testing.T) {
	// 3GPP TS 23.040 Section 9.2.3.11: "Messages shall be stored as received
	// without change to any time contained therein", and a non-integer digit
	// is read as 0 but "the entire field" is stored "exactly as received".
	patterns := []struct {
		name string
		in   []byte
		zero bool
	}{
		{"valid", []byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23}, false},
		{"utc", []byte{0x07, 0x10, 0x10, 0x10, 0x20, 0x30, 0x00}, false},
		{"negative zero tz", []byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x08}, false},
		{"non-integer year", []byte{0xa1, 0x80, 0x13, 0x11, 0x12, 0x45, 0x00}, false},
		{"non-integer seconds", []byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x4a, 0x23}, false},
		{"non-integer tz", []byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0xf8}, false},
		{"all non-integer", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, true},
		{"all zero", []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, true},
		{"month 0", []byte{0x71, 0x00, 0x13, 0x11, 0x12, 0x45, 0x23}, true},
		{"month 13", []byte{0x71, 0x31, 0x13, 0x11, 0x12, 0x45, 0x23}, true},
		{"day 0", []byte{0x71, 0x80, 0x00, 0x11, 0x12, 0x45, 0x23}, true},
		{"day 32", []byte{0x71, 0x80, 0x23, 0x11, 0x12, 0x45, 0x23}, true},
		{"31 September", []byte{0x71, 0x90, 0x13, 0x11, 0x12, 0x45, 0x23}, true},
		{"29 February 2017", []byte{0x71, 0x20, 0x92, 0x11, 0x12, 0x45, 0x23}, true},
		{"29 February 2016", []byte{0x61, 0x20, 0x92, 0x11, 0x12, 0x45, 0x23}, false},
		{"hour 24", []byte{0x71, 0x80, 0x13, 0x42, 0x12, 0x45, 0x23}, true},
		{"minute 60", []byte{0x71, 0x80, 0x13, 0x11, 0x06, 0x45, 0x23}, true},
		{"second 60", []byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x06, 0x23}, true},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			var ts tpdu.Timestamp
			err := ts.UnmarshalBinary(p.in)
			require.Nil(t, err)
			assert.Equal(t, p.zero, ts.IsZero())
			b, err := ts.MarshalBinary()
			require.Nil(t, err)
			assert.Equal(t, p.in, b)
		}
		t.Run(p.name, f)
	}
}

func TestTimestampChangedAfterUnmarshal(t *testing.T) {
	// The received octets are only reproduced while Time still holds the
	// time decoded from them.
	var ts tpdu.Timestamp
	err := ts.UnmarshalBinary([]byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x4a, 0x23})
	require.Nil(t, err)
	assert.True(t, ts.Equal(time.Date(2017, time.August, 31, 11, 21, 4, 0, time.FixedZone("SCTS", 8*3600))))
	ts.Time = ts.Add(time.Second)
	b, err := ts.MarshalBinary()
	require.Nil(t, err)
	assert.Equal(t, []byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x50, 0x23}, b)

	// the same instant in another time zone is a change
	err = ts.UnmarshalBinary([]byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x4a, 0x23})
	require.Nil(t, err)
	ts.Time = ts.In(time.UTC)
	b, err = ts.MarshalBinary()
	require.Nil(t, err)
	assert.Equal(t, []byte{0x71, 0x80, 0x13, 0x30, 0x12, 0x40, 0x00}, b)

	err = ts.UnmarshalBinary([]byte{0x71, 0x31, 0x13, 0x11, 0x12, 0x45, 0x23})
	require.Nil(t, err)
	ts.Time = time.Date(2017, time.August, 31, 11, 21, 54, 0, time.UTC)
	b, err = ts.MarshalBinary()
	require.Nil(t, err)
	assert.Equal(t, []byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x00}, b)

	// and a Timestamp that is decoded again drops the earlier octets
	err = ts.UnmarshalBinary([]byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23})
	require.Nil(t, err)
	assert.Equal(t, tpdu.Timestamp{Time: time.Date(2017, time.August, 31, 11, 21, 54, 0, time.FixedZone("SCTS", 8*3600))}, ts)
}

func TestTimestampNonIntegerDeliver(t *testing.T) {
	// An SMS-DELIVER whose SCTS seconds are 4a (a non-integer tens digit)
	// must decode, and re-marshal exactly as received.
	b := []byte{
		0x04,                                           // first octet
		0x0b, 0x91, 0x16, 0x04, 0x89, 0x56, 0x26, 0xf9, // OA
		0x00,                                     // PID
		0x00,                                     // DCS
		0x71, 0x80, 0x13, 0x11, 0x12, 0x4a, 0x23, // SCTS
		0x05, 0xe8, 0x32, 0x9b, 0xfd, 0x06, // UDL and UD "hello"
	}
	var pdu tpdu.TPDU
	err := pdu.UnmarshalBinary(b)
	require.Nil(t, err)
	assert.True(t, pdu.SCTS.Equal(time.Date(2017, time.August, 31, 11, 21, 4, 0, time.FixedZone("SCTS", 8*3600))))
	m, err := pdu.MarshalBinary()
	require.Nil(t, err)
	assert.Equal(t, b, m)
}

// FuzzTimestampUnmarshalBinary checks that any 7 octets unmarshal, as a
// non-integer digit is read as 0 and invalid fields leave the Time zero, and
// that they marshal back exactly as received (3GPP TS 23.040 Section
// 9.2.3.11).
func FuzzTimestampUnmarshalBinary(f *testing.F) {
	for _, seed := range [][]byte{
		{},
		{0x71, 0x80, 0x13, 0x11, 0x12, 0x45},
		{0x07, 0x10, 0x10, 0x10, 0x20, 0x30, 0x00},
		{0x99, 0x21, 0x13, 0x32, 0x95, 0x95, 0x18},
		{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23},
		{0x96, 0x21, 0x13, 0x32, 0x95, 0x95, 0x9f},
		{0xa1, 0x80, 0x13, 0x11, 0x12, 0x45, 0x00},
		{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x08},
		{0x61, 0x20, 0x92, 0x11, 0x12, 0x45, 0x23},
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		var ts tpdu.Timestamp
		err := ts.UnmarshalBinary(src)
		if len(src) < 7 {
			if err == nil {
				t.Fatalf("% x: no error", src)
			}
			return
		}
		if err != nil {
			t.Fatalf("% x: unexpected error %v", src, err)
		}
		b, err := ts.MarshalBinary()
		if err != nil {
			t.Fatalf("% x: marshal error %v", src, err)
		}
		if !bytes.Equal(src[:7], b) {
			t.Fatalf("% x: remarshalled to % x", src[:7], b)
		}
		if ts.IsZero() {
			return
		}
		// a decoded time is always one that can be marshalled afresh
		fresh := tpdu.Timestamp{Time: ts.Time}
		fb, err := fresh.MarshalBinary()
		if err != nil {
			t.Fatalf("% x: time %v cannot be marshalled: %v", src, ts.Time, err)
		}
		var rt tpdu.Timestamp
		if err := rt.UnmarshalBinary(fb); err != nil || !rt.Equal(ts.Time) {
			t.Fatalf("% x: time %v remarshalled to % x, decoded as %v", src, ts.Time, fb, rt.Time)
		}
	})
}
