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

func checkIEContract[T comparable](t *testing.T, decode func(tpdu.InformationElement) (T, error), lengths map[byte]int) {
	t.Helper()
	var zero T
	for id := 0; id < 256; id++ {
		for n := 0; n < 256; n++ {
			data := make([]byte, n)
			for i := range data {
				data[i] = 255
			}
			// Exercise nil as well as non-nil empty data.
			if n == 0 {
				data = nil
			}
			got, err := decode(tpdu.InformationElement{ID: byte(id), Data: data})
			length, ok := lengths[byte(id)]
			var want error
			field, offset := "ied", 2
			switch {
			case !ok:
				want, field, offset = tpdu.ErrInvalid, "iei", 0
			case n < length:
				want = tpdu.ErrUnderflow
			case n > length:
				want = tpdu.ErrOverlength
			}
			if want == nil {
				require.NoError(t, err)
				continue
			}
			var de tpdu.DecodeError
			if !errors.Is(err, want) || !errors.As(err, &de) || de.Field != field || de.Offset != offset || got != zero {
				t.Fatalf("id=%d length=%d: got %+v, %v; want zero, %s at %d: %v", id, n, got, err, field, offset, want)
			}
		}
	}
	for id := range lengths {
		got, err := decode(tpdu.InformationElement{ID: id, Data: []byte{}})
		require.ErrorIs(t, err, tpdu.ErrUnderflow)
		assert.Equal(t, zero, got)
	}
}

func TestInformationElementPortAddressAllIDsAndLengths(t *testing.T) {
	checkIEContract(t, tpdu.InformationElement.PortAddress, map[byte]int{4: 2, 5: 4})
}
func TestInformationElementSpecialMessageIndicationAllIDsAndLengths(t *testing.T) {
	checkIEContract(t, tpdu.InformationElement.SpecialMessageIndication, map[byte]int{1: 2})
}
func TestInformationElementSMSCControlParametersAllIDsAndLengths(t *testing.T) {
	checkIEContract(t, tpdu.InformationElement.SMSCControlParameters, map[byte]int{6: 1})
}
func TestInformationElementUDHSourceAllIDsAndLengths(t *testing.T) {
	checkIEContract(t, tpdu.InformationElement.UDHSource, map[byte]int{7: 1})
}

func checkPort(t *testing.T, ie tpdu.InformationElement, dest, orig int, wide, valid bool) {
	t.Helper()
	want := tpdu.PortAddress{Dest: dest, Orig: orig, Port16Bit: wide}
	got, err := ie.PortAddress()
	if err != nil || got != want || got.Valid() != valid {
		t.Fatalf("%x: got %+v, %v; want %+v valid=%t", ie.Data, got, err, want, valid)
	}
	header, ok := (tpdu.UserDataHeader{ie}).PortAddress()
	if !valid {
		want = tpdu.PortAddress{}
	}
	if header != want || ok != valid {
		t.Fatalf("%x: header %+v, %t; want %+v, %t", ie.Data, header, ok, want, valid)
	}
}

func TestPortAddressAllValues(t *testing.T) {
	// 3GPP TS 23.040 Sections 9.2.3.24.3 and 9.2.3.24.4 port tables.
	for dest := 0; dest < 256; dest++ {
		for orig := 0; orig < 256; orig++ {
			checkPort(t, tpdu.InformationElement{ID: 4, Data: []byte{byte(dest), byte(orig)}}, dest, orig, false, dest >= 240 && orig >= 240)
		}
	}
	for port := 0; port < 65536; port++ {
		for _, other := range []int{0, 49152, 49153, 65535} {
			for _, pair := range [][2]int{{port, other}, {other, port}} {
				dest, orig := pair[0], pair[1]
				checkPort(t, tpdu.InformationElement{ID: 5, Data: []byte{byte(dest / 256), byte(dest % 256), byte(orig / 256), byte(orig % 256)}}, dest, orig, true, dest <= 49152 && orig <= 49152)
			}
		}
	}
}

func TestSpecialMessageIndicationAllValues(t *testing.T) {
	// 3GPP TS 23.040 Section 9.2.3.24.2: bits 01, 432, 65, 7;
	// octet 2 is the message count, including 255 meaning 255 or greater.
	for first := 0; first < 256; first++ {
		for count := 0; count < 256; count++ {
			basic, extended := first%4, (first/4)%8
			want := tpdu.SpecialMessageIndication{Type: tpdu.SpecialMessageType(basic), ExtendedType: extended, Profile: (first/32)%4 + 1, Store: first >= 128, Count: count}
			valid := extended == 0 || basic == 3 && extended == 1
			ie := tpdu.InformationElement{ID: 1, Data: []byte{byte(first), byte(count)}}
			got, err := ie.SpecialMessageIndication()
			if err != nil || got != want || got.Valid() != valid {
				t.Fatalf("%x: got %+v, %v; want %+v valid=%t", ie.Data, got, err, want, valid)
			}
			indications := (tpdu.UserDataHeader{ie}).SpecialMessageIndications()
			if len(indications) != 1 || indications[0] != want {
				t.Fatalf("%x: header %+v; want %+v", ie.Data, indications, want)
			}
		}
	}
}

func TestSMSCControlParametersAllValues(t *testing.T) {
	// 3GPP TS 23.040 Section 9.2.3.24.5, Selective Status Report.
	for b := 0; b < 256; b++ {
		want := tpdu.SMSCControlParameters{ReportCompleted: b&1 != 0, ReportPermanentError: b&2 != 0, ReportTemporaryErrorNoRetry: b&4 != 0, ReportTemporaryErrorRetry: b&8 != 0, Reserved: b & 48, CancelRemainingReports: b&64 != 0, IncludeOriginalUDH: b&128 != 0}
		ie := tpdu.InformationElement{ID: 6, Data: []byte{byte(b)}}
		got, err := ie.SMSCControlParameters()
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.Equal(t, b&48 == 0, got.Valid())
		header, ok := (tpdu.UserDataHeader{ie}).SMSCControlParameters()
		assert.True(t, ok)
		assert.Equal(t, want, header)
	}
}

func TestUDHSourceAllValues(t *testing.T) {
	// 3GPP TS 23.040 Section 9.2.3.24.6 defines only 01, 02 and 03.
	for b := 0; b < 256; b++ {
		ie := tpdu.InformationElement{ID: 7, Data: []byte{byte(b)}}
		got, err := ie.UDHSource()
		require.NoError(t, err)
		assert.Equal(t, tpdu.UDHSource(b), got)
		assert.Equal(t, b >= 1 && b <= 3, got.Valid())
		assert.Equal(t, []tpdu.UDHSection{{Source: tpdu.UDHSource(b)}}, (tpdu.UserDataHeader{ie}).Sections())
	}
}

func TestPortAddressValid(t *testing.T) {
	for _, wide := range []bool{false, true} {
		for _, port := range []int{-1, 0, 239, 240, 255, 256, 49151, 49152, 49153, 65535, 65536} {
			for _, swap := range []bool{false, true} {
				t.Run(fmt.Sprintf("wide=%t/port=%d/swap=%t", wide, port, swap), func(t *testing.T) {
					p := tpdu.PortAddress{Dest: port, Orig: 240, Port16Bit: wide}
					if swap {
						p.Dest, p.Orig = p.Orig, p.Dest
					}
					want := port >= 240 && port <= 255
					if wide {
						want = port >= 0 && port <= 49152
					}
					assert.Equal(t, want, p.Valid())
				})
			}
		}
	}
}

func TestSpecialMessageIndicationValid(t *testing.T) {
	for _, basic := range []int{-1, 0, 1, 2, 3, 4, 256} {
		for _, ext := range []int{-1, 0, 1, 2, 7, 8, 256} {
			for _, profile := range []int{-1, 0, 1, 4, 5, 256} {
				for _, count := range []int{-1, 0, 255, 256} {
					x := tpdu.SpecialMessageIndication{Type: tpdu.SpecialMessageType(basic), ExtendedType: ext, Profile: profile, Count: count}
					want := basic >= 0 && basic <= 3 && (ext == 0 || basic == 3 && ext == 1) && profile >= 1 && profile <= 4 && count >= 0 && count <= 255
					assert.Equal(t, want, x.Valid(), "%+v", x)
				}
			}
		}
	}
}

func TestSMSCControlParametersValid(t *testing.T) {
	for _, reserved := range []int{-1, 0, 1, 16, 32, 48, 64, 256} {
		assert.Equal(t, reserved == 0, (tpdu.SMSCControlParameters{Reserved: reserved}).Valid())
	}
}
func TestUDHSourceValid(t *testing.T) {
	for _, source := range []int{-1, 0, 1, 2, 3, 4, 255, 256} {
		assert.Equal(t, source >= 1 && source <= 3, tpdu.UDHSource(source).Valid())
	}
}

func TestUserDataHeaderPortAddress(t *testing.T) {
	p8 := tpdu.InformationElement{ID: 4, Data: []byte{240, 255}}
	p16 := tpdu.InformationElement{ID: 5, Data: []byte{11, 132, 35, 240}}
	patterns := []struct {
		name   string
		header tpdu.UserDataHeader
		want   tpdu.PortAddress
		ok     bool
	}{
		{"nil", nil, tpdu.PortAddress{}, false},
		{"empty", tpdu.UserDataHeader{}, tpdu.PortAddress{}, false},
		{"8 then 16", tpdu.UserDataHeader{p8, p16}, tpdu.PortAddress{Dest: 2948, Orig: 9200, Port16Bit: true}, true},
		{"16 then 8", tpdu.UserDataHeader{p16, p8}, tpdu.PortAddress{Dest: 240, Orig: 255}, true},
		{"last same", tpdu.UserDataHeader{p8, {ID: 4, Data: []byte{255, 240}}}, tpdu.PortAddress{Dest: 255, Orig: 240}, true},
		{"reserved later", tpdu.UserDataHeader{p8, {ID: 5, Data: []byte{192, 1, 0, 0}}}, tpdu.PortAddress{Dest: 240, Orig: 255}, true},
		{"malformed later", tpdu.UserDataHeader{p16, {ID: 5, Data: []byte{1}}, {ID: 4, Data: []byte{240, 240, 0}}, {ID: 9}}, tpdu.PortAddress{Dest: 2948, Orig: 9200, Port16Bit: true}, true},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			got, ok := p.header.PortAddress()
			assert.Equal(t, p.want, got)
			assert.Equal(t, p.ok, ok)
		})
	}
}

func TestUserDataHeaderSpecialMessageIndications(t *testing.T) {
	voice := tpdu.SpecialMessageIndication{Type: tpdu.SpecialMessageVoice, Profile: 1, Count: 4}
	fax := tpdu.SpecialMessageIndication{Type: tpdu.SpecialMessageFax, Profile: 1, Store: true, Count: 2}
	reserved := tpdu.SpecialMessageIndication{Type: tpdu.SpecialMessageVoice, ExtendedType: 7, Profile: 1}
	patterns := []struct {
		name   string
		header tpdu.UserDataHeader
		want   []tpdu.SpecialMessageIndication
	}{
		{"nil", nil, nil}, {"empty", tpdu.UserDataHeader{}, nil},
		{"spec example", tpdu.UserDataHeader{{ID: 1, Data: []byte{0, 4}}, {ID: 1, Data: []byte{129, 2}}}, []tpdu.SpecialMessageIndication{voice, fax}},
		{"reserved kept malformed skipped", tpdu.UserDataHeader{{ID: 1, Data: []byte{0, 4}}, {ID: 1}, {ID: 1, Data: []byte{1, 2, 3}}, {ID: 9}, {ID: 1, Data: []byte{28, 0}}}, []tpdu.SpecialMessageIndication{voice, reserved}},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) { assert.Equal(t, p.want, p.header.SpecialMessageIndications()) })
	}
}

func TestUserDataHeaderSMSCControlParameters(t *testing.T) {
	patterns := []struct {
		name   string
		header tpdu.UserDataHeader
		want   tpdu.SMSCControlParameters
		ok     bool
	}{
		{"nil", nil, tpdu.SMSCControlParameters{}, false}, {"empty", tpdu.UserDataHeader{}, tpdu.SMSCControlParameters{}, false},
		{"last reserved retained", tpdu.UserDataHeader{{ID: 6, Data: []byte{1}}, {ID: 6, Data: []byte{48}}}, tpdu.SMSCControlParameters{Reserved: 48}, true},
		{"malformed skipped", tpdu.UserDataHeader{{ID: 6, Data: []byte{1}}, {ID: 6}, {ID: 6, Data: []byte{1, 2}}, {ID: 9}}, tpdu.SMSCControlParameters{ReportCompleted: true}, true},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			got, ok := p.header.SMSCControlParameters()
			assert.Equal(t, p.want, got)
			assert.Equal(t, p.ok, ok)
		})
	}
}

func TestUserDataHeaderSections(t *testing.T) {
	a := tpdu.InformationElement{ID: 9, Data: []byte{42}}
	bad := tpdu.InformationElement{ID: 7, Data: []byte{1, 2}}
	patterns := []struct {
		name   string
		header tpdu.UserDataHeader
		want   []tpdu.UDHSection
	}{
		{"nil", nil, nil}, {"empty", tpdu.UserDataHeader{}, nil},
		{"no indicators", tpdu.UserDataHeader{a, bad}, []tpdu.UDHSection{{Elements: tpdu.UserDataHeader{a, bad}}}},
		{"leading consecutive reserved malformed", tpdu.UserDataHeader{a, {ID: 7, Data: []byte{1}}, {ID: 7, Data: []byte{255}}, bad, a, {ID: 7, Data: []byte{3}}}, []tpdu.UDHSection{{Elements: tpdu.UserDataHeader{a}}, {Source: 1}, {Source: 255, Elements: tpdu.UserDataHeader{bad, a}}, {Source: 3}}},
		{"first indicator", tpdu.UserDataHeader{{ID: 7, Data: []byte{2}}, a}, []tpdu.UDHSection{{Source: 2, Elements: tpdu.UserDataHeader{a}}}},
		{"short indicator", tpdu.UserDataHeader{{ID: 7}}, []tpdu.UDHSection{{Elements: tpdu.UserDataHeader{{ID: 7}}}}},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) { assert.Equal(t, p.want, p.header.Sections()) })
	}
}

func TestUserDataHeaderSectionsShareMemory(t *testing.T) {
	header := tpdu.UserDataHeader{{ID: 9, Data: []byte{42}}, {ID: 7, Data: []byte{1}}, {ID: 10, Data: []byte{43}}}
	sections := header.Sections()
	sections[0].Elements[0].ID = 11
	sections[1].Elements[0].Data[0] = 44
	assert.Equal(t, byte(11), header[0].ID)
	assert.Equal(t, byte(44), header[2].Data[0])
	// Appending to a section must not overwrite the following source indicator.
	sections[0].Elements = append(sections[0].Elements, tpdu.InformationElement{ID: 12})
	assert.Equal(t, byte(7), header[1].ID)
}
