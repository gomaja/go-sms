// SPDX-License-Identifier: MIT

package sms

import (
	"slices"
	"sync"
	"time"

	"github.com/gomaja/go-sms/encoding/tpdu"
)

// Collector contains reassembly pipes that buffer concatenated TPDUs until a
// full set is available to be concatenated.
type Collector struct {
	mu            sync.Mutex // covers pipes, seq and closed
	pipes         map[pipeKey]*pipe
	seq           uint64 // numbers the pipes in the order they are created
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
		pipes: make(map[pipeKey]*pipe),
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

// Pipe describes a reassembly pipe, which holds the segments of a
// concatenated message collected so far.
type Pipe struct {
	// SmsType is the type of the segments, tpdu.SmsSubmit or tpdu.SmsDeliver.
	SmsType tpdu.SmsType

	// Originator is the originator given to Collect with WithOriginator, if
	// any.
	Originator string

	// Address is the TP-OA of an SMS-DELIVER, or the TP-DA of an SMS-SUBMIT.
	Address tpdu.Address

	// Ref is the reference number of the concatenated message, which is a
	// 16-bit one if Ref16Bit is set, and an 8-bit one otherwise.
	Ref      int
	Ref16Bit bool

	// Total is the number of segments of the concatenated message.
	Total int

	// Segments holds the segments collected so far, each at the index of its
	// sequence number less 1, and nil for each segment still missing, so its
	// length is the Total.
	Segments []*tpdu.TPDU
}

// Pipes returns a snapshot of the reassembly pipes, in the order in which
// they were created.
//
// This is intended for diagnostics. The snapshot is a copy, including of the
// TPDUs, so later calls of Collect do not change it, and changing it does not
// change the reassembly.
func (c *Collector) Pipes() []Pipe {
	c.mu.Lock()
	defer c.mu.Unlock()
	pipes := make([]*pipe, 0, len(c.pipes))
	for _, p := range c.pipes {
		pipes = append(pipes, p)
	}
	slices.SortFunc(pipes, func(a, b *pipe) int {
		switch {
		case a.seq < b.seq:
			return -1
		case a.seq > b.seq:
			return 1
		}
		return 0
	})
	out := make([]Pipe, len(pipes))
	for i, p := range pipes {
		out[i] = Pipe{
			SmsType:    p.key.st,
			Originator: p.key.originator,
			Address:    p.key.addr,
			Ref:        p.key.ref,
			Ref16Bit:   p.key.ref16,
			Total:      p.key.total,
			Segments:   cloneSegments(p.segments),
		}
	}
	return out
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

// CollectConfig contains the configuration options for Collect.
type CollectConfig struct {
	originator string
}

// CollectOption defines options for Collect.
type CollectOption interface {
	ApplyCollectOption(*CollectConfig)
}

// WithOriginator identifies the originating SME of the TPDU passed to
// Collect, for an SMS-SUBMIT, which does not carry it.
//
// It is the address given by the layer that carried the TPDU, such as the
// SM-RP-OA of the MAP MO-ForwardSM operation, or any other string that
// identifies the originator to the application. Collect compares originators
// as strings, so they must be given in the same form for every segment.
func WithOriginator(originator string) CollectOption {
	return originatorOption(originator)
}

type originatorOption string

func (o originatorOption) ApplyCollectOption(cfg *CollectConfig) {
	cfg.originator = string(o)
}

// Collect adds a TPDU to the collection.
//
// If the TPDU is not a segment of a concatenated message, or is the last
// missing segment of one, then the segments of the message are returned, in
// order. Otherwise the TPDU is kept until the other segments are collected,
// and no segment is returned.
//
// Only SMS-SUBMIT and SMS-DELIVER TPDUs are collected, as those are the types
// that 3GPP TS 23.040 Section 9.2.3.24.1 concatenates. Any other type is
// rejected with a tpdu.ErrUnsupportedSmsType, and nothing is stored.
//
// Section 9.2.3.24.1 says that the reference number of a concatenated message
// "together with the originating address and Service Centre address allows
// the receiving entity to discriminate between concatenated short messages
// sent from different originating SMEs and/or SCs". So segments are only
// reassembled together if they have the same originating address, reference
// number, reference size (8 or 16 bits) and total number of segments:
//
//   - The originating address of an SMS-DELIVER is its TP-OA.
//   - That of an SMS-SUBMIT is not in the TPDU, so it must be given with
//     WithOriginator, and ErrMissingOriginator is returned without it,
//     whether the TPDU is a segment or not. The TP-DA, which the Section
//     requires to be the same in every segment, must also be the same.
//
// An originator given for an SMS-DELIVER must also be the same, so it can
// carry the Service Centre address, which the Section recommends "should not
// be checked by the MS unless the application specifically requires such a
// check", or any other context the application needs to keep apart.
//
// A segment that duplicates one already collected, by its sequence number, is
// rejected with ErrDuplicateSegment, and the first one is kept.
//
// ErrMissingSegment is returned for a nil TPDU.
//
// Collect copies the TPDU, so the caller may change or reuse it once Collect
// returns. The TPDUs returned are not used by the Collector, and belong to
// the caller.
func (c *Collector) Collect(pdu *tpdu.TPDU, options ...CollectOption) ([]*tpdu.TPDU, error) {
	cfg := CollectConfig{}
	for _, option := range options {
		option.ApplyCollectOption(&cfg)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, ErrClosed
	}
	if pdu == nil {
		return nil, ErrMissingSegment
	}
	st := pdu.SmsType()
	switch st {
	case tpdu.SmsSubmit:
		if cfg.originator == "" {
			return nil, ErrMissingOriginator
		}
	case tpdu.SmsDeliver:
	default:
		return nil, tpdu.ErrUnsupportedSmsType(st)
	}
	t := cloneTPDU(pdu)
	// ConcatInfo ignores an IE whose total is 0 or whose sequence number is 0
	// or greater than the total, as 3GPP TS 23.040 Sections 9.2.3.24.1 and
	// 9.2.3.24.8 require, so the TPDU is then on its own, and the sequence
	// number always indexes the segments of the reassembly, whose key
	// includes the total.
	ci, ok := t.ConcatInfo()
	if !ok || ci.Total < 2 {
		// short circuit single segment - no need for a pipe
		return []*tpdu.TPDU{&t}, nil
	}
	segments, seqno := ci.Total, ci.Seqno
	key := newPipeKey(&t, cfg.originator, ci)
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
		c.seq++
		p = &pipe{key: key, seq: c.seq, segments: make([]*tpdu.TPDU, segments)}
		c.pipes[key] = p
	}
	p.segments[seqno-1] = &t
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

// pipeKey identifies the reassembly of a concatenated message.
type pipeKey struct {
	st         tpdu.SmsType
	originator string
	// addr is the TP-OA of an SMS-DELIVER, or the TP-DA of an SMS-SUBMIT.
	addr  tpdu.Address
	ref16 bool
	ref   int
	total int
}

// newPipeKey returns the key of the reassembly of an SMS-SUBMIT or
// SMS-DELIVER.
func newPipeKey(t *tpdu.TPDU, originator string, ci tpdu.ConcatInfo) pipeKey {
	st := t.SmsType()
	addr := t.OA
	if st == tpdu.SmsSubmit {
		addr = t.DA
	}
	return pipeKey{st, originator, addr, ci.Ref16Bit, ci.Ref, ci.Total}
}

// pipe is a buffer that contains the individual TPDUs in a concatenation set
// until the complete set is available or the reassembly times out.
type pipe struct {
	key      pipeKey
	seq      uint64
	cleanup  *time.Timer
	segments []*tpdu.TPDU
	frags    int
}
