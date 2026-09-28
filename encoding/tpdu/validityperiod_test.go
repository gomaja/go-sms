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

func TestVPEnhancedFormat(t *testing.T) {
	patterns := []struct {
		in  byte
		out tpdu.EnhancedValidityPeriodFormat
	}{
		{0x00, 0x00},
		{0x07, 0x07},
		{0x0f, 0x07},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			out := tpdu.EnhancedFormat(p.in)
			assert.Equal(t, p.out, out)
		}
		t.Run(fmt.Sprintf("%02x", p.in), f)
	}
}

func TestVPSetAbsolute(t *testing.T) {
	v := tpdu.ValidityPeriod{}
	soon := tpdu.Timestamp{Time: time.Now().Add(300 * time.Second)}
	v.SetAbsolute(soon)
	if v.Format != tpdu.VpfAbsolute {
		t.Errorf("format is %v, expected %v", v.Format, tpdu.VpfAbsolute)
	}
	if v.Time != soon {
		t.Errorf("time is %v, expected %v", v.Time, soon)
	}
}

func TestVPSetRelative(t *testing.T) {
	v := tpdu.ValidityPeriod{}
	v.SetRelative(123 * time.Second)
	if v.Format != tpdu.VpfRelative {
		t.Errorf("format is %v, expected %v", v.Format, tpdu.VpfRelative)
	}
	if v.Duration != 123*time.Second {
		t.Errorf("duration is %v, expected %v", v.Duration, 123*time.Second)
	}
}

func TestVPSetEnhanced(t *testing.T) {
	v := tpdu.ValidityPeriod{}
	seconds := 123 * time.Second
	efi := 4
	v.SetEnhanced(seconds, byte(efi))
	if v.Format != tpdu.VpfEnhanced {
		t.Errorf("format is %v, expected %v", v.Format, tpdu.VpfRelative)
	}
	if v.Duration != seconds {
		t.Errorf("duration is %v, expected %v", v.Duration, seconds)
	}
	if v.EFI != byte(efi) {
		t.Errorf("efi is %x, expected %x", v.EFI, efi)
	}
}

func TestVPMarshalBinary(t *testing.T) {
	patterns := []struct {
		name string
		in   tpdu.ValidityPeriod
		out  []byte
		err  error
	}{
		{"notpresent", tpdu.ValidityPeriod{}, nil, nil},
		{"absolute",
			tpdu.ValidityPeriod{
				Format: tpdu.VpfAbsolute,
				Time: tpdu.Timestamp{
					Time: time.Date(2017, time.August, 31, 11, 21, 54, 0,
						time.FixedZone("SCTS", 8*3600))},
			},
			[]byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23},
			nil},
		{"relativeMinutes",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 11 * time.Hour},
			[]byte{0x83},
			nil},
		// 3GPP TS 23.040 Section 9.2.3.12.1: TP-VP 0 is (0+1) x 5 minutes.
		{"relative5m",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 5 * time.Minute},
			[]byte{0x00},
			nil},
		{"relative5mRoundedDown",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 10*time.Minute - time.Second},
			[]byte{0x00},
			nil},
		{"relative10m",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 10 * time.Minute},
			[]byte{0x01},
			nil},
		{"relative12h",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 12 * time.Hour},
			[]byte{0x8f},
			nil},
		{"relative12h30m",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 12*time.Hour + 30*time.Minute},
			[]byte{0x90},
			nil},
		{"relative24h",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 24 * time.Hour},
			[]byte{0xa7},
			nil},
		{"relative30d",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 30 * 24 * time.Hour},
			[]byte{0xc4},
			nil},
		{"relative5w",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 5 * 7 * 24 * time.Hour},
			[]byte{0xc5},
			nil},
		{"relativeHours",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 23 * time.Hour},
			[]byte{0xa5},
			nil},
		{"relativeDays",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 29 * 24 * time.Hour},
			[]byte{0xc3},
			nil},
		{"relativeWeeks",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 62 * 7 * 24 * time.Hour},
			[]byte{0xfe},
			nil},
		{"relativeMax",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 63 * 7 * 24 * time.Hour},
			[]byte{0xff},
			nil},
		{"enhancedNotPresent",
			tpdu.ValidityPeriod{
				Format: tpdu.VpfEnhanced,
				EFI:    byte(tpdu.EvpfNotPresent)},
			[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			nil},
		{"enhancedRelative5m",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelative),
				Duration: 5 * time.Minute},
			[]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			nil},
		{"enhancedRelative10m",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelative),
				Duration: 10 * time.Minute},
			[]byte{0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00},
			nil},
		{"enhancedRelativeSeconds",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeSeconds),
				Duration: 255 * time.Second},
			[]byte{0x02, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00},
			nil},
		{"enhancedRelativeSecondsMin",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeSeconds),
				Duration: time.Second},
			[]byte{0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00},
			nil},
		{"enhancedRelativeSecondsRoundedDown",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeSeconds),
				Duration: 2*time.Second - time.Millisecond},
			[]byte{0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00},
			nil},
		{"enhancedHHMMSSMax",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeHHMMSS),
				Duration: 99*time.Hour + 59*time.Minute + 59*time.Second,
			},
			[]byte{0x03, 0x99, 0x95, 0x95, 0x00, 0x00, 0x00},
			nil},
		{"enhancedHHMMSSZero",
			tpdu.ValidityPeriod{
				Format: tpdu.VpfEnhanced,
				EFI:    byte(tpdu.EvpfRelativeHHMMSS),
			},
			[]byte{0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			nil},
		// durations that cannot be represented
		{"relativeNegative",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: -time.Hour},
			nil,
			tpdu.EncodeError("duration", tpdu.ErrInvalid)},
		{"relativeZero",
			tpdu.ValidityPeriod{
				Format: tpdu.VpfRelative},
			nil,
			tpdu.EncodeError("duration", tpdu.ErrInvalid)},
		{"relativeBelowMin",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 5*time.Minute - time.Second},
			nil,
			tpdu.EncodeError("duration", tpdu.ErrInvalid)},
		{"relativeAboveMax",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 63*7*24*time.Hour + time.Second},
			nil,
			tpdu.EncodeError("duration", tpdu.ErrInvalid)},
		{"enhancedRelativeNegative",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelative),
				Duration: -5 * time.Minute},
			nil,
			tpdu.EncodeError("duration", tpdu.ErrInvalid)},
		{"enhancedRelativeSecondsNegative",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeSeconds),
				Duration: -time.Second},
			nil,
			tpdu.EncodeError("duration", tpdu.ErrInvalid)},
		{"enhancedRelativeSecondsBelowMin",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeSeconds),
				Duration: time.Second - time.Millisecond},
			nil,
			tpdu.EncodeError("duration", tpdu.ErrInvalid)},
		{"enhancedRelativeSecondsAboveMax",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeSeconds),
				Duration: time.Hour},
			nil,
			tpdu.EncodeError("duration", tpdu.ErrInvalid)},
		{"enhancedHHMMSSNegative",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeHHMMSS),
				Duration: -time.Second},
			nil,
			tpdu.EncodeError("duration", tpdu.ErrInvalid)},
		{"enhancedHHMMSSAboveMax",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeHHMMSS),
				Duration: 100*time.Hour + 30*time.Minute},
			nil,
			tpdu.EncodeError("duration", tpdu.ErrInvalid)},
		{"enhancedHHMMSS",
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeHHMMSS),
				Duration: 3*time.Hour + 12*time.Minute + 45*time.Second,
			},
			[]byte{0x03, 0x30, 0x21, 0x54, 0x00, 0x00, 0x00},
			nil},
		{"invalid enhanced",
			tpdu.ValidityPeriod{Format: tpdu.VpfEnhanced, EFI: 0xff},
			nil,
			tpdu.EncodeError("fi", tpdu.ErrInvalid)},
	}

	for _, p := range patterns {
		f := func(t *testing.T) {
			b, err := p.in.MarshalBinary()
			if err != p.err {
				t.Errorf("error encoding '%v': %v", p.in, err)
			}
			assert.Equal(t, p.out, b)
		}
		t.Run(p.name, f)
	}
}

func TestVPUnmarshalBinary(t *testing.T) {
	patterns := []struct {
		name      string
		in        []byte
		vpf       tpdu.ValidityPeriodFormat
		readCount int
		out       tpdu.ValidityPeriod
		err       error
	}{
		{"notpresent", nil, tpdu.VpfNotPresent, 0, tpdu.ValidityPeriod{}, nil},
		{"absolute",
			[]byte{0x71, 0x80, 0x13, 0x11, 0x12, 0x45, 0x23},
			tpdu.VpfAbsolute, 7,
			tpdu.ValidityPeriod{
				Format: tpdu.VpfAbsolute,
				Time: tpdu.Timestamp{Time: time.Date(2017, time.August, 31, 11, 21, 54, 0,
					time.FixedZone("SCTS", 8*3600))}},
			nil},
		{"relativeMinutes",
			[]byte{0x83},
			tpdu.VpfRelative, 1,
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 11 * time.Hour},
			nil},
		{"relativeHours",
			[]byte{0xa5},
			tpdu.VpfRelative, 1,
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 23 * time.Hour},
			nil},
		{"relativeDays",
			[]byte{0xc3},
			tpdu.VpfRelative, 1,
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 29 * 24 * time.Hour},
			nil},
		{"relativeWeeks",
			[]byte{0xfe},
			tpdu.VpfRelative, 1,
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 62 * 7 * 24 * time.Hour},
			nil},
		{"relativeMax",
			[]byte{0xff},
			tpdu.VpfRelative, 1,
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfRelative,
				Duration: 63 * 7 * 24 * time.Hour},
			nil},
		{"enhancedNotPresent",
			[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 7,
			tpdu.ValidityPeriod{
				Format: tpdu.VpfEnhanced,
				EFI:    byte(tpdu.EvpfNotPresent)},
			nil},
		{"enhancedRelative5m",
			[]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 7,
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelative),
				Duration: 5 * time.Minute},
			nil},
		{"enhancedRelative10m",
			[]byte{0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 7,
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelative),
				Duration: 10 * time.Minute},
			nil},
		{"enhancedRelativeSeconds",
			[]byte{0x02, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 7,
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeSeconds),
				Duration: 255 * time.Second},
			nil},
		{"enhancedHHMMSS",
			[]byte{0x03, 0x30, 0x21, 0x54, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 7,
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeHHMMSS),
				Duration: 3*time.Hour + 12*time.Minute + 45*time.Second,
			},
			nil},
		{"underflow relative",
			nil,
			tpdu.VpfRelative, 0,
			tpdu.ValidityPeriod{},
			tpdu.ErrUnderflow},
		{"underflow enhanced",
			[]byte{0x00, 0x01},
			tpdu.VpfEnhanced, 0,
			tpdu.ValidityPeriod{},
			tpdu.ErrUnderflow},
		{"invalid vpf",
			nil,
			0x4, 0,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("vpf", 0, tpdu.ErrInvalid)},
		{"invalid evpf",
			[]byte{0x07, 0x01, 0x2d, 0x54, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 7,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("enhanced", 0, tpdu.ErrInvalid)},
		{"invalid enhancedHHMMSS",
			[]byte{0x03, 0x30, 0x2d, 0x54, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 4,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("enhanced", 1, bcd.ErrInvalidOctet(0x2d))},
		{"nonzero pad enhancedNotPresent",
			[]byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 1,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("enhanced", 1, tpdu.ErrNonZero)},
		{"nonzero pad enhancedRelative",
			[]byte{0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 2,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("enhanced", 2, tpdu.ErrNonZero)},
		{"enhancedSingleShot",
			[]byte{0x41, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 7,
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      0x41,
				Duration: 30 * time.Minute},
			nil},
		{"enhancedHHMMSSMax",
			[]byte{0x03, 0x99, 0x95, 0x95, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 7,
			tpdu.ValidityPeriod{
				Format:   tpdu.VpfEnhanced,
				EFI:      byte(tpdu.EvpfRelativeHHMMSS),
				Duration: 99*time.Hour + 59*time.Minute + 59*time.Second,
			},
			nil},
		// 3GPP TS 23.040 Section 9.2.3.12.3: "Any reserved/unused bits or
		// octets must be set to zero."
		{"reserved bits enhanced",
			[]byte{0x09, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 0,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("enhanced", 0, tpdu.ErrNonZero)},
		{"reserved bits extension",
			[]byte{0x81, 0x01, 0x05, 0x00, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 1,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("enhanced", 1, tpdu.ErrNonZero)},
		{"unterminated extension",
			[]byte{0x81, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80},
			tpdu.VpfEnhanced, 7,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("enhanced", 7, tpdu.ErrUnderflow)},
		{"extension leaves no room",
			[]byte{0x83, 0x80, 0x80, 0x80, 0x80, 0x00, 0x10},
			tpdu.VpfEnhanced, 6,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("enhanced", 6, tpdu.ErrUnderflow)},
		// "A TP-VP value of zero is undefined and reserved for future use."
		{"reserved enhancedRelativeSeconds",
			[]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 1,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("enhanced", 1, tpdu.ErrInvalid)},
		// HHMMSS uses the representation of the SCTS, so minutes and seconds
		// are 00 to 59.
		{"minutes enhancedHHMMSS",
			[]byte{0x03, 0x00, 0x06, 0x00, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 1,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("enhanced", 1, tpdu.ErrInvalid)},
		{"seconds enhancedHHMMSS",
			[]byte{0x03, 0x00, 0x00, 0x99, 0x00, 0x00, 0x00},
			tpdu.VpfEnhanced, 1,
			tpdu.ValidityPeriod{},
			tpdu.NewDecodeError("enhanced", 1, tpdu.ErrInvalid)},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			s := tpdu.ValidityPeriod{}
			n, err := s.UnmarshalBinary(p.in, p.vpf)
			if err != p.err {
				t.Errorf("error decoding '%v': %v", p.in, err)
			}
			if n != p.readCount {
				t.Errorf("expected to read %d characters, read %d", p.readCount, n)
			}
			assert.Equal(t, p.out, s)
		}
		t.Run(p.name, f)
	}
}

func TestVPEnhancedExtension(t *testing.T) {
	// 3GPP TS 23.040 Section 9.2.3.12.3: with bit 7 of the functionality
	// indicator set, the next octet is an extension of the indicator, and the
	// VP value follows the last indicator octet.
	patterns := []struct {
		name     string
		in       []byte
		efi      byte
		duration time.Duration
	}{
		{"relative",
			[]byte{0x81, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00},
			0x81,
			30 * time.Minute,
		},
		{"relative chained",
			[]byte{0xc1, 0x80, 0x00, 0xff, 0x00, 0x00, 0x00},
			0xc1,
			63 * 7 * 24 * time.Hour,
		},
		{"seconds",
			[]byte{0x82, 0x00, 0x2d, 0x00, 0x00, 0x00, 0x00},
			0x82,
			45 * time.Second,
		},
		{"hhmmss",
			[]byte{0x83, 0x80, 0x80, 0x00, 0x10, 0x20, 0x30},
			0x83,
			time.Hour + 2*time.Minute + 3*time.Second,
		},
		{"not present",
			[]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			0x80,
			0,
		},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			var v tpdu.ValidityPeriod
			n, err := v.UnmarshalBinary(p.in, tpdu.VpfEnhanced)
			require.Nil(t, err)
			assert.Equal(t, 7, n)
			assert.Equal(t, tpdu.VpfEnhanced, v.Format)
			assert.Equal(t, p.efi, v.EFI)
			assert.Equal(t, p.duration, v.Duration)
			b, err := v.MarshalBinary()
			require.Nil(t, err)
			assert.Equal(t, p.in, b)
		}
		t.Run(p.name, f)
	}
}

func TestVPEnhancedMarshalFI(t *testing.T) {
	patterns := []struct {
		name string
		efi  byte
		out  []byte
		err  error
	}{
		{"single shot", 0x41, []byte{0x41, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, nil},
		{"reserved bit 3", 0x09, nil, tpdu.EncodeError("fi", tpdu.ErrInvalid)},
		{"reserved bit 5", 0x21, nil, tpdu.EncodeError("fi", tpdu.ErrInvalid)},
		// the extension octets are not known
		{"extension", 0x81, nil, tpdu.EncodeError("fi", tpdu.ErrInvalid)},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			var v tpdu.ValidityPeriod
			v.SetEnhanced(5*time.Minute, p.efi)
			b, err := v.MarshalBinary()
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, b)
		}
		t.Run(p.name, f)
	}
	// SetEnhanced drops the extension octets of a decoded VP
	var v tpdu.ValidityPeriod
	_, err := v.UnmarshalBinary([]byte{0x81, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00}, tpdu.VpfEnhanced)
	require.Nil(t, err)
	v.SetEnhanced(5*time.Minute, 0x01)
	b, err := v.MarshalBinary()
	require.Nil(t, err)
	assert.Equal(t, []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, b)
	v.SetEnhanced(5*time.Minute, 0x81)
	b, err = v.MarshalBinary()
	assert.Equal(t, tpdu.EncodeError("fi", tpdu.ErrInvalid), err)
	assert.Nil(t, b)
}

// FuzzValidityPeriodUnmarshalBinary checks that a VP decoded in the relative
// or enhanced format marshals back to the octets it was decoded from.
func FuzzValidityPeriodUnmarshalBinary(f *testing.F) {
	for _, seed := range [][]byte{
		{0x00}, {0x8f}, {0x90}, {0xa7}, {0xc4}, {0xff},
	} {
		f.Add(byte(tpdu.VpfRelative), seed)
	}
	for _, seed := range [][]byte{
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x41, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x02, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x03, 0x30, 0x21, 0x54, 0x00, 0x00, 0x00},
		{0x03, 0x99, 0x95, 0x95, 0x00, 0x00, 0x00},
		{0x81, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00},
		{0x83, 0x80, 0x80, 0x00, 0x10, 0x20, 0x30},
		{0x09, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x07, 0x01, 0x2d, 0x54, 0x00, 0x00, 0x00},
	} {
		f.Add(byte(tpdu.VpfEnhanced), seed)
	}
	f.Add(byte(tpdu.VpfNotPresent), []byte{})
	f.Fuzz(func(t *testing.T, vpf byte, src []byte) {
		format := tpdu.ValidityPeriodFormat(vpf % 3) // not absolute
		var v tpdu.ValidityPeriod
		n, err := v.UnmarshalBinary(src, format)
		if err != nil {
			return
		}
		if n > len(src) {
			t.Fatalf("%v % x: read %d octets", format, src, n)
		}
		b, err := v.MarshalBinary()
		if err != nil {
			t.Fatalf("%v % x: marshal error %v", format, src, err)
		}
		if !bytes.Equal(src[:n], b) {
			t.Fatalf("%v % x: remarshalled to % x", format, src[:n], b)
		}
	})
}

func TestVPRelativeRoundTrip(t *testing.T) {
	// Every TP-VP value in 3GPP TS 23.040 Section 9.2.3.12.1 is a distinct
	// period, so decoding and re-encoding must reproduce it, in both the
	// relative format and the relative sub-format of the enhanced format.
	for i := 0; i < 256; i++ {
		var v tpdu.ValidityPeriod
		n, err := v.UnmarshalBinary([]byte{byte(i)}, tpdu.VpfRelative)
		require.Nil(t, err)
		require.Equal(t, 1, n)
		b, err := v.MarshalBinary()
		require.Nil(t, err)
		assert.Equal(t, []byte{byte(i)}, b, "relative %d (%v)", i, v.Duration)

		e := []byte{byte(tpdu.EvpfRelative), byte(i), 0, 0, 0, 0, 0}
		n, err = v.UnmarshalBinary(e, tpdu.VpfEnhanced)
		require.Nil(t, err)
		require.Equal(t, 7, n)
		b, err = v.MarshalBinary()
		require.Nil(t, err)
		assert.Equal(t, e, b, "enhanced relative %d (%v)", i, v.Duration)
	}
}

func TestVPFString(t *testing.T) {
	patterns := []struct {
		vpf tpdu.ValidityPeriodFormat
		out string
	}{
		{0, "Not Present"},
		{1, "Enhanced"},
		{2, "Relative"},
		{3, "Absolute"},
		{4, "Unknown"},
	}
	for _, p := range patterns {
		assert.Equal(t, p.out, p.vpf.String())
	}
}

func TestEVPFString(t *testing.T) {
	patterns := []struct {
		evpf tpdu.EnhancedValidityPeriodFormat
		out  string
	}{
		{0, "Not Present"},
		{1, "Relative"},
		{2, "Relative Seconds"},
		{3, "Relative HHMMSS"},
		{4, "Unknown"},
	}
	for _, p := range patterns {
		assert.Equal(t, p.out, p.evpf.String())
	}
}
