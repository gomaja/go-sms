// SPDX-License-Identifier: MIT

package tpdu_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/gomaja/go-sms/encoding/gsm7"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitUserData(t *testing.T) {
	ucs := []byte{0, 0x41, 0xd8, 0x3d, 0xde, 0, 0, 0x42}
	patterns := []struct {
		name  string
		ud    []byte
		alpha tpdu.Alphabet
		limit int
		out   [][]byte
		err   error
	}{
		{"escape", []byte{0x41, 0x1b, 0x65, 0x42}, tpdu.Alpha7Bit, 2, [][]byte{{0x41}, {0x1b, 0x65}, {0x42}}, nil},
		{"double escape", []byte{0x41, 0x1b, 0x1b, 0x42}, tpdu.Alpha7Bit, 3, [][]byte{{0x41, 0x1b, 0x1b}, {0x42}}, nil},
		{"escape run", []byte{0x1b, 0x1b, 0x1b, 0x41}, tpdu.Alpha7Bit, 3, [][]byte{{0x1b, 0x1b}, {0x1b, 0x41}}, nil},
		{"invalid septet", []byte{0x41, 0x80}, tpdu.Alpha7Bit, 2, nil, gsm7.ErrInvalidSeptet{Offset: 1, Septet: 0x80}},
		{"invalid septet before size", []byte{0x1b, 0x80}, tpdu.Alpha7Bit, 1, nil, gsm7.ErrInvalidSeptet{Offset: 1, Septet: 0x80}},
		{"escape too long", []byte{0x1b}, tpdu.Alpha7Bit, 1, nil, tpdu.ErrOverlength},
		{"surrogate", ucs, tpdu.AlphaUCS2, 4, [][]byte{{0, 0x41}, {0xd8, 0x3d, 0xde, 0}, {0, 0x42}}, nil},
		{"odd limit", ucs, tpdu.AlphaUCS2, 5, [][]byte{{0, 0x41}, {0xd8, 0x3d, 0xde, 0}, {0, 0x42}}, nil},
		{"surrogate too long", ucs, tpdu.AlphaUCS2, 3, nil, tpdu.ErrOverlength},
		{"odd length", []byte{0, 0x41, 0}, tpdu.AlphaUCS2, 2, nil, tpdu.ErrOddUCS2Length},
		{"odd length before size", []byte{0, 0x41, 0}, tpdu.AlphaUCS2, 1, nil, tpdu.ErrOddUCS2Length},
		{"unit too long", []byte{0, 0x41}, tpdu.AlphaUCS2, 1, nil, tpdu.ErrOverlength},
		{"large limit", bytes.Repeat([]byte{0x11}, 401), tpdu.Alpha8Bit, 200, [][]byte{bytes.Repeat([]byte{0x11}, 200), bytes.Repeat([]byte{0x11}, 200), {0x11}}, nil},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			original := bytes.Clone(p.ud)
			// Spare capacity also exercises the last chunk's capacity limit.
			ud := append(bytes.Clone(p.ud), 0xaa)[:len(p.ud)]
			chunks, err := tpdu.SplitUserData(ud, p.alpha, p.limit)
			if p.err != nil {
				require.ErrorIs(t, err, p.err)
				require.Equal(t, tpdu.NewEncodeError("sm", p.err), err)
				var want gsm7.ErrInvalidSeptet
				if errors.As(p.err, &want) {
					var got gsm7.ErrInvalidSeptet
					require.ErrorAs(t, err, &got)
					assert.Equal(t, want, got)
				}
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, p.out, chunks)
			offset := 0
			for _, c := range chunks {
				assert.Equal(t, len(c), cap(c))
				assert.Same(t, &ud[offset], &c[0])
				offset += len(c)
				appended := append(c, 0xee)
				assert.Equal(t, byte(0xee), appended[len(c)])
			}
			assert.Equal(t, original, ud)
			assert.Equal(t, byte(0xaa), ud[:cap(ud)][len(ud)])
		})
	}
	t.Run("uncapped count", func(t *testing.T) {
		chunks, err := tpdu.SplitUserData(bytes.Repeat([]byte{0x11}, 300), tpdu.Alpha8Bit, 1)
		require.NoError(t, err)
		require.Len(t, chunks, 300)
		for _, c := range chunks {
			assert.Equal(t, []byte{0x11}, c)
			assert.Equal(t, 1, cap(c))
		}
	})
}

func TestSplitUserDataArguments(t *testing.T) {
	for _, alpha := range []tpdu.Alphabet{tpdu.Alpha7Bit, tpdu.Alpha8Bit, tpdu.AlphaUCS2, tpdu.AlphaReserved, tpdu.Alphabet(-1)} {
		for _, ud := range [][]byte{nil, {}, {0, 0}, {0xff}} {
			for _, limit := range []int{-1, 0, 1} {
				t.Run(fmt.Sprintf("%d/%x/%d", alpha, ud, limit), func(t *testing.T) {
					chunks, err := tpdu.SplitUserData(ud, alpha, limit)
					if alpha == tpdu.AlphaReserved || alpha == tpdu.Alphabet(-1) || limit <= 0 {
						require.ErrorIs(t, err, tpdu.ErrInvalid)
						assert.Equal(t, tpdu.ErrInvalid, err)
						assert.Nil(t, chunks)
					} else if len(ud) == 0 {
						require.NoError(t, err)
						assert.Nil(t, chunks)
					}
				})
			}
		}
	}
}

func TestSplitUserDataSegment(t *testing.T) {
	for _, p := range []struct {
		name  string
		alpha tpdu.Alphabet
		dcs   tpdu.DCS
		msg   []byte
	}{
		{"7bit", tpdu.Alpha7Bit, 0, append(bytes.Repeat([]byte{0x41}, 152), bytes.Repeat([]byte{0x1b, 0x65, 0x1b, 0x1b, 0x1b, 0x41}, 60)...)},
		{"8bit", tpdu.Alpha8Bit, tpdu.Dcs8BitData, bytes.Repeat([]byte{0x11}, 300)},
		{"ucs2", tpdu.AlphaUCS2, tpdu.DcsUCS2Data, append(bytes.Repeat([]byte{0, 0x41}, 66), bytes.Repeat([]byte{0xd8, 0x3d, 0xde, 0}, 100)...)},
	} {
		t.Run(p.name, func(t *testing.T) {
			tmpl := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 1, DCS: p.dcs}
			original := bytes.Clone(p.msg)
			pdus, err := tmpl.Segment(p.msg)
			require.NoError(t, err)
			require.Greater(t, len(pdus), 1)
			chunks, err := tpdu.SplitUserData(p.msg, p.alpha, pdus[0].UDBlockSize())
			require.NoError(t, err)
			require.Len(t, chunks, len(pdus))
			snapshots := make([][]byte, len(pdus))
			for i, pdu := range pdus {
				assert.Equal(t, chunks[i], []byte(pdu.UD))
				snapshots[i] = bytes.Clone(pdu.UD)
			}
			for _, pdu := range pdus {
				assert.Equal(t, len(pdu.UD), cap(pdu.UD))
				appended := append(pdu.UD, 0xee)
				assert.Equal(t, byte(0xee), appended[len(pdu.UD)])
				for j, other := range pdus {
					assert.Equal(t, snapshots[j], []byte(other.UD))
				}
				assert.Equal(t, original, p.msg)
			}
		})
	}
}

func TestSegmentSingleCapacity(t *testing.T) {
	for _, dcs := range []tpdu.DCS{0, tpdu.Dcs8BitData, tpdu.DcsUCS2Data} {
		t.Run(fmt.Sprint(dcs), func(t *testing.T) {
			msg := make([]byte, 4, 20)
			all := msg[:cap(msg)]
			for i := range all {
				all[i] = 0x11
			}
			original := bytes.Clone(all)
			tmpl := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 1, DCS: dcs}
			pdus, err := tmpl.Segment(msg)
			require.NoError(t, err)
			require.Len(t, pdus, 1)
			assert.Same(t, &msg[0], &pdus[0].UD[0])
			assert.Equal(t, len(msg), cap(pdus[0].UD))
			appended := append(pdus[0].UD, 0xee)
			assert.Equal(t, byte(0xee), appended[len(msg)])
			assert.Equal(t, original, all)
		})
	}
}

func FuzzSplitUserData(f *testing.F) {
	for _, seed := range []struct {
		ud    []byte
		alpha byte
		limit int16
	}{
		{[]byte{0x41, 0x1b, 0x65, 0x42}, 0, 1},
		{[]byte{0x1b, 0x1b, 0x1b, 0x41}, 0, 2},
		{[]byte{0x1b}, 0, 0},
		{bytes.Repeat([]byte{0x11}, 401), 1, 199},
		{[]byte{0, 0x41, 0xd8, 0x3d, 0xde, 0, 0, 0x42}, 2, 3},
		{[]byte{0xd8, 0, 0xd8, 0, 0xd8, 0}, 2, 3},
		{[]byte{0, 0x41}, 2, 0},
		{nil, 2, 0},
	} {
		f.Add(seed.ud, seed.alpha, seed.limit)
	}
	f.Fuzz(func(t *testing.T, ud []byte, alpha byte, limit int16) {
		coding := []tpdu.Alphabet{tpdu.Alpha7Bit, tpdu.Alpha8Bit, tpdu.AlphaUCS2}[alpha%3]
		ud = bytes.Clone(ud)
		if coding == tpdu.Alpha7Bit {
			for i := range ud {
				ud[i] &= 0x7f
			}
		}
		if coding == tpdu.AlphaUCS2 {
			ud = ud[:len(ud)&^1]
		}
		size := int(uint16(limit)%300) + 1
		original := bytes.Clone(ud)
		chunks, err := tpdu.SplitUserData(ud, coding, size)
		assert.Equal(t, original, ud)
		if err != nil {
			require.ErrorIs(t, err, tpdu.ErrOverlength)
			require.Equal(t, tpdu.NewEncodeError("sm", tpdu.ErrOverlength), err)
			require.Nil(t, chunks)
			tooSmall := coding == tpdu.Alpha7Bit && size < 2 && bytes.Contains(ud, []byte{0x1b})
			if coding == tpdu.AlphaUCS2 && len(ud) > 0 {
				tooSmall = size < 2
				for i := 0; i < len(ud); i += 2 {
					if ud[i]&0xfc == 0xd8 && size < 4 {
						tooSmall = true
					}
				}
			}
			require.True(t, tooSmall)
			return
		}
		var joined []byte
		offset := 0
		boundaries := map[int]bool{0: true}
		for i := 0; i < len(ud); {
			if coding == tpdu.Alpha7Bit && ud[i] == 0x1b && i+1 < len(ud) {
				i += 2
			} else {
				i++
			}
			boundaries[i] = true
		}
		for i, c := range chunks {
			require.NotEmpty(t, c)
			maxSize := size
			if coding == tpdu.AlphaUCS2 {
				maxSize &^= 1
				require.Zero(t, len(c)%2)
			}
			require.LessOrEqual(t, len(c), maxSize)
			require.Equal(t, len(c), cap(c))
			require.Same(t, &ud[offset], &c[0])
			offset += len(c)
			if coding == tpdu.Alpha7Bit {
				require.True(t, boundaries[offset])
			}
			if coding == tpdu.AlphaUCS2 && i+1 < len(chunks) {
				// A valid surrogate pair must not cross a chunk boundary.
				require.False(t, c[len(c)-2]&0xfc == 0xd8 && ud[offset]&0xfc == 0xdc)
			}
			joined = append(joined, c...)
		}
		require.True(t, bytes.Equal(ud, joined))
	})
}
