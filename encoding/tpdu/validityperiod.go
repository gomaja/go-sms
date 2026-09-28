// SPDX-License-Identifier: MIT

package tpdu

import (
	"time"

	"github.com/gomaja/go-sms/encoding/bcd"
)

// ValidityPeriod represents the validity period as defined in 3GPP TS 23.040
// Section 9.2.3.12.
//
// When marshalled, a relative Duration is rounded down to the resolution of
// its format, and must lie within the range of the format:
//
//	VpfRelative, EvpfRelative: 5 minutes to 63 weeks
//	EvpfRelativeSeconds:        1 to 255 seconds (0 is reserved)
//	EvpfRelativeHHMMSS:         0 to 99:59:59
type ValidityPeriod struct {
	Format   ValidityPeriodFormat
	Time     Timestamp     // for VpfAbsolute
	Duration time.Duration // for VpfRelative and VpfEnhanced
	EFI      byte          // enhanced functionality indicator - first octet of enhanced format
}

// EnhancedFormat extracts the format field from the EFI.
func EnhancedFormat(efi byte) EnhancedValidityPeriodFormat {
	return EnhancedValidityPeriodFormat(efi & 0x07)
}

// SetAbsolute sets the validity period to an absolute time.
func (v *ValidityPeriod) SetAbsolute(t Timestamp) {
	v.Format = VpfAbsolute
	v.Duration = 0
	v.Time = t
	v.EFI = 0
}

// SetRelative sets the validity period to a relative time.
//
// The duration must be from 5 minutes to 63 weeks to be marshalled.
func (v *ValidityPeriod) SetRelative(d time.Duration) {
	v.Format = VpfRelative
	v.Duration = d
	v.Time = Timestamp{}
	v.EFI = 0
}

// SetEnhanced sets the validity period to an enhanced format as determined
// from the functionality identifier (efi).
//
// The range of durations that can be marshalled depends on the format, as
// described for ValidityPeriod.
func (v *ValidityPeriod) SetEnhanced(d time.Duration, efi byte) {
	v.Format = VpfEnhanced
	v.Duration = d
	v.Time = Timestamp{}
	v.EFI = efi
}

// MarshalBinary marshals a ValidityPeriod.
func (v *ValidityPeriod) MarshalBinary() ([]byte, error) {
	switch v.Format {
	case VpfAbsolute:
		return v.Time.MarshalBinary()
	case VpfEnhanced:
		evpf := EnhancedFormat(v.EFI)
		if evpf > EvpfRelativeHHMMSS {
			return nil, EncodeError("fi", ErrInvalid)
		}
		dst := make([]byte, 7)
		dst[0] = v.EFI
		switch evpf {
		case EvpfRelative:
			t, err := durationToRelative(v.Duration)
			if err != nil {
				return nil, err
			}
			dst[1] = t
		case EvpfRelativeSeconds:
			if v.Duration < minSecondsVP || v.Duration > maxSecondsVP {
				return nil, EncodeError("duration", ErrInvalid)
			}
			dst[1] = byte(v.Duration / time.Second)
		case EvpfRelativeHHMMSS:
			if v.Duration < 0 || v.Duration > maxHHMMSSVP {
				return nil, EncodeError("duration", ErrInvalid)
			}
			d := v.Duration
			f := []int{int(d / time.Hour), int(d / time.Minute % 60), int(d / time.Second % 60)}
			for i, tf := range f {
				t, err := bcd.Encode(tf)
				// this should never trip, as the encoded values should always be valid, but just in case...
				if err != nil {
					return nil, EncodeError("enhanced", err)
				}
				dst[i+1] = t
			}
		}
		return dst, nil
	case VpfRelative:
		t, err := durationToRelative(v.Duration)
		if err != nil {
			return nil, err
		}
		return []byte{t}, nil
	case VpfNotPresent:
		return nil, nil
	}
	return nil, EncodeError("vpf", ErrInvalid)
}

// UnmarshalBinary unmarshals a ValidityPeriod stored in the given format.
//
// Returns the number of bytes read from the src, and any error detected during
// the unmarshalling.
func (v *ValidityPeriod) UnmarshalBinary(src []byte, vpf ValidityPeriodFormat) (int, error) {
	v.Format = VpfNotPresent
	switch vpf {
	case VpfAbsolute:
		t := Timestamp{}
		err := t.UnmarshalBinary(src)
		if err == nil {
			v.Time = t
			v.Format = vpf
		}
		return 7, err
	case VpfEnhanced:
		used, err := v.unmarshalVPEnhanced(src)
		if err == nil {
			v.Format = vpf
		}
		return used, err
	case VpfRelative:
		if len(src) < 1 {
			return 0, ErrUnderflow
		}
		v.Duration = relativeToDuration(src[0])
		v.Format = vpf
		return 1, nil
	case VpfNotPresent:
		return 0, nil
	}
	return 0, NewDecodeError("vpf", 0, ErrInvalid)
}

func (v *ValidityPeriod) unmarshalVPEnhanced(src []byte) (int, error) {
	if len(src) < 7 {
		return 0, ErrUnderflow
	}
	efi := src[0]
	evpf := EnhancedValidityPeriodFormat(efi & 0x7)
	used := 0
	d := time.Duration(0)
	switch evpf {
	case EvpfNotPresent:
	case EvpfRelative:
		d = relativeToDuration(src[1])
		used = 1
	case EvpfRelativeSeconds:
		d = time.Second * time.Duration(src[1])
		used = 1
	case EvpfRelativeHHMMSS:
		i := make([]int, 3)
		var err error
		for idx := 0; idx < 3; idx++ {
			i[idx], err = bcd.Decode(src[idx+1])
			if err != nil {
				return 4, NewDecodeError("enhanced", 1, err)
			}
		}
		d = time.Duration(i[0])*time.Hour + time.Duration(i[1])*time.Minute + time.Duration(i[2])*time.Second
		used = 3
	default:
		return 7, NewDecodeError("enhanced", 0, ErrInvalid)
	}
	for i := used + 1; i < 7; i++ {
		if src[i] != 0 {
			return used + 1, NewDecodeError("enhanced", i, ErrNonZero)
		}
	}
	v.EFI = efi
	v.Duration = d
	return 7, nil
}

// ValidityPeriodFormat identifies the format of the ValidityPeriod when encoded to binary.
type ValidityPeriodFormat byte

const (
	// VpfNotPresent indicates no VP is present.
	VpfNotPresent ValidityPeriodFormat = iota

	// VpfEnhanced indicates the VP is stored in enhanced format as per 3GPP TS
	// 23.040 Section 9.2.3.12.3.
	VpfEnhanced

	// VpfRelative indicates the VP is stored in relative format as per 3GPP TS
	// 23.040 Section 9.2.3.12.1.
	VpfRelative

	// VpfAbsolute indicates the VP is stored in absolute format as per 3GPP TS
	// 23.040 Section 9.2.3.12.2. The absolute format is the same format as the
	// SCTS.
	VpfAbsolute
)

// EnhancedValidityPeriodFormat identifies the subformat of the ValidityPeriod
// when encoded to binary in enhanced format, as per 3GPP TS 23.040 Section
// 9.2.3.12.3
type EnhancedValidityPeriodFormat byte

const (
	// EvpfNotPresent indicates no VP is present.
	EvpfNotPresent EnhancedValidityPeriodFormat = iota

	// EvpfRelative indicates the VP is stored in relative format as per 3GPP
	// TS 23.040 Section 9.2.3.12.1.
	EvpfRelative

	// EvpfRelativeSeconds indicates the VP is stored in relative format as an
	// integer number of seconds, from 0 to 255.
	EvpfRelativeSeconds

	// EvpfRelativeHHMMSS indicates the VP is stored in relative format as a
	// period of hours, minutes and seconds in semioctet format as per SCTS
	// time.
	EvpfRelativeHHMMSS

	// All other values currently reserved.
)

// The range of each relative format, as defined in 3GPP TS 23.040 Sections
// 9.2.3.12.1 and 9.2.3.12.3.
const (
	minRelativeVP = 5 * time.Minute
	maxRelativeVP = 63 * 7 * 24 * time.Hour
	minSecondsVP  = time.Second // TP-VP 0 is reserved
	maxSecondsVP  = 255 * time.Second
	maxHHMMSSVP   = 99*time.Hour + 59*time.Minute + 59*time.Second
)

// durationToRelative converts a duration into the TP-VP relative format
// defined in 3GPP TS 23.040 Section 9.2.3.12.1, rounding down to the
// resolution of the format:
//
//	0 to 143:   (TP-VP + 1) x 5 minutes
//	144 to 167: 12 hours + ((TP-VP - 143) x 30 minutes)
//	168 to 196: (TP-VP - 166) x 1 day
//	197 to 255: (TP-VP - 192) x 1 week
//
// An error is returned if the duration is outside the range of the format.
func durationToRelative(d time.Duration) (byte, error) {
	switch {
	case d < minRelativeVP || d > maxRelativeVP:
		return 0, EncodeError("duration", ErrInvalid)
	case d < time.Hour*12:
		return byte(d/(time.Minute*5)) - 1, nil
	case d < time.Hour*24:
		return 119 + byte(d/(time.Minute*30)), nil
	case d < time.Hour*24*30:
		return 166 + byte(d/(time.Hour*24)), nil
	default:
		return 192 + byte(d/(time.Hour*24*7)), nil
	}
}

func relativeToDuration(t byte) time.Duration {
	switch {
	case t < 144:
		return time.Minute * 5 * time.Duration(t+1)
	case t < 168:
		return time.Minute * 30 * time.Duration(t-119)
	case t < 197:
		return time.Hour * 24 * time.Duration(t-166)
	default:
		return time.Hour * 24 * 7 * time.Duration(t-192)
	}
}

func (vpf ValidityPeriodFormat) String() string {
	switch vpf {
	default:
		return "Unknown"
	case VpfNotPresent:
		return "Not Present"
	case VpfAbsolute:
		return "Absolute"
	case VpfRelative:
		return "Relative"
	case VpfEnhanced:
		return "Enhanced"
	}
}

func (evpf EnhancedValidityPeriodFormat) String() string {
	switch evpf {
	default:
		return "Unknown"
	case EvpfNotPresent:
		return "Not Present"
	case EvpfRelative:
		return "Relative"
	case EvpfRelativeSeconds:
		return "Relative Seconds"
	case EvpfRelativeHHMMSS:
		return "Relative HHMMSS"
	}
}
