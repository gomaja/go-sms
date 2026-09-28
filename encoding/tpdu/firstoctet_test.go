// SPDX-License-Identifier: MIT

package tpdu_test

import (
	"testing"

	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
)

func TestFirstOctetLP(t *testing.T) {
	assert.False(t, tpdu.FirstOctet(0).LP())
	assert.True(t, tpdu.FirstOctet(tpdu.FoLP).LP())
}

func TestFirstOctetMMS(t *testing.T) {
	assert.False(t, tpdu.FirstOctet(0).MMS())
	assert.True(t, tpdu.FirstOctet(tpdu.FoMMS).MMS())
}

func TestFirstMTI(t *testing.T) {
	patterns := []struct {
		inout tpdu.MessageType
	}{
		{tpdu.MtCommand},
		{tpdu.MtDeliver},
		{tpdu.MtSubmit},
		{tpdu.MtReserved},
	}
	for _, p := range patterns {
		assert.Equal(t, p.inout, tpdu.FirstOctet(p.inout).MTI())
	}
}
func TestFirstOctetRD(t *testing.T) {
	assert.False(t, tpdu.FirstOctet(0).RD())
	assert.True(t, tpdu.FirstOctet(tpdu.FoRD).RD())
}

func TestFirstOctetRP(t *testing.T) {
	assert.False(t, tpdu.FirstOctet(0).RP())
	assert.True(t, tpdu.FirstOctet(tpdu.FoRP).RP())
}

func TestFirstOctetSRI(t *testing.T) {
	assert.False(t, tpdu.FirstOctet(0).SRI())
	assert.True(t, tpdu.FirstOctet(tpdu.FoSRI).SRI())
}

func TestFirstOctetSRR(t *testing.T) {
	assert.False(t, tpdu.FirstOctet(0).SRR())
	assert.True(t, tpdu.FirstOctet(tpdu.FoSRR).SRR())
}

func TestFirstOctetSRQ(t *testing.T) {
	assert.False(t, tpdu.FirstOctet(0).SRQ())
	assert.True(t, tpdu.FirstOctet(tpdu.FoSRQ).SRQ())
}

func TestFirstOctetUDHI(t *testing.T) {
	assert.False(t, tpdu.FirstOctet(0).UDHI())
	assert.True(t, tpdu.FirstOctet(tpdu.FoUDHI).UDHI())
}

func TestFirstOctetVPF(t *testing.T) {
	patterns := []struct {
		in  tpdu.FirstOctet
		out tpdu.ValidityPeriodFormat
	}{
		{0, tpdu.VpfNotPresent},
		{0x10, tpdu.VpfRelative},
		{0x18, tpdu.VpfAbsolute},
		{0x08, tpdu.VpfEnhanced},
	}
	for _, p := range patterns {
		fo := tpdu.FirstOctet(p.in)
		assert.Equal(t, p.out, fo.VPF())
	}
}

func TestFirstWithMTI(t *testing.T) {
	patterns := []struct {
		inout tpdu.MessageType
	}{
		{tpdu.MtCommand},
		{tpdu.MtDeliver},
		{tpdu.MtSubmit},
		{tpdu.MtReserved},
	}
	for _, p := range patterns {
		fo := tpdu.FirstOctet(0).WithMTI(p.inout)
		assert.Equal(t, p.inout, fo.MTI())
	}
}

func TestFirstOctetWithVPF(t *testing.T) {
	patterns := []struct {
		inout tpdu.ValidityPeriodFormat
	}{
		{tpdu.VpfNotPresent},
		{tpdu.VpfRelative},
		{tpdu.VpfAbsolute},
		{tpdu.VpfEnhanced},
	}
	for _, p := range patterns {
		assert.Equal(t, p.inout, tpdu.FirstOctet(0).WithVPF(p.inout).VPF())
	}
}

// TestFirstOctetWithMasksFields checks that WithMTI and WithVPF only change
// their own bits, TS 23.040 9.2.3.1 bits 1 and 0 and 9.2.3.3 bits 4 and 3,
// whatever value they are given.
func TestFirstOctetWithMasksFields(t *testing.T) {
	for _, fo := range []tpdu.FirstOctet{0x00, 0xff, 0xa5, 0x5a} {
		for mti := tpdu.MessageType(-1); mti < 9; mti++ {
			out := fo.WithMTI(mti)
			assert.Equal(t, fo&^tpdu.FoMTIMask, out&^tpdu.FoMTIMask, "mti %d fo %02x", mti, fo)
			assert.Equal(t, mti&0x3, out.MTI())
		}
		for vpf := tpdu.ValidityPeriodFormat(0); vpf < 16; vpf++ {
			out := fo.WithVPF(vpf)
			assert.Equal(t, fo&^tpdu.FoVPFMask, out&^tpdu.FoVPFMask, "vpf %d fo %02x", vpf, fo)
			assert.Equal(t, vpf&0x3, out.VPF())
		}
	}
}
