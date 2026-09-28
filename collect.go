// SPDX-License-Identifier: MIT

package sms

import (
	"fmt"
	"sync"
	"time"

	"github.com/gomaja/go-sms/encoding/tpdu"
)

// Collector contains reassembly pipes that buffer concatenated TPDUs until a
// full set is available to be concatenated.
type Collector struct {
	mu            sync.Mutex // covers pipes and closed
	pipes         map[string]*pipe
	closed        bool
	duration      time.Duration
	expiryHandler func([]*tpdu.TPDU)
}

// CollectorOption alters the behaviour of a Collector.
type CollectorOption interface {
	ApplyCollectorOption(*Collector)
}

type reassemblyTimeoutOption struct {
	d  time.Duration
	eh func([]*tpdu.TPDU)
}

func (o reassemblyTimeoutOption) ApplyCollectorOption(c *Collector) {
	c.duration = o.d
	c.expiryHandler = o.eh
}

// WithReassemblyTimeout limits the time allowed for a collection of TPDUs to
// be collected.
//
// If the timer expires before the collection is complete then the collected
// TPDUs are passed to the expiryHandler. The expiry handler can be nil in
// which case the collected TPDUs are simply discarded.
//
// A zero duration disables the timeout.
func WithReassemblyTimeout(d time.Duration, eh func([]*tpdu.TPDU)) CollectorOption {
	return reassemblyTimeoutOption{d, eh}
}

// NewCollector creates a Collector.
func NewCollector(options ...CollectorOption) *Collector {
	c := Collector{
		pipes: make(map[string]*pipe),
	}
	for _, o := range options {
		o.ApplyCollectorOption(&c)
	}
	return &c
}

// Close shuts down the Collector and all active pipes.
func (c *Collector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	for _, p := range c.pipes {
		if p.cleanup != nil {
			p.cleanup.Stop()
		}
	}
}

// Pipes returns a snapshot of the reassembly pipes.
//
// This is intended for diagnostics. The snapshot is a copy, including of the
// TPDUs, so later calls of Collect do not change it, and changing it does not
// change the reassembly.
func (c *Collector) Pipes() map[string][]*tpdu.TPDU {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := make(map[string][]*tpdu.TPDU, len(c.pipes))
	for k, v := range c.pipes {
		m[k] = cloneSegments(v.segments)
	}
	return m
}

// cloneSegments returns a copy of the segments of a pipe, including of each
// TPDU, and nil for each missing segment.
func cloneSegments(segments []*tpdu.TPDU) []*tpdu.TPDU {
	out := make([]*tpdu.TPDU, len(segments))
	for i, s := range segments {
		if s != nil {
			c := cloneTPDU(s)
			out[i] = &c
		}
	}
	return out
}

// Collect adds a TPDU to the collection.
//
// If all the components of a concatenated TPDU are available then they are
// returned.
//
// Only SMS-SUBMIT and SMS-DELIVER TPDUs are collected, as those are the types
// that 3GPP TS 23.040 Section 9.2.3.24.1 concatenates. Any other type is
// rejected with a tpdu.ErrUnsupportedSmsType, and nothing is stored.
func (c *Collector) Collect(pdu tpdu.TPDU) ([]*tpdu.TPDU, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, ErrClosed
	}
	switch st := pdu.SmsType(); st {
	case tpdu.SmsSubmit, tpdu.SmsDeliver:
	default:
		return nil, tpdu.ErrUnsupportedSmsType(st)
	}
	// ConcatInfo ignores an IE whose total is 0 or whose sequence number is 0
	// or greater than the total, as 3GPP TS 23.040 Sections 9.2.3.24.1 and
	// 9.2.3.24.8 require, so the TPDU is then on its own, and the sequence
	// number always indexes the segments of the reassembly, whose key
	// includes the total.
	ci, ok := pdu.ConcatInfo()
	if !ok || ci.Total < 2 {
		// short circuit single segment - no need for a pipe
		return []*tpdu.TPDU{&pdu}, nil
	}
	segments, seqno := ci.Total, ci.Seqno
	key := pduKey(pdu, ci)
	p, ok := c.pipes[key]
	if ok {
		if p.segments[seqno-1] != nil {
			return nil, ErrDuplicateSegment
		}
		if p.cleanup != nil && !p.cleanup.Stop() {
			// timer has fired, but cleanup hasn't been performed yet - so need
			// a new pipe
			ok = false
		}
	}
	if !ok {
		p = &pipe{nil, make([]*tpdu.TPDU, segments), 0}
		c.pipes[key] = p
	}
	p.segments[seqno-1] = &pdu
	p.frags++
	if p.frags == segments {
		delete(c.pipes, key)
		return p.segments, nil
	}
	if c.duration != 0 {
		p.cleanup = time.AfterFunc(c.duration, func() {
			c.mu.Lock()
			m := c.pipes[key]
			if m == p {
				delete(c.pipes, key)
			}
			c.mu.Unlock()
			if c.expiryHandler != nil {
				c.expiryHandler(p.segments)
			}
		})
	}
	return nil, nil
}

// pduKey returns the key of the reassembly of an SMS-SUBMIT or SMS-DELIVER.
func pduKey(pdu tpdu.TPDU, ci tpdu.ConcatInfo) string {
	st := pdu.SmsType()
	// 8-bit and 16-bit references with the same value are distinct.
	concatRef := fmt.Sprintf("%d", ci.Ref)
	if ci.Ref16Bit {
		concatRef += "/16"
	}
	addr := pdu.OA
	if st == tpdu.SmsSubmit {
		addr = pdu.DA
	}
	return fmt.Sprintf("%d:%02x:%s:%s:%d", st, addr.TOA, addr.Addr, concatRef, ci.Total)
}

// pipe is a buffer that contains the individual TPDUs in a concatenation set
// until the complete set is available or the reassembly times out.
type pipe struct {
	cleanup  *time.Timer
	segments []*tpdu.TPDU
	frags    int
}
