// SPDX-License-Identifier: MIT

package sms

import (
	"errors"
)

var (
	// ErrClosed indicates that the collector has been closed and is no longer
	// accepting PDUs.
	ErrClosed = errors.New("closed")
	// ErrDcsConflict indicates the required encoding for user data conflicts with the
	// encoding specified in the template TPDU DCS.
	ErrDcsConflict = errors.New("DCS conflict")
	// ErrCompressedUserData indicates TP-UD is compressed using the algorithm
	// defined by 3GPP TS 23.042, which the library does not implement, so it
	// cannot be decoded as clear-text content, or encoded.
	ErrCompressedUserData = errors.New("compressed user data unsupported")
	// ErrDuplicateSegment indicates a segment has arrived for a reassembly
	// that already has that segment.
	// The segments are duplicates in terms of their concatentation information.
	// They may differ in other fields, particularly UD, but those fields
	// cannot be used to determine which of the two may better fit the
	// reassembly, so the first is kept and the second discarded.
	ErrDuplicateSegment = errors.New("duplicate segment")
	// ErrMissingSegment indicates a segment passed to Decode is nil, as the
	// Collector gives for a segment of a concatenated message that was not
	// received, or that the TPDU passed to Collect is nil.
	ErrMissingSegment = errors.New("missing segment")
	// ErrMissingOriginator indicates an SMS-SUBMIT was passed to Collect
	// without WithOriginator, which is required as the TPDU does not carry
	// its originating address.
	ErrMissingOriginator = errors.New("missing originator")
)
