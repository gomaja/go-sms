// SPDX-License-Identifier: MIT

package sms

import (
	"container/list"
	"sync"
	"time"

	"github.com/gomaja/go-sms/encoding/tpdu"
)

// Collector contains reassembly pipes that buffer concatenated TPDUs until a
// full set is available to be concatenated.
//
// Each reassembly is settled exactly once: either Collect returns its
// segments, complete, or they are passed to the expiry handler, if any, when
// the reassembly is abandoned. A Collector is safe for concurrent use by
// multiple goroutines.
//
// The zero value is a Collector with the default settings, as NewCollector
// returns without options.
type Collector struct {
	mu     sync.Mutex // covers pipes, order, held and closed
	pipes  map[pipeKey]*pipe
	order  list.List // the pipes, in the order they were created
	held   int       // the number of segments in the pipes
	closed bool

	// timeout and limit are 0 for their default, and negative for none.
	timeout time.Duration
	limit   int
	handler func([]*tpdu.TPDU, error)

	// settling counts the reassembly timers that may yet call the handler,
	// which Close waits for.
	settling sync.WaitGroup

	// done is closed once Close has settled every reassembly.
	done chan struct{}
}

// CollectorOption alters the behaviour of a Collector.
type CollectorOption interface {
	ApplyCollectorOption(*Collector)
}

// DefaultReassemblyTimeout is the reassembly timeout of a Collector created
// without WithReassemblyTimeout.
//
// An SC delivers each segment as a separate short message, and retries one it
// could not deliver, so the last segment can follow the first by hours when
// the recipient is out of coverage between them. Memory is bounded by the
// reassembly limit, not the timeout, so the default is long enough for a
// segment retried later that day, and short enough that a reassembly missing
// a segment is reported within a day.
const DefaultReassemblyTimeout = 24 * time.Hour

// DefaultReassemblyLimit is the reassembly limit of a Collector created
// without WithReassemblyLimit.
//
// A segment held takes about 550 octets, with 140 of UD, and a reassembly
// about 2.5 KiB more, mostly for its index of up to 255 segments, so the
// default bounds the memory of a Collector to about 12 MiB, even when a flood
// of first segments starts a reassembly with each, while it holds some 2000
// messages of 3 segments awaiting their last.
const DefaultReassemblyLimit = 4096

type reassemblyTimeoutOption time.Duration

func (o reassemblyTimeoutOption) ApplyCollectorOption(c *Collector) {
	c.timeout = time.Duration(o)
	if c.timeout <= 0 {
		c.timeout = -1
	}
}

// WithReassemblyTimeout limits the time allowed to collect all the segments
// of a concatenated message to d, counted from the collection of its first
// segment. Later segments do not extend it.
//
// Once d has passed, the reassembly is abandoned: its segments are passed to
// the expiry handler, with ErrReassemblyTimeout, and discarded, and a segment
// of the message collected later starts a new reassembly.
//
// A duration of zero or less disables the timeout. Without this option the
// timeout is DefaultReassemblyTimeout.
func WithReassemblyTimeout(d time.Duration) CollectorOption {
	return reassemblyTimeoutOption(d)
}

type reassemblyLimitOption int

func (o reassemblyLimitOption) ApplyCollectorOption(c *Collector) {
	c.limit = int(o)
	if c.limit <= 0 {
		c.limit = -1
	}
}

// WithReassemblyLimit limits to n the number of segments the Collector holds
// in the reassemblies it has not completed, which bounds its memory.
//
// When a segment would take the number beyond n, the oldest reassemblies, by
// their first segment, are abandoned until it does not: their segments are
// passed to the expiry handler, with ErrReassemblyLimit, and discarded. That
// may include the reassembly of the segment itself, which then starts a new
// one. A segment that completes a reassembly is not held, so it abandons
// none. As a concatenated message has up to 255 segments, a limit below 254
// prevents the reassembly of the longest.
//
// A limit of zero or less removes the limit. Without this option the limit
// is DefaultReassemblyLimit.
func WithReassemblyLimit(n int) CollectorOption {
	return reassemblyLimitOption(n)
}

type expiryHandlerOption func([]*tpdu.TPDU, error)

func (o expiryHandlerOption) ApplyCollectorOption(c *Collector) {
	c.handler = o
}

// WithExpiryHandler specifies a function to be passed the segments of each
// reassembly the Collector abandons, with the reason:
//
//   - ErrReassemblyTimeout, when the reassembly timeout has passed.
//   - ErrReassemblyLimit, when the reassembly limit is reached.
//   - ErrClosed, when the Collector is closed.
//
// The segments are in place, each at the index of its sequence number less 1,
// with nil for each segment that was not collected, so their number is the
// total of the concatenated message. Decode returns ErrMissingSegment for
// them, and IsCompleteMessage returns false. They belong to the handler.
//
// The handler is called from the goroutine that calls Collect or Close, or
// from one of the Collector's own, and may be called concurrently. It may call Collect
// and Pipes, but must not call Close, which waits for it to return.
//
// Without a handler the segments are discarded.
func WithExpiryHandler(h func(segments []*tpdu.TPDU, reason error)) CollectorOption {
	return expiryHandlerOption(h)
}

// NewCollector creates a Collector.
//
// Without options, a Collector abandons a reassembly once
// DefaultReassemblyTimeout has passed since its first segment, and holds at
// most DefaultReassemblyLimit segments, so its memory is bounded whatever it
// is given to collect. It has no expiry handler.
func NewCollector(options ...CollectorOption) *Collector {
	c := Collector{}
	for _, o := range options {
		o.ApplyCollectorOption(&c)
	}
	c.init()
	return &c
}

// init initialises the state of the Collector, if it is not yet, including
// the default timeout and limit, if none was given.
func (c *Collector) init() {
	if c.pipes != nil {
		return
	}
	c.pipes = make(map[pipeKey]*pipe)
	c.done = make(chan struct{})
	if c.timeout == 0 {
		c.timeout = DefaultReassemblyTimeout
	}
	if c.limit == 0 {
		c.limit = DefaultReassemblyLimit
	}
}

// Close shuts down the Collector.
//
// Each partial reassembly, one for which Collect has not yet returned the
// segments, is passed to the expiry handler with ErrClosed, in the order the
// reassemblies were started, or discarded without a handler. Close returns
// once that is done, and once any call of the handler for a timeout has
// returned, so no handler runs after Close returns. Collect then returns
// ErrClosed, and Pipes returns none.
//
// Close may be called more than once, and from several goroutines. Each call
// returns once the first has.
func (c *Collector) Close() {
	c.mu.Lock()
	c.init()
	if c.closed {
		c.mu.Unlock()
		<-c.done
		return
	}
	c.closed = true
	pipes := c.orderedPipes()
	for _, p := range pipes {
		c.remove(p)
	}
	c.mu.Unlock()
	c.settling.Wait()
	for _, p := range pipes {
		c.settle(p, ErrClosed)
	}
	close(c.done)
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
	c.init()
	pipes := c.orderedPipes()
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

// orderedPipes returns the pipes in the order in which they were created.
func (c *Collector) orderedPipes() []*pipe {
	pipes := make([]*pipe, 0, len(c.pipes))
	for e := c.order.Front(); e != nil; e = e.Next() {
		pipes = append(pipes, e.Value.(*pipe))
	}
	return pipes
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
// ErrMissingSegment is returned for a nil TPDU, and ErrClosed once the
// Collector is closed.
//
// A segment that the Collector holds may make it abandon reassemblies to keep
// within its reassembly limit, which Collect passes to the expiry handler
// before it returns.
//
// Collect copies the TPDU, so the caller may change or reuse it once Collect
// returns. The TPDUs returned are not used by the Collector, and belong to
// the caller.
func (c *Collector) Collect(pdu *tpdu.TPDU, options ...CollectOption) ([]*tpdu.TPDU, error) {
	cfg := CollectConfig{}
	for _, option := range options {
		option.ApplyCollectOption(&cfg)
	}
	segments, abandoned, err := c.collect(pdu, cfg)
	for _, p := range abandoned {
		c.settle(p, ErrReassemblyLimit)
		c.settling.Done()
	}
	return segments, err
}

// collect collects the TPDU, and returns the segments of the message it
// completes, if any, and the reassemblies it abandoned to keep within the
// limit, which the caller must settle, and then mark as done.
func (c *Collector) collect(pdu *tpdu.TPDU, cfg CollectConfig) ([]*tpdu.TPDU, []*pipe, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.init()
	if c.closed {
		return nil, nil, ErrClosed
	}
	if pdu == nil {
		return nil, nil, ErrMissingSegment
	}
	st := pdu.SmsType()
	switch st {
	case tpdu.SmsSubmit:
		if cfg.originator == "" {
			return nil, nil, ErrMissingOriginator
		}
	case tpdu.SmsDeliver:
	default:
		return nil, nil, tpdu.ErrUnsupportedSmsType(st)
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
		return []*tpdu.TPDU{&t}, nil, nil
	}
	key := newPipeKey(&t, cfg.originator, ci)
	p := c.pipes[key]
	if p != nil {
		if p.segments[ci.Seqno-1] != nil {
			return nil, nil, ErrDuplicateSegment
		}
		if p.frags+1 == ci.Total {
			// The reassembly is complete, so it is removed before its timer
			// can abandon it. A timer that has fired, but is waiting for the
			// lock, finds it removed and leaves it.
			c.remove(p)
			p.segments[ci.Seqno-1] = &t
			return p.segments, nil, nil
		}
	}
	// The segment is to be held, so the oldest reassemblies make room for it.
	var abandoned []*pipe
	for c.limit > 0 && c.held >= c.limit {
		oldest := c.order.Front().Value.(*pipe)
		c.remove(oldest)
		abandoned = append(abandoned, oldest)
		if oldest == p {
			p = nil
		}
	}
	c.settling.Add(len(abandoned))
	if p == nil {
		p = &pipe{key: key, segments: make([]*tpdu.TPDU, ci.Total)}
		c.pipes[key] = p
		p.elem = c.order.PushBack(p)
		if c.timeout > 0 {
			c.settling.Add(1)
			p.timer = time.AfterFunc(c.timeout, func() { c.expire(p) })
		}
	}
	p.segments[ci.Seqno-1] = &t
	p.frags++
	c.held++
	return nil, abandoned, nil
}

// expire abandons the reassembly once its timeout has passed, unless it has
// been completed or abandoned already.
func (c *Collector) expire(p *pipe) {
	defer c.settling.Done()
	c.mu.Lock()
	if c.pipes[p.key] != p {
		c.mu.Unlock()
		return
	}
	c.unlink(p)
	c.mu.Unlock()
	c.settle(p, ErrReassemblyTimeout)
}

// unlink removes a pipe from the Collector.
func (c *Collector) unlink(p *pipe) {
	delete(c.pipes, p.key)
	c.order.Remove(p.elem)
	c.held -= p.frags
}

// remove removes a pipe from the Collector, and stops its timer.
func (c *Collector) remove(p *pipe) {
	c.unlink(p)
	c.stopTimer(p)
}

// stopTimer stops the timer of the pipe, if it has one that has not yet
// fired, so that it will not call expire.
func (c *Collector) stopTimer(p *pipe) {
	if p.timer != nil && p.timer.Stop() {
		c.settling.Done()
	}
}

// settle passes the segments of an abandoned reassembly to the handler.
func (c *Collector) settle(p *pipe, reason error) {
	if c.handler != nil {
		c.handler(p.segments, reason)
	}
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
// until the complete set is available or the reassembly is abandoned.
type pipe struct {
	key      pipeKey
	elem     *list.Element // in the order of the Collector
	timer    *time.Timer
	segments []*tpdu.TPDU
	frags    int
}
