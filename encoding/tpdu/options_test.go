// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tpdu_test

import (
	"testing"

	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithDA(t *testing.T) {
	addr := tpdu.NewAddress(tpdu.FromNumber("12345"))
	s, err := tpdu.New(tpdu.WithDA(addr))
	require.Nil(t, err)
	assert.Equal(t, addr, s.DA)
}

func TestWithOA(t *testing.T) {
	addr := tpdu.NewAddress(tpdu.FromNumber("12345"))
	s, err := tpdu.New(tpdu.WithOA(addr))
	require.Nil(t, err)
	assert.Equal(t, addr, s.OA)
}

func TestWithUDH(t *testing.T) {
	udh := tpdu.UserDataHeader{
		tpdu.InformationElement{ID: 0, Data: []byte{3, 2, 1}},
	}
	s, err := tpdu.New(tpdu.WithUDH(udh))
	require.Nil(t, err)
	assert.Equal(t, udh, s.UDH)
	// TS 23.040 9.2.3.23: the TP-UDHI indicates the header.
	assert.True(t, s.UDHI())
	assert.True(t, s.PI.UDL())

	s, err = tpdu.NewSubmit(tpdu.WithUDH(udh), tpdu.Dcs8BitData)
	require.Nil(t, err)
	s.UD = []byte("hi")
	b, err := s.MarshalBinary()
	require.Nil(t, err)
	d := tpdu.TPDU{Direction: tpdu.MO}
	require.Nil(t, d.UnmarshalBinary(b))
	assert.Equal(t, udh, d.UDH)
	assert.Equal(t, []byte("hi"), []byte(d.UD))
	assert.Equal(t, s.FirstOctet, d.FirstOctet)

	s, err = tpdu.New(tpdu.WithUDH(udh), tpdu.WithUDH(nil))
	require.Nil(t, err)
	assert.Nil(t, s.UDH)
	assert.False(t, s.UDHI())
}

func TestWithMTI(t *testing.T) {
	s, err := tpdu.New(tpdu.MtSubmit)
	require.Nil(t, err)
	assert.Equal(t, tpdu.MtSubmit, s.MTI())
	s, err = tpdu.New(tpdu.MtReserved)
	require.Nil(t, err)
	assert.Equal(t, tpdu.MtReserved, s.MTI())
	// TS 23.040 9.2.3.1: the TP-MTI is a 2-bit field.
	for _, mti := range []tpdu.MessageType{-1, 4, 5} {
		s, err = tpdu.New(mti)
		assert.Equal(t, tpdu.ErrInvalid, err)
		assert.Nil(t, s)
	}
}

func TestWithDirection(t *testing.T) {
	s, err := tpdu.New(tpdu.MO)
	require.Nil(t, err)
	assert.Equal(t, tpdu.MO, s.Direction)
}
