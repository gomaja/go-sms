// SPDX-License-Identifier: MIT

package tpdu_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserDataCapacity(t *testing.T) {
	patterns := []struct {
		header                int
		septets, octets, ucs2 int
	}{
		{0, 160, 140, 140},
		{1, 158, 139, 138},
		{6, 153, 134, 134},
		{7, 152, 133, 132},
		{9, 149, 131, 130},
		{10, 148, 130, 130},
		{12, 146, 128, 128},
		{13, 145, 127, 126},
		{139, 1, 1, 0},
		{140, 0, 0, 0},
	}
	for _, p := range patterns {
		for _, a := range []struct {
			alpha tpdu.Alphabet
			want  int
		}{{tpdu.Alpha7Bit, p.septets}, {tpdu.Alpha8Bit, p.octets}, {tpdu.AlphaUCS2, p.ucs2}} {
			t.Run(fmt.Sprintf("alphabet=%d/header=%d", a.alpha, p.header), func(t *testing.T) {
				got, err := tpdu.UserDataCapacity(a.alpha, p.header)
				require.NoError(t, err)
				assert.Equal(t, a.want, got)
			})
		}
	}
}

func TestUserDataCapacityErrors(t *testing.T) {
	patterns := []struct {
		alpha  tpdu.Alphabet
		header int
		err    error
		field  string
	}{
		{tpdu.Alpha7Bit, -1, tpdu.ErrInvalid, "udh"},
		{tpdu.Alpha8Bit, -1, tpdu.ErrInvalid, "udh"},
		{tpdu.AlphaUCS2, -1, tpdu.ErrInvalid, "udh"},
		{tpdu.Alpha7Bit, 141, tpdu.ErrOverlength, "udh"},
		{tpdu.Alpha8Bit, 141, tpdu.ErrOverlength, "udh"},
		{tpdu.AlphaUCS2, 141, tpdu.ErrOverlength, "udh"},
		{tpdu.AlphaReserved, 0, tpdu.ErrInvalid, ""},
		{tpdu.Alphabet(-1), 0, tpdu.ErrInvalid, ""},
	}
	for _, p := range patterns {
		t.Run(fmt.Sprintf("alphabet=%d/header=%d", p.alpha, p.header), func(t *testing.T) {
			got, err := tpdu.UserDataCapacity(p.alpha, p.header)
			assert.Zero(t, got)
			assert.True(t, errors.Is(err, p.err), "got %v, want %v", err, p.err)
			if p.field != "" {
				var ee tpdu.EncodeError
				require.True(t, errors.As(err, &ee), "got %v", err)
				assert.Equal(t, p.field, ee.Field)
			} else {
				assert.Equal(t, p.err, err)
			}
		})
	}
}

func TestUserDataCapacityUDBlockSize(t *testing.T) {
	for _, st := range []tpdu.SmsType{tpdu.SmsSubmit, tpdu.SmsDeliver} {
		for _, p := range []struct {
			dcs   tpdu.DCS
			alpha tpdu.Alphabet
		}{
			{0x00, tpdu.Alpha7Bit},
			{tpdu.Dcs8BitData, tpdu.Alpha8Bit},
			{tpdu.DcsUCS2Data, tpdu.AlphaUCS2},
			{0x20, tpdu.Alpha8Bit},
			{0x24, tpdu.Alpha8Bit},
			{0x28, tpdu.Alpha8Bit},
		} {
			t.Run(fmt.Sprintf("%s/dcs=%02x", st, byte(p.dcs)), func(t *testing.T) {
				for h := 0; h <= 140; h++ {
					if h == 2 {
						continue // No well-formed UDH takes two octets.
					}
					var udh tpdu.UserDataHeader
					if h == 1 {
						udh = tpdu.UserDataHeader{}
					} else if h >= 3 {
						udh = tpdu.UserDataHeader{{ID: 0x7f, Data: make([]byte, h-3)}}
					}
					d, err := tpdu.New(st, p.dcs)
					require.NoError(t, err)
					d.SetUDH(udh)
					got, err := tpdu.UserDataCapacity(p.alpha, h)
					require.NoError(t, err)
					assert.Equal(t, d.UDBlockSize(), got, "header %d", h)
				}
			})
		}
	}
}
