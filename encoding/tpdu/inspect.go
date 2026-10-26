// SPDX-License-Identifier: MIT

package tpdu

// decodeRecorder carries a field's position and the shared, ordered findings.
// A nil recorder keeps the ordinary decoder's stopping behavior.
type decodeRecorder struct {
	departures *[]Departure
	reservedPI *bool
	field      string
	offset     int
}

func (r *decodeRecorder) child(field string, offset int) *decodeRecorder {
	if r == nil {
		return nil
	}
	if r.field != "" {
		field = r.field + "." + field
	}
	return &decodeRecorder{departures: r.departures, reservedPI: r.reservedPI, field: field, offset: r.offset + offset}
}

func (r *decodeRecorder) add(err error, rejected bool) {
	if r == nil {
		return
	}
	if r.field != "" {
		if de, ok := err.(DecodeError); ok && de.Field == "" {
			de.Field, de.Offset = r.field, de.Offset+r.offset
			err = de
		} else {
			err = NewDecodeError(r.field, r.offset, err)
		}
	}
	*r.departures = append(*r.departures, Departure{Err: err, Rejected: rejected})
}

func (r *decodeRecorder) note(field string, offset int, err error) {
	r.add(NewDecodeError(field, offset, err), false)
}

// reject reports whether decoding must stop, recording a rejection otherwise.
func (r *decodeRecorder) reject(err error) bool {
	if r == nil {
		return true
	}
	r.add(err, true)
	return false
}

// fill reports the bits ignored under 3GPP TS 23.040 Section 9.2.2.1:
// "Any unused bits shall be set to zero by the sending entity and shall be
// ignored by the receiving entity." Section 9.2.3.24 inserts header fill
// bits "up to the next septet boundary".
func (r *decodeRecorder) fill(src []byte, septets, header int) {
	if r == nil {
		return
	}
	fill := (7 - header%7) % 7
	if fill != 0 && 1+header < len(src) && src[1+header]&byte((1<<fill)-1) != 0 {
		r.note("fill", 1+header, ErrNonZero)
	}
	bits := septets * 7
	last, used := bits/8+1, bits%8
	if used != 0 && last < len(src) && src[last]>>used != 0 {
		r.note("fill", last, ErrNonZero)
	}
}

func (r *decodeRecorder) piReserved(offset int) {
	if r == nil {
		return
	}
	*r.reservedPI = true
	// 3GPP TS 23.040 Section 9.2.3.27: "If a Reserved bit is set to
	// '1' then the receiving entity shall ignore the setting."
	r.note("pi", offset, ErrReserved)
}
