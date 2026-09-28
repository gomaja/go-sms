// SPDX-License-Identifier: MIT

package tpdu_test

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/gomaja/go-sms/encoding/gsm7"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type decodeTestPattern struct {
	Field  string
	Offset int
	Err    error
}

// TestDecodeError tests that the errors can be stringified.
// It is fragile, as it compares the strings exactly, but its main purpose is
// to confirm the Error function doesn't recurse, as that is bad.
func TestDecodeError(t *testing.T) {
	patterns := []decodeTestPattern{
		{"nil", 0, nil},
		{"err", 2, errors.New("an error")},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			err := tpdu.NewDecodeError(p.Field, p.Offset, p.Err)
			expected := fmt.Sprintf("tpdu: error decoding %s at octet %d: %v", p.Field, p.Offset, p.Err)
			s := err.Error()
			if s != expected {
				t.Errorf("failed to stringify, expected '%s', got '%s'", expected, s)
			}
		}
		t.Run(p.Field, f)
	}
	// nested
	f := func(t *testing.T) {
		err := tpdu.NewDecodeError("nested", 40, tpdu.NewDecodeError("inner", 2, nil))
		expected := fmt.Sprintf("tpdu: error decoding nested.inner at octet 42: %v", nil)
		s := err.Error()
		if s != expected {
			t.Errorf("failed to stringify, expected '%s', got '%s'", expected, s)
		}
	}
	t.Run("nested", f)
}

// TestEncodeError tests that the errors can be stringified.
// It is fragile, as it compares the strings exactly, but its main purpose is
// to confirm the Error function doesn't recurse, as that is bad.
func TestEncodeError(t *testing.T) {
	patterns := []decodeTestPattern{
		{"nil", 0, nil},
		{"err", 2, errors.New("an error")},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			err := tpdu.NewEncodeError(p.Field, p.Err)
			expected := fmt.Sprintf("tpdu: error encoding %s: %v", p.Field, p.Err)
			s := err.Error()
			if s != expected {
				t.Errorf("failed to stringify, expected '%s', got '%s'", expected, s)
			}
		}
		t.Run(p.Field, f)
	}
	// nested
	f := func(t *testing.T) {
		err := tpdu.NewEncodeError("nested", tpdu.NewEncodeError("inner", nil))
		expected := fmt.Sprintf("tpdu: error encoding nested.inner: %v", nil)
		s := err.Error()
		if s != expected {
			t.Errorf("failed to stringify, expected '%s', got '%s'", expected, s)
		}
	}
	t.Run("nested", f)
}

// TestErrorsIsThroughTPDUErrors checks that the sentinel errors can be matched
// with errors.Is through the errors returned by UnmarshalBinary and
// MarshalBinary.
func TestErrorsIsThroughTPDUErrors(t *testing.T) {
	d := tpdu.TPDU{}
	err := d.UnmarshalBinary([]byte{0x04})
	if !errors.Is(err, tpdu.ErrUnderflow) {
		t.Errorf("decode: errors.Is(%v, ErrUnderflow) is false", err)
	}
	e := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, DCS: 0x08, UD: []byte{0x00}}
	_, err = e.MarshalBinary()
	if !errors.Is(err, tpdu.ErrOddUCS2Length) {
		t.Errorf("encode: errors.Is(%v, ErrOddUCS2Length) is false", err)
	}
}

// TestErrorsAsThroughTPDUErrors checks that the error types can be extracted
// with errors.As from the errors returned by UnmarshalBinary and
// MarshalBinary.
func TestErrorsAsThroughTPDUErrors(t *testing.T) {
	d := tpdu.TPDU{Direction: tpdu.MO}
	err := d.UnmarshalBinary([]byte{0x03})
	var de tpdu.DecodeError
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "tpdu.firstOctet", de.Field)
	var ust tpdu.ErrUnsupportedSmsType
	require.True(t, errors.As(err, &ust))
	assert.Equal(t, tpdu.ErrUnsupportedSmsType(7), ust)

	// a septet with its 8th bit set cannot be packed
	e := tpdu.TPDU{Direction: tpdu.MO, FirstOctet: 0x01, UD: []byte("caf\xc3\xa9")}
	_, err = e.MarshalBinary()
	var ee tpdu.EncodeError
	require.True(t, errors.As(err, &ee))
	assert.Equal(t, "SmsSubmit.ud.sm", ee.Field)
	var ise gsm7.ErrInvalidSeptet
	require.True(t, errors.As(err, &ise))
	assert.Equal(t, gsm7.ErrInvalidSeptet{Offset: 3, Septet: 0xc3}, ise)
}

func TestErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	assert.Equal(t, inner, tpdu.NewDecodeError("f", 1, inner).Unwrap())
	assert.Equal(t, inner, tpdu.NewEncodeError("f", inner).Unwrap())
	// nested errors are flattened, so unwrap to the root cause
	assert.Equal(t, inner,
		tpdu.NewDecodeError("outer", 1, tpdu.NewDecodeError("f", 1, inner)).Unwrap())
	assert.Equal(t, inner,
		tpdu.NewEncodeError("outer", tpdu.NewEncodeError("f", inner)).Unwrap())
}

func TestNewDecodeErrorMapsEOF(t *testing.T) {
	for _, e := range []error{
		io.EOF,
		io.ErrUnexpectedEOF,
		fmt.Errorf("wrapped: %w", io.EOF),
	} {
		err := tpdu.NewDecodeError("f", 2, e)
		assert.Equal(t, tpdu.DecodeError{Field: "f", Offset: 2, Err: tpdu.ErrUnderflow}, err)
	}
}

// TestErrUnsupportedMTI tests that the errors can be stringified.
// It is fragile, as it compares the strings exactly, but its main purpose is
// to confirm the Error function doesn't recurse, as that is bad.
func TestErrUnsupportedSmsType(t *testing.T) {
	patterns := []byte{0x00, 0xa0, 0x0a, 0x9a, 0xa9, 0xff}
	for _, p := range patterns {
		f := func(t *testing.T) {
			err := tpdu.ErrUnsupportedSmsType(p)
			expected := fmt.Sprintf("unsupported SMS type: 0x%x", uint(err))
			s := err.Error()
			if s != expected {
				t.Errorf("failed to stringify %02x, expected '%s', got '%s'", p, expected, s)
			}
		}
		t.Run(fmt.Sprintf("%x", p), f)
	}
}
