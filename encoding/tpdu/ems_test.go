// SPDX-License-Identifier: MIT

package tpdu_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEMSIdentifiers(t *testing.T) {
	ids := []byte{
		tpdu.IEITextFormatting,
		tpdu.IEIPredefinedSound,
		tpdu.IEIUserDefinedSound,
		tpdu.IEIPredefinedAnimation,
		tpdu.IEILargeAnimation,
		tpdu.IEISmallAnimation,
		tpdu.IEILargePicture,
		tpdu.IEISmallPicture,
		tpdu.IEIVariablePicture,
		tpdu.IEIUserPromptIndicator,
		tpdu.IEIExtendedObject,
		tpdu.IEIReusedExtendedObject,
		tpdu.IEICompressionControl,
		tpdu.IEIObjectDistributionIndicator,
		tpdu.IEIStandardWVGObject,
		tpdu.IEICharacterSizeWVGObject,
		tpdu.IEIExtendedObjectDataRequestCommand,
	}
	for i, id := range ids {
		assert.Equal(t, byte(0x0a+i), id)
	}
	assert.Equal(t, []tpdu.TextAlignment{0, 1, 2, 3}, []tpdu.TextAlignment{tpdu.TextAlignLeft, tpdu.TextAlignCenter, tpdu.TextAlignRight, tpdu.TextAlignLanguage})
	assert.Equal(t, []tpdu.TextFontSize{0, 1, 2}, []tpdu.TextFontSize{tpdu.TextFontNormal, tpdu.TextFontLarge, tpdu.TextFontSmall})
	types := []tpdu.ExtendedObjectType{
		tpdu.ExtendedObjectPredefinedSound,
		tpdu.ExtendedObjectIMelody,
		tpdu.ExtendedObjectBlackWhiteBitmap,
		tpdu.ExtendedObjectGreyscaleBitmap,
		tpdu.ExtendedObjectColourBitmap,
		tpdu.ExtendedObjectPredefinedAnimation,
		tpdu.ExtendedObjectBlackWhiteAnimation,
		tpdu.ExtendedObjectGreyscaleAnimation,
		tpdu.ExtendedObjectColourAnimation,
		tpdu.ExtendedObjectVCard,
		tpdu.ExtendedObjectVCalendar,
		tpdu.ExtendedObjectStandardWVG,
		tpdu.ExtendedObjectPolyphonicMelody,
	}
	for i, typ := range types {
		assert.Equal(t, tpdu.ExtendedObjectType(i), typ)
	}
	assert.Equal(t, tpdu.ExtendedObjectType(255), tpdu.ExtendedObjectDataFormatDeliveryRequest)
	assert.Equal(t, 0, tpdu.CompressionLZSS)
}

func TestEMSFixedContracts(t *testing.T) {
	checkIEContract(t, tpdu.InformationElement.PredefinedSound, map[byte]int{0x0b: 2})
	checkIEContract(t, tpdu.InformationElement.PredefinedAnimation, map[byte]int{0x0d: 2})
	checkIEContract(t, tpdu.InformationElement.UserPromptIndicator, map[byte]int{0x13: 1})
	checkIEContract(t, tpdu.InformationElement.ReusedExtendedObject, map[byte]int{0x15: 3})
	checkIEContract(t, tpdu.InformationElement.ObjectDistributionIndicator, map[byte]int{0x17: 2})
	// A comparable projection lets the existing helper also check copied data.
	type object struct {
		ID       byte
		Position int
		Data     string
	}
	for _, id := range []byte{0x0e, 0x0f, 0x10, 0x11} {
		length := 33
		if id == 0x0e || id == 0x10 {
			length = 129
		}
		checkIEContract(t, func(ie tpdu.InformationElement) (object, error) {
			// EMSObject accepts seven IEIs; map the other supported ones to an
			// unsupported ID so this fixed-length contract isolates one layout.
			if ie.ID != id {
				ie.ID = 0xff
			}
			v, err := ie.EMSObject()
			return object{v.ID, v.Position, string(v.Data)}, err
		}, map[byte]int{id: length})
	}
}

// checkEMSContract extends checkIEContract to optional fields, variable
// lengths and values containing slices, with parameter-specific offsets.
func checkEMSContract[T any](t *testing.T, decode func(tpdu.InformationElement) (T, error), layout func(byte, int) (int, error)) {
	t.Helper()
	var zero T
	for id := 0; id < 256; id++ {
		for n := 0; n < 256; n++ {
			data := make([]byte, n)
			for i := range data {
				data[i] = 255
			}
			if n == 0 {
				data = nil
			}
			got, err := decode(tpdu.InformationElement{ID: byte(id), Data: data})
			offset, want := layout(byte(id), n)
			if want == nil {
				require.NoError(t, err)
				continue
			}
			field := "ied"
			if want == tpdu.ErrInvalid {
				field = "iei"
			}
			var de tpdu.DecodeError
			if !errors.Is(err, want) || !errors.As(err, &de) || de.Field != field || de.Offset != offset || !reflect.DeepEqual(got, zero) {
				t.Fatalf("id=%02x length=%d: got %+v, %v; want zero, %s at %d: %v", id, n, got, err, field, offset, want)
			}
		}
	}
}

func TestEMSVariableContracts(t *testing.T) {
	checkEMSContract(t, tpdu.InformationElement.TextFormatting, func(id byte, n int) (int, error) {
		if id != 0x0a {
			return 0, tpdu.ErrInvalid
		}
		if n < 3 {
			return 2, tpdu.ErrUnderflow
		}
		if n > 4 {
			return 2, tpdu.ErrOverlength
		}
		return 0, nil
	})
	checkEMSContract(t, tpdu.InformationElement.EMSObject, func(id byte, n int) (int, error) {
		length := 0
		switch id {
		case 0x0c, 0x18, 0x19:
			if n == 0 {
				return 2, tpdu.ErrUnderflow
			}
			return 0, nil
		case 0x0e, 0x10:
			length = 129
		case 0x0f, 0x11:
			length = 33
		default:
			return 0, tpdu.ErrInvalid
		}
		if n < length {
			return 2, tpdu.ErrUnderflow
		}
		if n > length {
			return 2, tpdu.ErrOverlength
		}
		return 0, nil
	})
	checkEMSContract(t, tpdu.InformationElement.VariablePicture, func(id byte, n int) (int, error) {
		if id != 0x12 {
			return 0, tpdu.ErrInvalid
		}
		if n < 3 {
			return 2 + n, tpdu.ErrUnderflow
		}
		return 5, tpdu.ErrUnderflow // 255*255 data octets
	})
	checkEMSContract(t, tpdu.InformationElement.ExtendedObject, func(id byte, n int) (int, error) {
		if id != 0x14 {
			return 0, tpdu.ErrInvalid
		}
		if n < 7 {
			return []int{2, 3, 3, 5, 6, 7, 7}[n], tpdu.ErrUnderflow
		}
		return 0, nil
	})
	checkEMSContract(t, tpdu.InformationElement.CompressionControl, func(id byte, n int) (int, error) {
		if id != 0x16 {
			return 0, tpdu.ErrInvalid
		}
		if n < 3 {
			return []int{2, 3, 3}[n], tpdu.ErrUnderflow
		}
		return 0, nil
	})
}

func TestTextFormattingOracle(t *testing.T) {
	// Section 9.2.3.24.10.1.1: alignment, size, four independent styles,
	// then the optional foreground/background semi-octets.
	for mode := 0; mode < 256; mode++ {
		for colour := -1; colour < 256; colour++ {
			data := []byte{15, 18, byte(mode)}
			want := tpdu.TextFormatting{
				Start: 15, Length: 18,
				Alignment: tpdu.TextAlignment(mode % 4), FontSize: tpdu.TextFontSize(mode / 4 % 4),
				Bold: mode/16%2 == 1, Italic: mode/32%2 == 1,
				Underlined: mode/64%2 == 1, Strikethrough: mode/128 == 1,
				HasColour: colour >= 0,
			}
			if colour >= 0 {
				data = append(data, byte(colour))
				want.Foreground = colour % 16
				want.Background = colour / 16
			}
			got, err := (tpdu.InformationElement{ID: 0x0a, Data: data}).TextFormatting()
			if err != nil || got != want || got.Valid() != (mode/4%4 != 3) {
				t.Fatalf("mode=%02x colour=%d: %+v, %v; want %+v", mode, colour, got, err, want)
			}
		}
	}
}

func TestPredefinedObjectsOracle(t *testing.T) {
	for _, id := range []byte{0x0b, 0x0d} {
		for pos := 0; pos < 256; pos++ {
			for number := 0; number < 256; number++ {
				ie := tpdu.InformationElement{ID: id, Data: []byte{byte(pos), byte(number)}}
				decode, limit := tpdu.InformationElement.PredefinedSound, 10
				if id == 0x0d {
					decode, limit = tpdu.InformationElement.PredefinedAnimation, 15
				}
				got, err := decode(ie)
				want := tpdu.PredefinedObject{ID: id, Position: pos, Number: number}
				if err != nil || got != want || got.Valid() != (number < limit) {
					t.Fatalf("%x: %+v, %v; want %+v", ie.Data, got, err, want)
				}
			}
		}
	}
}

func TestEMSObjectData(t *testing.T) {
	for _, id := range []byte{0x0c, 0x0e, 0x0f, 0x10, 0x11, 0x18, 0x19} {
		lengths := []int{0, 1, 127, 128, 129, 254}
		if id == 0x0e || id == 0x10 {
			lengths = []int{128}
		}
		if id == 0x0f || id == 0x11 {
			lengths = []int{32}
		}
		for _, n := range lengths {
			data := make([]byte, n+1)
			data[0] = 37
			for i := 1; i < len(data); i++ {
				data[i] = byte(i)
			}
			got, err := (tpdu.InformationElement{ID: id, Data: data}).EMSObject()
			require.NoError(t, err)
			assert.Equal(t, id, got.ID)
			assert.Equal(t, 37, got.Position)
			assert.Equal(t, data[1:], got.Data)
			assert.Equal(t, id != 0x0c || n <= 128, got.Valid())
			saved := append([]byte{}, got.Data...)
			for i := range data {
				data[i] = 0xa5
			}
			assert.Equal(t, saved, got.Data)
		}
	}
}

func TestVariablePictureBoundaries(t *testing.T) {
	// Each row uses horizontal-units*8 bits, hence horizontal-units octets.
	for _, d := range [][2]int{{0, 0}, {0, 255}, {255, 0}, {1, 1}, {2, 16}, {3, 7}, {31, 8}, {255, 255}} {
		size := d[0] * d[1]
		for _, n := range []int{size - 1, size, size + 1} {
			if n < 0 {
				continue
			}
			data := make([]byte, 3+n)
			data[0] = 9
			data[1] = byte(d[0])
			data[2] = byte(d[1])
			for i := 3; i < len(data); i++ {
				data[i] = byte(i)
			}
			got, err := (tpdu.InformationElement{ID: 0x12, Data: data}).VariablePicture()
			if n != size {
				want := tpdu.ErrUnderflow
				if n > size {
					want = tpdu.ErrOverlength
				}
				assert.Equal(t, tpdu.NewDecodeError("ied", 5, want), err)
				assert.Equal(t, tpdu.VariablePicture{}, got)
				continue
			}
			require.NoError(t, err)
			assert.Equal(t, tpdu.VariablePicture{Position: 9, Width: d[0] * 8, Height: d[1], Data: data[3:]}, got)
			saved := append([]byte{}, got.Data...)
			for i := range data {
				data[i] = 0
			}
			assert.Equal(t, saved, got.Data)
		}
	}
}

func TestExtendedObjectOracle(t *testing.T) {
	for control := 0; control < 256; control++ {
		for typ := 0; typ < 256; typ++ {
			got, err := (tpdu.InformationElement{ID: 0x14, Data: []byte{42, 0x12, 0x34, byte(control), byte(typ), 0x01, 0x23, 0xa5}}).ExtendedObject()
			want := tpdu.ExtendedObject{Reference: 42, Length: 4660, DoNotForward: control%2 == 1, UserPrompt: control/2%2 == 1, Reserved: control / 4 * 4, Type: tpdu.ExtendedObjectType(typ), Position: 291, Data: []byte{0xa5}}
			valid := control < 4 && (typ <= 12 || typ == 255)
			if err != nil || !reflect.DeepEqual(got, want) || got.Valid() != valid {
				t.Fatalf("control=%02x type=%02x: %+v, %v; want %+v valid=%t", control, typ, got, err, want, valid)
			}
		}
	}
}

func TestCompressionControlOracle(t *testing.T) {
	for info := 0; info < 256; info++ {
		got, err := (tpdu.InformationElement{ID: 0x16, Data: []byte{byte(info), 0x12, 0x34, 0x81, 0xa5}}).CompressionControl()
		require.NoError(t, err)
		assert.Equal(t, tpdu.CompressionControl{Algorithm: info % 16, Parameters: info / 16, Length: 4660, Data: []byte{0x81, 0xa5}}, got)
		assert.Equal(t, info == 0, got.Valid())
	}
}

func TestDistributionOracle(t *testing.T) {
	for count := 0; count < 256; count++ {
		for attr := 0; attr < 256; attr++ {
			got, err := (tpdu.InformationElement{ID: 0x17, Data: []byte{byte(count), byte(attr)}}).ObjectDistributionIndicator()
			want := tpdu.ObjectDistributionIndicator{Count: count, DoNotForward: attr%2 == 1, Reserved: attr / 2 * 2}
			if err != nil || got != want || got.Valid() != (attr < 2) {
				t.Fatalf("%d %02x: %+v, %v; want %+v", count, attr, got, err, want)
			}
		}
	}
}

func TestExtendedObjectAndCompressionFragments(t *testing.T) {
	for _, length := range []int{0, 1, 2, 256, 65535} {
		for _, n := range []int{0, 1, 2, 3} {
			payload := make([]byte, n)
			for i := range payload {
				payload[i] = byte(i + 1)
			}
			eoData := append([]byte{8, byte(length / 256), byte(length), 0, 2, 0, 1}, payload...)
			eo, err := (tpdu.InformationElement{ID: 0x14, Data: eoData}).ExtendedObject()
			require.NoError(t, err)
			assert.Equal(t, length, eo.Length)
			assert.Equal(t, payload, eo.Data)
			assert.Equal(t, n <= length, eo.Valid())
			ccData := append([]byte{0, byte(length / 256), byte(length)}, payload...)
			cc, err := (tpdu.InformationElement{ID: 0x16, Data: ccData}).CompressionControl()
			require.NoError(t, err)
			assert.Equal(t, length, cc.Length)
			assert.Equal(t, payload, cc.Data)
			assert.Equal(t, n <= length, cc.Valid())
			for i := range eoData {
				eoData[i] = 0xff
			}
			for i := range ccData {
				ccData[i] = 0xff
			}
			assert.Equal(t, payload, eo.Data)
			assert.Equal(t, payload, cc.Data)
		}
	}
	for pos := 0; pos <= 65535; pos++ {
		data := []byte{255, 0, 0, 0, 0, byte(pos / 256), byte(pos)}
		got, err := (tpdu.InformationElement{ID: 0x14, Data: data}).ExtendedObject()
		require.NoError(t, err)
		assert.Equal(t, pos, got.Position)
		assert.Equal(t, pos != 0, got.Valid())
	}
}

func TestPromptAndReusedExtendedObjectOracle(t *testing.T) {
	for n := 0; n < 256; n++ {
		got, err := (tpdu.InformationElement{ID: 0x13, Data: []byte{byte(n)}}).UserPromptIndicator()
		require.NoError(t, err)
		assert.Equal(t, n, got)
	}
	for pos := 0; pos <= 65535; pos++ {
		got, err := (tpdu.InformationElement{ID: 0x15, Data: []byte{42, byte(pos / 256), byte(pos)}}).ReusedExtendedObject()
		require.NoError(t, err)
		assert.Equal(t, tpdu.ReusedExtendedObject{Reference: 42, Position: pos}, got)
	}
}

func TestEMSSpecExamples(t *testing.T) {
	// Section 9.2.3.24.10.2.1 gives complete formatting and sound headers.
	var header tpdu.UserDataHeader
	_, err := header.UnmarshalBinary([]byte{5, 0x0a, 3, 0x0f, 0x12, 0x10})
	require.NoError(t, err)
	require.Len(t, header, 1)
	text, err := header[0].TextFormatting()
	require.NoError(t, err)
	assert.Equal(t, tpdu.TextFormatting{Start: 15, Length: 18, Bold: true}, text)
	_, err = header.UnmarshalBinary([]byte{8, 0x0b, 2, 9, 5, 0x0b, 2, 0x1c, 7})
	require.NoError(t, err)
	require.Len(t, header, 2)
	for i, want := range []tpdu.PredefinedObject{{ID: 0x0b, Position: 9, Number: 5}, {ID: 0x0b, Position: 28, Number: 7}} {
		got, err := header[i].PredefinedSound()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	// Sections .10.2.2 and .10.2.3 leave object bytes unspecified.
	// Synthetic bytes fill those placeholders; positions and sizes are given.
	for _, p := range []struct {
		id             byte
		position, size int
	}{
		{0x11, 8, 32}, {0x10, 0, 128}, {0x0c, 0, 128}, {0x0f, 0x24, 32}, {0x0f, 0x2f, 32}, {0x0f, 7, 32}, {0x0f, 0x25, 32}, {0x0c, 0, 128}, {0x10, 0, 128},
	} {
		data := make([]byte, 1+p.size)
		data[0] = byte(p.position)
		for i := 1; i < len(data); i++ {
			data[i] = byte(i)
		}
		got, err := (tpdu.InformationElement{ID: p.id, Data: data}).EMSObject()
		require.NoError(t, err)
		assert.Equal(t, tpdu.EMSObject{ID: p.id, Position: p.position, Data: data[1:]}, got)
		assert.True(t, got.Valid())
	}
}

func ExampleInformationElement_PredefinedSound() {
	// 3GPP TS 23.040 Section 9.2.3.24.10.2.1, in header order.
	var header tpdu.UserDataHeader
	if _, err := header.UnmarshalBinary([]byte{8, 0x0b, 2, 9, 5, 0x0b, 2, 0x1c, 7}); err != nil {
		fmt.Println(err)
		return
	}
	for _, ie := range header {
		sound, err := ie.PredefinedSound()
		if err == nil && sound.Valid() {
			fmt.Printf("Sound %d after character %d\n", sound.Number, sound.Position)
		}
	}
	// Output:
	// Sound 5 after character 9
	// Sound 7 after character 28
}

func TestEMSValid(t *testing.T) {
	for _, n := range []int{-1, 0, 1, 2, 3, 15, 16, 255, 256, 65535, 65536} {
		assert.Equal(t, n >= 0 && n <= 2, (tpdu.TextFormatting{FontSize: tpdu.TextFontSize(n)}).Valid())
		assert.Equal(t, n >= 0 && n <= 3, (tpdu.TextFormatting{Alignment: tpdu.TextAlignment(n)}).Valid())
		assert.Equal(t, n >= 0 && n <= 255, (tpdu.TextFormatting{Start: n}).Valid())
		assert.Equal(t, n >= 0 && n <= 255, (tpdu.TextFormatting{Length: n}).Valid())
		assert.Equal(t, n >= 0 && n <= 15, (tpdu.TextFormatting{HasColour: true, Foreground: n}).Valid())
		assert.Equal(t, n >= 0 && n <= 15, (tpdu.TextFormatting{HasColour: true, Background: n}).Valid())
		assert.Equal(t, n >= 0 && n < 10, (tpdu.PredefinedObject{ID: 0x0b, Number: n}).Valid())
		assert.Equal(t, n >= 0 && n < 15, (tpdu.PredefinedObject{ID: 0x0d, Number: n}).Valid())
		assert.Equal(t, n >= 0 && n <= 255, (tpdu.PredefinedObject{ID: 0x0b, Position: n}).Valid())
		assert.Equal(t, n >= 0 && n <= 255, (tpdu.EMSObject{ID: 0x0c, Position: n}).Valid())
		assert.Equal(t, n >= 0 && n <= 255, (tpdu.ExtendedObject{Reference: n, Position: 1}).Valid())
		assert.Equal(t, n >= 0 && n <= 65535, (tpdu.ExtendedObject{Length: n, Position: 1}).Valid())
		assert.Equal(t, n >= 1 && n <= 65535, (tpdu.ExtendedObject{Position: n}).Valid())
		assert.Equal(t, n >= 0 && n <= 65535, (tpdu.CompressionControl{Length: n}).Valid())
		assert.Equal(t, n == 0, (tpdu.CompressionControl{Algorithm: n}).Valid())
		assert.Equal(t, n == 0, (tpdu.CompressionControl{Parameters: n}).Valid())
		assert.Equal(t, n == 0, (tpdu.ExtendedObject{Position: 1, Reserved: n}).Valid())
		assert.Equal(t, n == 0, (tpdu.ObjectDistributionIndicator{Reserved: n}).Valid())
		assert.Equal(t, n >= 0 && n <= 255, (tpdu.ObjectDistributionIndicator{Count: n}).Valid())
	}
	for _, typ := range []tpdu.ExtendedObjectType{-1, 0, 12, 13, 254, 255, 256} {
		assert.Equal(t, typ >= 0 && typ <= 12 || typ == 255, (tpdu.ExtendedObject{Position: 1, Type: typ}).Valid())
	}
	assert.False(t, (tpdu.PredefinedObject{ID: 0xff}).Valid())
	assert.False(t, (tpdu.EMSObject{ID: 0xff}).Valid())
	for _, id := range []byte{0x0e, 0x0f, 0x10, 0x11} {
		size := 32
		if id == 0x0e || id == 0x10 {
			size = 128
		}
		for _, n := range []int{size - 1, size, size + 1} {
			assert.Equal(t, n == size, (tpdu.EMSObject{ID: id, Data: make([]byte, n)}).Valid())
		}
	}
}
