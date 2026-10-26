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

func TestInformationElementEmailHeaderLengthAllIDsAndLengths(t *testing.T) {
	checkIEContract(t, tpdu.InformationElement.EmailHeaderLength, map[byte]int{0x20: 1})
}

func TestEmailHeaderLengthAllValues(t *testing.T) {
	assert.Equal(t, byte(0x20), tpdu.IEIEmailHeader)
	for n := 0; n < 256; n++ {
		got, err := (tpdu.InformationElement{ID: 0x20, Data: []byte{byte(n)}}).EmailHeaderLength()
		require.NoError(t, err)
		assert.Equal(t, n, got)
	}
}

func TestInformationElementHyperlinkAllIDsAndLengths(t *testing.T) {
	checkIEContract(t, tpdu.InformationElement.Hyperlink, map[byte]int{0x21: 4})
}

func TestHyperlinkAllValues(t *testing.T) {
	assert.Equal(t, byte(0x21), tpdu.IEIHyperlink)
	check := func(position, title, url int) {
		t.Helper()
		// Section 9.2.3.24.12 numbers the first character 1; both lengths
		// count characters and have no reserved values.
		ie := tpdu.InformationElement{ID: 0x21, Data: []byte{byte(position / 256), byte(position % 256), byte(title), byte(url)}}
		want := tpdu.Hyperlink{Position: position, TitleLength: title, URLLength: url}
		got, err := ie.Hyperlink()
		if err != nil || got != want || got.Valid() != (position >= 1) {
			t.Fatalf("%x: got %+v, %v; want %+v valid=%t", ie.Data, got, err, want, position >= 1)
		}
	}
	for position := 0; position <= 65535; position++ {
		for _, length := range []int{0, 1, 127, 255} {
			check(position, length, 255-length)
		}
	}
	for length := 0; length <= 255; length++ {
		for _, position := range []int{0, 1, 256, 65535} {
			check(position, length, 17)
			check(position, 23, length)
		}
	}
}

func TestHyperlinkValid(t *testing.T) {
	for _, position := range []int{-1, 0, 1, 65535, 65536} {
		for _, title := range []int{-1, 0, 255, 256} {
			for _, url := range []int{-1, 0, 255, 256} {
				got := tpdu.Hyperlink{Position: position, TitleLength: title, URLLength: url}
				assert.Equal(t, position >= 1 && position <= 65535 && title >= 0 && title <= 255 && url >= 0 && url <= 255, got.Valid(), "%+v", got)
			}
		}
	}
}

func TestReplyAddress(t *testing.T) {
	assert.Equal(t, byte(0x22), tpdu.IEIReplyAddress)
	for _, p := range []struct {
		name string
		data []byte
		want tpdu.Address
	}{
		{"numeric", []byte{3, 0x91, 0x21, 0xf3}, tpdu.Address{TOA: 0x91, Addr: "123"}},
		{"alphanumeric", []byte{2, 0xd0, 0x41}, tpdu.Address{TOA: 0xd0, Addr: "A"}},
		{"empty", []byte{0, 0x81}, tpdu.Address{TOA: 0x81}},
	} {
		t.Run(p.name, func(t *testing.T) {
			got, err := (tpdu.InformationElement{ID: 0x22, Data: p.data}).ReplyAddress()
			require.NoError(t, err)
			assert.Equal(t, p.want, got)
			for n := 0; n < len(p.data); n++ {
				got, err = (tpdu.InformationElement{ID: 0x22, Data: p.data[:n]}).ReplyAddress()
				assert.Equal(t, tpdu.Address{}, got)
				assert.Equal(t, tpdu.NewDecodeError("ied", 2, tpdu.ErrUnderflow), err, "length %d", n)
			}
			got, err = (tpdu.InformationElement{ID: 0x22, Data: append(append([]byte(nil), p.data...), 0)}).ReplyAddress()
			assert.Equal(t, tpdu.Address{}, got)
			assert.Equal(t, tpdu.NewDecodeError("ied", 2, tpdu.ErrOverlength), err)
		})
	}
}

func TestReplyAddressErrors(t *testing.T) {
	for id := 0; id < 256; id++ {
		if id == 0x22 {
			continue
		}
		got, err := (tpdu.InformationElement{ID: byte(id), Data: []byte{0, 0x81}}).ReplyAddress()
		assert.Equal(t, tpdu.Address{}, got)
		assert.Equal(t, tpdu.NewDecodeError("iei", 0, tpdu.ErrInvalid), err)
	}
	for _, data := range [][]byte{{21, 0x91}, {2, 0x91, 0xff}} {
		var address tpdu.Address
		_, addressErr := address.UnmarshalBinary(data)
		require.Error(t, addressErr)
		got, err := (tpdu.InformationElement{ID: 0x22, Data: data}).ReplyAddress()
		assert.Equal(t, tpdu.Address{}, got)
		assert.Equal(t, tpdu.NewDecodeError("ied", 2, addressErr), err)
	}
}

func TestUserDataHeaderEmailHyperlinkReply(t *testing.T) {
	for _, h := range []tpdu.UserDataHeader{nil, {}, {{ID: 0x20}, {ID: 0x21}, {ID: 0x22}}, {{ID: 0xff}}} {
		n, ok := h.EmailHeaderLength()
		assert.Zero(t, n)
		assert.False(t, ok)
		assert.Nil(t, h.Hyperlinks())
		a, ok := h.ReplyAddress()
		assert.Equal(t, tpdu.Address{}, a)
		assert.False(t, ok)
	}
	h := tpdu.UserDataHeader{
		{ID: 0x20, Data: []byte{3}}, {ID: 0x20, Data: []byte{0}}, {ID: 0x20}, {ID: 0x20, Data: []byte{1, 2}},
		{ID: 0x21, Data: []byte{0, 1, 2, 3}}, {ID: 0x21}, {ID: 0x21, Data: []byte{0, 0, 0, 0}}, {ID: 0x21, Data: []byte{1, 2, 3, 4, 5}},
		{ID: 0x22, Data: []byte{2, 0x81, 0x21}}, {ID: 0x22, Data: []byte{2, 0x81, 0x43}}, {ID: 0x22}, {ID: 0x22, Data: []byte{0, 0x81, 0}}, {ID: 0xff},
	}
	n, ok := h.EmailHeaderLength()
	assert.True(t, ok)
	assert.Zero(t, n)
	assert.Equal(t, []tpdu.Hyperlink{{Position: 1, TitleLength: 2, URLLength: 3}, {}}, h.Hyperlinks())
	a, ok := h.ReplyAddress()
	assert.True(t, ok)
	assert.Equal(t, tpdu.Address{TOA: 0x81, Addr: "34"}, a)
}

func ExampleUserDataHeader_Hyperlinks() {
	header := tpdu.UserDataHeader{{ID: tpdu.IEIHyperlink, Data: []byte{0, 1, 4, 19}}}
	for _, link := range header.Hyperlinks() {
		fmt.Printf("position %d, title %d characters, URL %d characters\n", link.Position, link.TitleLength, link.URLLength)
	}
	// Output: position 1, title 4 characters, URL 19 characters
}

// Each width is one parameter in table order, including whole address
// fields and conditional length/data parameters. It is also the independent
// oracle for the parameter cut short at every possible truncation.
type voiceMailVector struct {
	name   string
	data   []byte
	widths []int
	want   tpdu.EnhancedVoiceMail
	valid  bool
}

func voiceMailVectors() []voiceMailVector {
	return []voiceMailVector{
		{"zero notifications", []byte{0, 2, 0x91, 0x21, 0, 0}, []int{1, 3, 1, 1},
			tpdu.EnhancedVoiceMail{Profile: 1, AccessAddress: tpdu.Address{TOA: 0x91, Addr: "12"}}, true},
		{"two notifications", []byte{0x7c, 3, 0x91, 0x21, 0xf3, 255, 2,
			0x12, 0x34, 60, 0x47, 2, 0xd0, 0x41,
			0xff, 0xff, 255, 31, 0, 0x81}, []int{1, 4, 1, 1, 2, 1, 1, 3, 2, 1, 1, 2},
			tpdu.EnhancedVoiceMail{Profile: 4, Store: true, AlmostFull: true, Full: true, AccessAddress: tpdu.Address{TOA: 0x91, Addr: "123"}, VoiceMessages: 255,
				Notifications: []tpdu.VoiceMailNotification{
					{ID: 0x1234, Length: 60, RetentionDays: 7, Urgent: true, CallingLineIdentity: tpdu.Address{TOA: 0xd0, Addr: "A"}},
					{ID: 65535, Length: 255, RetentionDays: 31, CallingLineIdentity: tpdu.Address{TOA: 0x81}},
				}}, true},
		{"mailbox extension", []byte{0x80, 0, 0x81, 1, 0, 2, 0xaa, 0xbb}, []int{1, 2, 1, 1, 1, 2},
			tpdu.EnhancedVoiceMail{Profile: 1, AccessAddress: tpdu.Address{TOA: 0x81}, VoiceMessages: 1, MailboxExtensionIndicator: true, MailboxExtension: []byte{0xaa, 0xbb}}, false},
		{"notification extension", []byte{0, 0, 0x81, 1, 1, 0, 1, 2, 0x83, 0, 0x81, 2, 0xaa, 0xbb}, []int{1, 2, 1, 1, 2, 1, 1, 2, 1, 2},
			tpdu.EnhancedVoiceMail{Profile: 1, AccessAddress: tpdu.Address{TOA: 0x81}, VoiceMessages: 1,
				Notifications: []tpdu.VoiceMailNotification{{ID: 1, Length: 2, RetentionDays: 3, CallingLineIdentity: tpdu.Address{TOA: 0x81}, ExtensionIndicator: true, Extension: []byte{0xaa, 0xbb}}}}, false},
		{"empty extensions", []byte{0x80, 0, 0x81, 1, 1, 0, 0, 1, 2, 0x80, 0, 0x81, 0}, []int{1, 2, 1, 1, 1, 0, 2, 1, 1, 2, 1, 0},
			tpdu.EnhancedVoiceMail{Profile: 1, AccessAddress: tpdu.Address{TOA: 0x81}, VoiceMessages: 1, MailboxExtensionIndicator: true, MailboxExtension: []byte{},
				Notifications: []tpdu.VoiceMailNotification{{ID: 1, Length: 2, CallingLineIdentity: tpdu.Address{TOA: 0x81}, ExtensionIndicator: true, Extension: []byte{}}}}, false},
		{"zero deletes", []byte{1, 0, 0x81, 0, 0}, []int{1, 2, 1, 1},
			tpdu.EnhancedVoiceMail{DeleteConfirmation: true, Profile: 1, AccessAddress: tpdu.Address{TOA: 0x81}}, true},
		{"three deletes", []byte{0x15, 2, 0x81, 0x21, 2, 3, 0x12, 0x34, 0, 0xab, 0xcd, 0, 0, 0, 0}, []int{1, 3, 1, 1, 2, 1, 2, 1, 2, 1},
			tpdu.EnhancedVoiceMail{DeleteConfirmation: true, Profile: 2, Store: true, AccessAddress: tpdu.Address{TOA: 0x81, Addr: "12"}, VoiceMessages: 2,
				Deletes: []tpdu.VoiceMailDelete{{ID: 0x1234}, {ID: 0xabcd}, {}}}, true},
		{"delete extensions", []byte{0x89, 0, 0x81, 255, 2, 1, 0xaa, 0x12, 0x34, 0x80, 2, 0xbb, 0xcc, 0xab, 0xcd, 0}, []int{1, 2, 1, 1, 1, 1, 2, 1, 1, 2, 2, 1},
			tpdu.EnhancedVoiceMail{DeleteConfirmation: true, Profile: 3, AccessAddress: tpdu.Address{TOA: 0x81}, VoiceMessages: 255, MailboxExtensionIndicator: true, MailboxExtension: []byte{0xaa},
				Deletes: []tpdu.VoiceMailDelete{{ID: 0x1234, ExtensionIndicator: true, Extension: []byte{0xbb, 0xcc}}, {ID: 0xabcd}}}, false},
	}
}

func TestEnhancedVoiceMailVectors(t *testing.T) {
	assert.Equal(t, byte(0x23), tpdu.IEIEnhancedVoiceMail)
	for _, p := range voiceMailVectors() {
		t.Run(p.name, func(t *testing.T) {
			data := append([]byte(nil), p.data...)
			got, err := (tpdu.InformationElement{ID: 0x23, Data: data}).EnhancedVoiceMail()
			require.NoError(t, err)
			assert.Equal(t, p.want, got)
			assert.Equal(t, p.valid, got.Valid())
			// The extensions are copied, not shared with the IE data.
			for i := range data {
				data[i] = 0x5a
			}
			assert.Equal(t, p.want, got)
		})
	}
}

func TestEnhancedVoiceMailTruncationAndSurplus(t *testing.T) {
	for _, p := range voiceMailVectors() {
		t.Run(p.name, func(t *testing.T) {
			start, last := 0, 0
			for _, width := range p.widths {
				last = start
				for n := start; n < start+width; n++ {
					got, err := (tpdu.InformationElement{ID: 0x23, Data: p.data[:n]}).EnhancedVoiceMail()
					assert.Equal(t, tpdu.EnhancedVoiceMail{}, got, "length %d", n)
					assert.Equal(t, tpdu.NewDecodeError("ied", 2+start, tpdu.ErrUnderflow), err, "length %d", n)
				}
				start += width
			}
			require.Equal(t, len(p.data), start)
			for _, extra := range [][]byte{{0}, {0, 1, 2}} {
				got, err := (tpdu.InformationElement{ID: 0x23, Data: append(append([]byte(nil), p.data...), extra...)}).EnhancedVoiceMail()
				assert.Equal(t, tpdu.EnhancedVoiceMail{}, got)
				assert.Equal(t, tpdu.NewDecodeError("ied", 2+last, tpdu.ErrOverlength), err)
			}
		})
	}
}

func TestEnhancedVoiceMailAddressErrors(t *testing.T) {
	for _, p := range []struct {
		data   []byte
		offset int
	}{
		{[]byte{0, 21, 0x91}, 1},
		{[]byte{1, 2, 0x91, 0xff}, 1},
		{[]byte{0, 0, 0x81, 1, 1, 0, 1, 2, 3, 21, 0x91}, 9},
		{[]byte{0, 0, 0x81, 1, 1, 0, 1, 2, 3, 2, 0x91, 0xff}, 9},
	} {
		var a tpdu.Address
		_, addressErr := a.UnmarshalBinary(p.data[p.offset:])
		require.Error(t, addressErr)
		got, err := (tpdu.InformationElement{ID: 0x23, Data: p.data}).EnhancedVoiceMail()
		assert.Equal(t, tpdu.EnhancedVoiceMail{}, got)
		assert.Equal(t, tpdu.NewDecodeError("ied", 2+p.offset, addressErr), err)
	}
	for id := 0; id < 256; id++ {
		if id == 0x23 {
			continue
		}
		got, err := (tpdu.InformationElement{ID: byte(id)}).EnhancedVoiceMail()
		assert.Equal(t, tpdu.EnhancedVoiceMail{}, got)
		assert.Equal(t, tpdu.NewDecodeError("iei", 0, tpdu.ErrInvalid), err)
	}
}

func TestEnhancedVoiceMailProfilesAndFlags(t *testing.T) {
	for _, kind := range []byte{0, 1} {
		// Codes are written bit 2 first: 00, 10, 01, 11.
		for profile, code := range []byte{0x00, 0x04, 0x08, 0x0c} {
			for _, flags := range []byte{0, 0x10, 0x20, 0x40, 0x70} {
				got, err := (tpdu.InformationElement{ID: 0x23, Data: []byte{kind | code | flags, 0, 0x81, 0, 0}}).EnhancedVoiceMail()
				require.NoError(t, err)
				assert.Equal(t, tpdu.EnhancedVoiceMail{DeleteConfirmation: kind == 1, Profile: profile + 1, Store: flags&0x10 != 0, AlmostFull: flags&0x20 != 0, Full: flags&0x40 != 0, AccessAddress: tpdu.Address{TOA: 0x81}}, got)
				assert.True(t, got.Valid())
			}
		}
	}
}

func TestEnhancedVoiceMailReservedBits(t *testing.T) {
	for _, kind := range []byte{0, 1} {
		for _, p := range []struct {
			offset   int
			bit      byte
			reserved int
			count    int
		}{
			{0, 0x02, 0x02, 0}, {4, 0x20, 0, 0x20}, {4, 0x40, 0, 0x40}, {4, 0x80, 0, 0x80},
		} {
			data := []byte{kind, 0, 0x81, 0, 0}
			data[p.offset] |= p.bit
			got, err := (tpdu.InformationElement{ID: 0x23, Data: data}).EnhancedVoiceMail()
			require.NoError(t, err)
			assert.Equal(t, tpdu.EnhancedVoiceMail{DeleteConfirmation: kind == 1, Profile: 1, AccessAddress: tpdu.Address{TOA: 0x81}, Reserved: p.reserved, CountReserved: p.count}, got)
			assert.False(t, got.Valid())
		}
	}
	got, err := (tpdu.InformationElement{ID: 0x23, Data: []byte{0, 0, 0x81, 1, 1, 0, 1, 2, 0x20, 0, 0x81}}).EnhancedVoiceMail()
	require.NoError(t, err)
	assert.Equal(t, tpdu.EnhancedVoiceMail{Profile: 1, AccessAddress: tpdu.Address{TOA: 0x81}, VoiceMessages: 1, Notifications: []tpdu.VoiceMailNotification{{ID: 1, Length: 2, Reserved: 0x20, CallingLineIdentity: tpdu.Address{TOA: 0x81}}}}, got)
	assert.False(t, got.Valid())
	for bit := 0; bit < 7; bit++ {
		got, err = (tpdu.InformationElement{ID: 0x23, Data: []byte{1, 0, 0x81, 0, 1, 0, 1, 1 << bit}}).EnhancedVoiceMail()
		require.NoError(t, err)
		assert.Equal(t, tpdu.EnhancedVoiceMail{DeleteConfirmation: true, Profile: 1, AccessAddress: tpdu.Address{TOA: 0x81}, Deletes: []tpdu.VoiceMailDelete{{ID: 1, Reserved: 1 << bit}}}, got)
		assert.False(t, got.Valid())
	}
}

func TestEnhancedVoiceMailCounts(t *testing.T) {
	for _, kind := range []byte{0, 1} {
		for count := 0; count <= 31; count++ {
			data := []byte{kind, 0, 0x81, 255, byte(count)}
			want := tpdu.EnhancedVoiceMail{DeleteConfirmation: kind == 1, Profile: 1, AccessAddress: tpdu.Address{TOA: 0x81}, VoiceMessages: 255}
			for i := 0; i < count; i++ {
				if kind == 1 {
					data = append(data, 0, byte(i), 0)
					want.Deletes = append(want.Deletes, tpdu.VoiceMailDelete{ID: i})
				} else {
					data = append(data, 0, byte(i), 0, 0, 0, 0x81)
					want.Notifications = append(want.Notifications, tpdu.VoiceMailNotification{ID: i, CallingLineIdentity: tpdu.Address{TOA: 0x81}})
				}
			}
			got, err := (tpdu.InformationElement{ID: 0x23, Data: data}).EnhancedVoiceMail()
			require.NoError(t, err)
			assert.Equal(t, want, got)
			assert.Equal(t, kind == 1 || count <= 15, got.Valid())
		}
	}
}

func TestEnhancedVoiceMailValid(t *testing.T) {
	for _, profile := range []int{-1, 0, 1, 4, 5} {
		for _, count := range []int{-1, 0, 255, 256} {
			v := tpdu.EnhancedVoiceMail{Profile: profile, VoiceMessages: count}
			assert.Equal(t, profile >= 1 && profile <= 4 && count >= 0 && count <= 255, v.Valid(), "%+v", v)
		}
	}
	for _, n := range []int{0, 1, 15, 16, 31, 32, 63, 64} {
		v := tpdu.EnhancedVoiceMail{Profile: 1, Notifications: make([]tpdu.VoiceMailNotification, n)}
		assert.Equal(t, n <= 15, v.Valid())
		v = tpdu.EnhancedVoiceMail{Profile: 1, DeleteConfirmation: true, Deletes: make([]tpdu.VoiceMailDelete, n)}
		assert.Equal(t, n <= 63, v.Valid())
	}
	for _, id := range []int{-1, 0, 65535, 65536} {
		for _, length := range []int{-1, 0, 255, 256} {
			for _, days := range []int{-1, 0, 31, 32} {
				v := tpdu.EnhancedVoiceMail{Profile: 1, Notifications: []tpdu.VoiceMailNotification{{ID: id, Length: length, RetentionDays: days}}}
				assert.Equal(t, id >= 0 && id <= 65535 && length >= 0 && length <= 255 && days >= 0 && days <= 31, v.Valid())
			}
		}
		v := tpdu.EnhancedVoiceMail{Profile: 1, DeleteConfirmation: true, Deletes: []tpdu.VoiceMailDelete{{ID: id}}}
		assert.Equal(t, id >= 0 && id <= 65535, v.Valid())
	}
	for _, reserved := range []int{-1, 1, 2, 0x20, 0x2000, 0x10000} {
		assert.False(t, (tpdu.EnhancedVoiceMail{Profile: 1, Reserved: reserved}).Valid())
		assert.False(t, (tpdu.EnhancedVoiceMail{Profile: 1, CountReserved: reserved}).Valid())
		assert.False(t, (tpdu.EnhancedVoiceMail{Profile: 1, Notifications: []tpdu.VoiceMailNotification{{Reserved: reserved}}}).Valid())
		assert.False(t, (tpdu.EnhancedVoiceMail{Profile: 1, DeleteConfirmation: true, Deletes: []tpdu.VoiceMailDelete{{Reserved: reserved}}}).Valid())
	}
	for _, length := range []int{0, 1, 255, 256} {
		for _, indicator := range []bool{false, true} {
			ext := make([]byte, length)
			want := !indicator && length == 0
			assert.Equal(t, want, (tpdu.EnhancedVoiceMail{Profile: 1, MailboxExtensionIndicator: indicator, MailboxExtension: ext}).Valid())
			assert.Equal(t, want, (tpdu.EnhancedVoiceMail{Profile: 1, Notifications: []tpdu.VoiceMailNotification{{ExtensionIndicator: indicator, Extension: ext}}}).Valid())
			assert.Equal(t, want, (tpdu.EnhancedVoiceMail{Profile: 1, DeleteConfirmation: true, Deletes: []tpdu.VoiceMailDelete{{ExtensionIndicator: indicator, Extension: ext}}}).Valid())
		}
	}
	assert.False(t, (tpdu.EnhancedVoiceMail{Profile: 1, Deletes: []tpdu.VoiceMailDelete{{}}}).Valid())
	assert.False(t, (tpdu.EnhancedVoiceMail{Profile: 1, DeleteConfirmation: true, Notifications: []tpdu.VoiceMailNotification{{}}}).Valid())
}

func TestUserDataHeaderEnhancedVoiceMail(t *testing.T) {
	for _, h := range []tpdu.UserDataHeader{nil, {}, {{ID: 0x23}}, {{ID: 0xff}}} {
		got, ok := h.EnhancedVoiceMail()
		assert.Equal(t, tpdu.EnhancedVoiceMail{}, got)
		assert.False(t, ok)
	}
	for _, p := range voiceMailVectors() {
		h := tpdu.UserDataHeader{{ID: 0x23, Data: []byte{0, 0, 0x81, 42, 0}}, {ID: 0x23, Data: p.data}, {ID: 0xff}, {ID: 0x23}, {ID: 0x23, Data: []byte{0, 0, 0x81, 0, 0, 0}}}
		got, ok := h.EnhancedVoiceMail()
		assert.True(t, ok)
		assert.Equal(t, p.want, got)
	}
	got, ok := (tpdu.UserDataHeader{{ID: 0x23, Data: []byte{0, 0, 0x81, 0, 0}}, {ID: 0x23, Data: []byte{2, 0, 0x81, 0, 0}}}).EnhancedVoiceMail()
	assert.True(t, ok)
	assert.Equal(t, 2, got.Reserved)
	assert.False(t, got.Valid())
}

func ExampleUserDataHeader_EnhancedVoiceMail() {
	header := tpdu.UserDataHeader{{ID: tpdu.IEIEnhancedVoiceMail, Data: []byte{
		0x10, 3, 0x91, 0x21, 0xf3, 2, 2,
		0, 1, 60, 0x47, 2, 0x81, 0x54,
		0, 2, 30, 7, 0, 0x81,
	}}}
	mail, ok := header.EnhancedVoiceMail()
	fmt.Printf("mailbox %s: %d unread, found %t\n", mail.AccessAddress.Number(), mail.VoiceMessages, ok)
	for _, message := range mail.Notifications {
		fmt.Printf("message %d: %d seconds, urgent %t\n", message.ID, message.Length, message.Urgent)
	}
	// Output:
	// mailbox +123: 2 unread, found true
	// message 1: 60 seconds, urgent true
	// message 2: 30 seconds, urgent false
}
