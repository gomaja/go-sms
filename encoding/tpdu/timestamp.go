// SPDX-License-Identifier: MIT

package tpdu

import (
	"time"

	"github.com/gomaja/go-sms/encoding/bcd"
)

// Timestamp represents a SCTS timestamp, as defined in 3GPP TS 23.040 Section
// 9.2.3.11.
//
// The same representation is used for the TP-DT and for the TP-VP in absolute
// format.
//
// The SCTS carries only a two digit year, which is taken to be from 1970 to
// 2069, a time zone in quarters of an hour, and a resolution of one second.
//
// The zero Timestamp corresponds to the all-zero SCTS, which is not a valid
// time and is commonly used where no time is available.
type Timestamp struct {
	time.Time

	// raw holds the octets as received, if hasRaw is set, when they differ
	// from the encoding of the Time decoded from them.
	raw    [7]byte
	hasRaw bool
}

func (t Timestamp) String() string {
	return t.Format("2006-01-02 15:04:05 -0700")
}

// MarshalBinary encodes the SCTS timestamp into binary.
//
// A Timestamp that was unmarshalled is encoded exactly as received, as long
// as its Time has not been changed, as required by 3GPP TS 23.040 Section
// 9.2.3.11.
//
// A zero Time is encoded as all zeros. Otherwise an error is returned if the
// year is outside 1970 to 2069, or if the time zone offset is not a whole
// number of quarter hours from -79 to 79, as these cannot be represented.
// Fractions of a second are dropped.
func (t *Timestamp) MarshalBinary() ([]byte, error) {
	if t.hasRaw && sameTime(t.Time, decodeTimestamp(t.raw)) {
		return append([]byte(nil), t.raw[:]...), nil
	}
	dst, err := encodeTimestamp(t.Time)
	if err != nil {
		return nil, err
	}
	return dst[:], nil
}

// UnmarshalBinary decodes the SCTS timestamp from binary.
//
// Only an src shorter than the 7 octets of the SCTS results in an error.
//
// As required by 3GPP TS 23.040 Section 9.2.3.11, "If the MS receives a
// non-integer value in the SCTS, it shall assume that the digit is set to 0
// but shall store the entire field exactly as received." If the octets do not
// form a valid date and time, such as the all-zero SCTS, then the Time is left
// zero.  In either case the received octets are retained, and are reproduced
// by MarshalBinary.
func (t *Timestamp) UnmarshalBinary(src []byte) error {
	if len(src) < 7 {
		return ErrUnderflow
	}
	var raw [7]byte
	copy(raw[:], src)
	*t = Timestamp{Time: decodeTimestamp(raw)}
	if enc, err := encodeTimestamp(t.Time); err != nil || enc != raw {
		t.raw = raw
		t.hasRaw = true
	}
	return nil
}

// semiOctetDigits returns the two digits of a field in semi-octet
// representation, with the first digit in the lower nibble, and any
// non-integer digit taken as 0, as per 3GPP TS 23.040 Section 9.2.3.11.
func semiOctetDigits(b byte) int {
	tens := int(b & 0x0f)
	units := int(b >> 4)
	if tens > 9 {
		tens = 0
	}
	if units > 9 {
		units = 0
	}
	return tens*10 + units
}

// decodeTimestamp returns the time represented by the SCTS octets, or the zero
// time if they do not form a valid date and time.
func decodeTimestamp(src [7]byte) time.Time {
	var f [6]int
	for i := range f {
		f[i] = semiOctetDigits(src[i])
	}
	year := f[0] + 1900
	if f[0] < 70 {
		year += 100
	}
	mon := time.Month(f[1])
	day, hour, min, sec := f[2], f[3], f[4], f[5]
	if mon < time.January || mon > time.December ||
		day < 1 || day > daysIn(mon, year) ||
		hour > 23 || min > 59 || sec > 59 {
		return time.Time{}
	}
	// The first bit of the time zone, bit 3 of the octet, is the sign.
	tz := semiOctetDigits(src[6] &^ 0x08)
	if src[6]&0x08 != 0 {
		tz = -tz
	}
	loc := time.UTC
	if tz != 0 {
		tzoffset := tz * 15 * 60 // seconds east of UTC
		loc = time.FixedZone("SCTS", tzoffset)
	}
	return time.Date(year, mon, day, hour, min, sec, 0, loc)
}

// encodeTimestamp returns the SCTS octets representing the time.
func encodeTimestamp(t time.Time) (dst [7]byte, err error) {
	if t.IsZero() {
		return dst, nil
	}
	year := t.Year()
	if year < 1970 || year > 2069 {
		return dst, EncodeError("year", ErrInvalid)
	}
	_, tz := t.Zone()
	if tz%(15*60) != 0 {
		return dst, EncodeError("tz", ErrInvalid)
	}
	f := []int{year % 100, int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second()}
	for i, v := range f {
		// these are in range for the time methods, so never trip
		dst[i], err = bcd.Encode(v)
		if err != nil {
			return dst, err
		}
	}
	dst[6], err = bcd.EncodeSigned(tz / (15 * 60))
	return dst, err
}

// daysIn returns the number of days in the month of the year.
func daysIn(m time.Month, year int) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// sameTime reports whether a and b are the same instant in the same time
// zone offset, and so have the same SCTS encoding.
func sameTime(a, b time.Time) bool {
	if !a.Equal(b) {
		return false
	}
	_, ao := a.Zone()
	_, bo := b.Zone()
	return ao == bo
}
