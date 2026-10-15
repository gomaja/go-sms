// SPDX-License-Identifier: MIT

package gsm7_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/gomaja/go-sms/encoding/gsm7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test7BitCRReproducer(t *testing.T) {
	u := []byte("AAAAAAAA")
	p := []byte{0x82, 0xc1, 0x60, 0x30, 0x18, 0x0c, 0x06, 0x1b}
	got, err := gsm7.Pack7BitCR(u, 1)
	require.NoError(t, err)
	assert.Equal(t, p, got)
	assert.Equal(t, u, gsm7.Unpack7BitCR(p, 1))
}

// packCRBits builds the bit stream before dividing it into octets. Padding
// follows 3GPP TS 23.038 V20.0.0 Section 6.1.2.3.1, after the fill bits.
func packCRBits(u []byte, fill int) []byte {
	if len(u) == 0 {
		return nil
	}
	bits := make([]byte, fill)
	appendSeptet := func(s byte) {
		for b := 0; b < 7; b++ {
			bits = append(bits, (s>>b)&1)
		}
	}
	for _, s := range u {
		appendSeptet(s)
	}
	if len(bits)%8 == 1 || (len(bits)%8 == 0 && u[len(u)-1] == cr) {
		appendSeptet(cr)
	}
	for len(bits)%8 != 0 {
		bits = append(bits, 0)
	}
	p := make([]byte, len(bits)/8)
	for i, bit := range bits {
		p[i/8] |= bit << (i % 8)
	}
	return p
}

func TestPack7BitCRReference(t *testing.T) {
	for fill := 0; fill <= 6; fill++ {
		for n := 0; n <= 40; n++ {
			for _, last := range []byte{0, 1, 2, cr, 0x7f} {
				u := make([]byte, n)
				for i := range u {
					u[i] = byte((i*37 + n*11 + fill) % 128)
				}
				if n > 0 {
					u[n-1] = last
				}
				p, err := gsm7.Pack7BitCR(u, fill)
				require.NoError(t, err)
				assert.True(t, bytes.Equal(packCRBits(u, fill), p),
					"fill %d n %d last %#x: got % x want % x", fill, n, last, p, packCRBits(u, fill))
			}
		}
	}
}

func Test7BitCRRoundTrip(t *testing.T) {
	for fill := 0; fill <= 6; fill++ {
		for n := 0; n <= 40; n++ {
			for _, last := range []byte{0, 1, 2, cr, 0x7f} {
				u := make([]byte, n)
				for i := range u {
					u[i] = byte((i*53 + n*7 + fill) % 128)
				}
				if n > 0 {
					u[n-1] = last
				}
				want := bytes.Clone(u)
				if n > 0 && last == cr && (fill+7*n)%8 == 0 {
					want = append(want, cr)
				}
				p, err := gsm7.Pack7BitCR(u, fill)
				require.NoError(t, err)
				assert.Equal(t, want, gsm7.Unpack7BitCR(p, fill), "fill %d n %d last %#x", fill, n, last)
			}
		}
	}
}

func Test7BitCRUSSD(t *testing.T) {
	check := func(t *testing.T, u, p, unpacked []byte) {
		t.Helper()
		got, err := gsm7.Pack7BitCR(u, 0)
		require.NoError(t, err)
		assert.Equal(t, p, got)
		old, err := gsm7.Pack7BitUSSD(u)
		require.NoError(t, err)
		assert.Equal(t, old, got)
		assert.Equal(t, unpacked, gsm7.Unpack7BitCR(p, 0))
		assert.Equal(t, gsm7.Unpack7BitUSSD(p), gsm7.Unpack7BitCR(p, 0))
	}
	for _, p := range ussdTestPatterns {
		t.Run(p.name, func(t *testing.T) { check(t, p.u, p.p, p.u) })
	}
	for _, p := range ussdCRPatterns {
		t.Run(p.name, func(t *testing.T) { check(t, p.u, p.p, p.unpacked) })
	}
}

func Test7BitCREmpty(t *testing.T) {
	for fill := 0; fill <= 6; fill++ {
		for _, empty := range [][]byte{nil, {}} {
			p, err := gsm7.Pack7BitCR(empty, fill)
			require.NoError(t, err)
			assert.Equal(t, empty, p)
			assert.Equal(t, empty, gsm7.Unpack7BitCR(empty, fill))
		}
	}
}

func Test7BitCRInvalidFill(t *testing.T) {
	for _, fill := range []int{-1, 7} {
		for _, u := range [][]byte{nil, {}, {0x41}} {
			msg := fmt.Sprintf("gsm7: fillBits %d not in range 0..6", fill)
			assert.PanicsWithValue(t, msg, func() { _, _ = gsm7.Pack7BitCR(u, fill) })
			assert.PanicsWithValue(t, msg, func() { gsm7.Unpack7BitCR(u, fill) })
		}
	}
}

func TestPack7BitCRInvalidSeptet(t *testing.T) {
	for fill := 0; fill <= 6; fill++ {
		for _, u := range [][]byte{{0x80}, {0x41, 0xff}, {0x41, 0x80, 0xff}} {
			p, err := gsm7.Pack7BitCR(u, fill)
			var invalid gsm7.ErrInvalidSeptet
			require.ErrorAs(t, err, &invalid)
			offset := 0
			if u[0] == 0x41 {
				offset = 1
			}
			assert.Equal(t, gsm7.ErrInvalidSeptet{Offset: offset, Septet: u[offset]}, invalid)
			assert.Nil(t, p)
		}
	}
}

func FuzzPack7BitCR(f *testing.F) {
	for fill := byte(0); fill <= 6; fill++ {
		for n := 0; n <= 40; n++ {
			u := bytes.Repeat([]byte{0x41}, n)
			f.Add(bytes.Clone(u), fill)
			if n > 0 {
				u[n-1] = cr
				f.Add(u, fill)
			}
		}
	}
	f.Fuzz(func(t *testing.T, u []byte, f byte) {
		fill := int(f % 7)
		for i := range u {
			u[i] &= 0x7f
		}
		p, err := gsm7.Pack7BitCR(u, fill)
		require.NoError(t, err)
		if want := packCRBits(u, fill); !bytes.Equal(want, p) {
			t.Fatalf("fill %d pack % x: got % x want % x", fill, u, p, want)
		}
		want := bytes.Clone(u)
		if len(u) > 0 && u[len(u)-1] == cr && (fill+7*len(u))%8 == 0 {
			want = append(want, cr)
		}
		if got := gsm7.Unpack7BitCR(p, fill); !bytes.Equal(want, got) {
			t.Fatalf("fill %d pack % x -> % x -> unpack % x want % x", fill, u, p, got, want)
		}
	})
}
