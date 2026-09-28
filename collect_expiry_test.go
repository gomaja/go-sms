// SPDX-License-Identifier: MIT

package sms_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expiry records the calls of an expiry handler.
type expiry struct {
	mu    sync.Mutex
	calls []expiryCall
	ch    chan expiryCall
}

type expiryCall struct {
	segments []*tpdu.TPDU
	reason   error
}

// closeWithin closes the Collector, failing if that takes longer than d.
func closeWithin(t *testing.T, c *sms.Collector, d time.Duration) {
	t.Helper()
	closed := make(chan struct{})
	go func() {
		c.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(d):
		t.Fatalf("Close did not return within %s", d)
	}
}

func newExpiry() *expiry {
	return &expiry{ch: make(chan expiryCall, 1024)}
}

func (e *expiry) handler(segments []*tpdu.TPDU, reason error) {
	e.mu.Lock()
	e.calls = append(e.calls, expiryCall{segments, reason})
	e.mu.Unlock()
	e.ch <- expiryCall{segments, reason}
}

func (e *expiry) wait(t *testing.T, d time.Duration) expiryCall {
	t.Helper()
	select {
	case c := <-e.ch:
		return c
	case <-time.After(d):
		t.Fatalf("no expiry within %s", d)
	}
	return expiryCall{}
}

func (e *expiry) none(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case c := <-e.ch:
		t.Fatalf("unexpected expiry %v", c)
	case <-time.After(d):
	}
}

// recovered calls f and returns what it panics with, if anything.
func recovered(f func()) (r any) {
	defer func() { r = recover() }()
	f()
	return nil
}

// An expiry handler that panics, called by Collect or Close in a goroutine
// that recovers, as an HTTP server recovers a handler, does not leave the
// Collector waiting for it: Close returns, and so does every later Close.
// The reassemblies the panicking call had yet to pass to the handler are
// discarded.
func TestCollectorHandlerPanic(t *testing.T) {
	t.Run("collect", func(t *testing.T) {
		var calls atomic.Int32
		c := sms.NewCollector(sms.WithReassemblyLimit(1),
			sms.WithExpiryHandler(func([]*tpdu.TPDU, error) {
				calls.Add(1)
				panic("handler")
			}))
		_, err := c.Collect(deliverSegment("1", 1, 2, 1, "a"))
		require.NoError(t, err)
		// the segment of 2 abandons the reassembly of 1, to keep within
		// the limit, and the handler panics.
		assert.Equal(t, "handler", recovered(func() {
			_, _ = c.Collect(deliverSegment("2", 1, 2, 1, "b"))
		}))
		assert.Equal(t, int32(1), calls.Load())
		// the Collector goes on, with the segment of 2 held
		out, err := c.Collect(deliverSegment("2", 1, 2, 2, "c"))
		require.NoError(t, err)
		assert.Equal(t, "bc", decoded(t, out))
		closeWithin(t, c, time.Second)
		closeWithin(t, c, time.Second)
		assert.Equal(t, int32(1), calls.Load())
	})
	t.Run("close", func(t *testing.T) {
		var calls atomic.Int32
		c := sms.NewCollector(sms.WithExpiryHandler(func([]*tpdu.TPDU, error) {
			calls.Add(1)
			panic("handler")
		}))
		for _, oa := range []string{"1", "2"} {
			_, err := c.Collect(deliverSegment(oa, 1, 2, 1, "a"))
			require.NoError(t, err)
		}
		assert.Equal(t, "handler", recovered(c.Close))
		// the reassembly of 2, which the handler was yet to be passed, is
		// discarded
		assert.Equal(t, int32(1), calls.Load())
		closeWithin(t, c, time.Second)
		closeWithin(t, c, time.Second)
		assert.Empty(t, c.Pipes())
		_, err := c.Collect(deliverSegment("3", 1, 2, 1, "a"))
		assert.Equal(t, sms.ErrClosed, err)
		assert.Equal(t, int32(1), calls.Load())
	})
}

// The timeout runs from the first segment, and later segments do not extend
// it, as WithReassemblyTimeout says: segments 200ms apart with a timeout of
// 300ms expire at 300ms, with the first two segments, and the third starts a
// new reassembly.
func TestCollectorTimeoutFromFirstSegment(t *testing.T) {
	e := newExpiry()
	c := sms.NewCollector(sms.WithReassemblyTimeout(300*time.Millisecond), sms.WithExpiryHandler(e.handler))
	defer c.Close()
	start := time.Now()
	for seqno := byte(1); seqno <= 3; seqno++ {
		if seqno > 1 {
			time.Sleep(time.Until(start.Add(time.Duration(seqno-1) * 200 * time.Millisecond)))
		}
		out, err := c.Collect(deliverSegment("1234", 7, 3, seqno, "x"))
		require.NoError(t, err)
		assert.Nil(t, out, "segment %d", seqno)
	}
	call := e.wait(t, time.Second)
	elapsed := time.Since(start)
	assert.Equal(t, sms.ErrReassemblyTimeout, call.reason)
	require.Len(t, call.segments, 3)
	assert.NotNil(t, call.segments[0])
	assert.NotNil(t, call.segments[1])
	assert.Nil(t, call.segments[2])
	assert.Less(t, elapsed, 400*time.Millisecond+200*time.Millisecond)
	// the third segment is in a new reassembly
	pipes := c.Pipes()
	require.Len(t, pipes, 1)
	assert.Equal(t, []*tpdu.TPDU{nil, nil, deliverSegment("1234", 7, 3, 3, "x")}, pipes[0].Segments)
}

// The expiry handler gets the segments in place, with nil for each missing
// one, which Decode and IsCompleteMessage report rather than panic on.
func TestCollectorExpiredSegments(t *testing.T) {
	e := newExpiry()
	c := sms.NewCollector(sms.WithReassemblyTimeout(time.Millisecond), sms.WithExpiryHandler(e.handler))
	defer c.Close()
	out, err := c.Collect(deliverSegment("1234", 7, 3, 2, "b"))
	require.NoError(t, err)
	require.Nil(t, out)
	call := e.wait(t, time.Second)
	assert.Equal(t, sms.ErrReassemblyTimeout, call.reason)
	assert.Equal(t, []*tpdu.TPDU{nil, deliverSegment("1234", 7, 3, 2, "b"), nil}, call.segments)
	assert.NotPanics(t, func() {
		assert.False(t, sms.IsCompleteMessage(call.segments))
		msg, err := sms.Decode(call.segments)
		assert.Equal(t, sms.ErrMissingSegment, err)
		assert.Nil(t, msg)
	})
	assert.Empty(t, c.Pipes())
}

// Close passes each partial reassembly to the expiry handler, in the order
// they were started, before it returns, and nothing after. Collect then
// returns ErrClosed and Pipes is empty.
func TestCollectorCloseSettles(t *testing.T) {
	e := newExpiry()
	c := sms.NewCollector(sms.WithReassemblyTimeout(time.Hour), sms.WithExpiryHandler(e.handler))
	for _, ref := range []byte{9, 3, 5} {
		_, err := c.Collect(deliverSegment("1234", ref, 2, 1, "a"))
		require.NoError(t, err)
	}
	closeWithin(t, c, time.Second)
	e.mu.Lock()
	calls := e.calls
	e.mu.Unlock()
	require.Len(t, calls, 3)
	for i, ref := range []byte{9, 3, 5} {
		assert.Equal(t, sms.ErrClosed, calls[i].reason)
		assert.Equal(t, []*tpdu.TPDU{deliverSegment("1234", ref, 2, 1, "a"), nil}, calls[i].segments)
	}
	assert.Empty(t, c.Pipes())
	out, err := c.Collect(deliverSegment("1234", 9, 2, 2, "b"))
	assert.Equal(t, sms.ErrClosed, err)
	assert.Nil(t, out)
	c.Close()
	e.mu.Lock()
	assert.Len(t, e.calls, 3)
	e.mu.Unlock()
}

// Without a handler, Close discards the partial reassemblies.
func TestCollectorCloseWithoutHandler(t *testing.T) {
	c := sms.NewCollector(sms.WithReassemblyTimeout(time.Millisecond))
	_, err := c.Collect(deliverSegment("1234", 9, 2, 1, "a"))
	require.NoError(t, err)
	c.Close()
	assert.Empty(t, c.Pipes())
}

// A zero timeout keeps a reassembly until Close.
func TestCollectorNoTimeout(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		e := newExpiry()
		c := sms.NewCollector(sms.WithReassemblyTimeout(d), sms.WithExpiryHandler(e.handler))
		_, err := c.Collect(deliverSegment("1234", 9, 2, 1, "a"))
		require.NoError(t, err)
		e.none(t, 50*time.Millisecond)
		assert.Len(t, c.Pipes(), 1)
		c.Close()
		assert.Equal(t, sms.ErrClosed, e.wait(t, time.Second).reason)
	}
}

// A completed reassembly leaves no timer for Close to wait for.
func TestCollectorCloseAfterComplete(t *testing.T) {
	e := newExpiry()
	c := sms.NewCollector(sms.WithReassemblyTimeout(time.Hour), sms.WithExpiryHandler(e.handler))
	for seqno := byte(1); seqno <= 2; seqno++ {
		_, err := c.Collect(deliverSegment("1234", 9, 2, seqno, "a"))
		require.NoError(t, err)
	}
	closeWithin(t, c, time.Second)
	e.none(t, 10*time.Millisecond)
}

// A second Close returns once the first has, so no handler runs after it
// returns either.
func TestCollectorSecondCloseWaits(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	c := sms.NewCollector(sms.WithExpiryHandler(func([]*tpdu.TPDU, error) {
		close(entered)
		<-release
	}))
	_, err := c.Collect(deliverSegment("1234", 9, 2, 1, "a"))
	require.NoError(t, err)
	first := make(chan struct{})
	go func() {
		c.Close()
		close(first)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Close did not call the handler")
	}
	second := make(chan struct{})
	go func() {
		c.Close()
		close(second)
	}()
	select {
	case <-second:
		t.Fatal("second Close returned while the first is settling")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-first
	<-second
}

// The handler may call Collect and Pipes.
func TestCollectorHandlerReentry(t *testing.T) {
	done := make(chan error, 1)
	var c *sms.Collector
	c = sms.NewCollector(sms.WithReassemblyTimeout(time.Millisecond),
		sms.WithExpiryHandler(func(segs []*tpdu.TPDU, reason error) {
			_ = c.Pipes()
			out, err := c.Collect(deliverSegment("5678", 1, 1, 1, "x"))
			if err == nil && len(out) != 1 {
				err = sms.ErrMissingSegment
			}
			if reason == sms.ErrReassemblyTimeout {
				done <- err
			}
		}))
	_, err := c.Collect(deliverSegment("1234", 9, 2, 1, "a"))
	require.NoError(t, err)
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("handler did not complete")
	}
	c.Close()
}

// No expiry handler runs once Close has returned, even when timers fire as
// it runs, and each partial reassembly is passed to the handler exactly once.
func TestCollectorNoExpiryAfterClose(t *testing.T) {
	var late atomic.Int32
	for i := 0; i < 500; i++ {
		var closed atomic.Bool
		var calls atomic.Int32
		c := sms.NewCollector(
			sms.WithReassemblyTimeout(time.Duration(i%20)*time.Microsecond),
			sms.WithExpiryHandler(func(_ []*tpdu.TPDU, _ error) {
				time.Sleep(10 * time.Microsecond)
				if closed.Load() {
					late.Add(1)
				}
				calls.Add(1)
			}))
		_, err := c.Collect(deliverSegment("1234", 9, 2, 1, "a"))
		require.NoError(t, err)
		time.Sleep(time.Duration(i%20) * time.Microsecond)
		c.Close()
		closed.Store(true)
		assert.Equal(t, int32(1), calls.Load(), "run %d", i)
	}
	assert.Zero(t, late.Load())
}

// Each segment ends up exactly once either in a message returned by Collect
// or in an expiry handler call, when timers race with the segments that
// complete a message.
func TestCollectorSettlesOnce(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	count := func(segs []*tpdu.TPDU) {
		mu.Lock()
		defer mu.Unlock()
		for _, s := range segs {
			if s != nil {
				seen[string(s.UD)]++
			}
		}
	}
	c := sms.NewCollector(sms.WithReassemblyTimeout(20*time.Microsecond),
		sms.WithExpiryHandler(func(segs []*tpdu.TPDU, _ error) { count(segs) }))
	const n = 2000
	for i := 0; i < n; i++ {
		ref := byte(i)
		for seqno := byte(1); seqno <= 2; seqno++ {
			out, err := c.Collect(deliverSegment("1234", ref, 2, seqno, string(rune(i))+string(rune('0'+seqno))))
			require.NoError(t, err)
			count(out)
		}
	}
	c.Close()
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, seen, 2*n)
	for k, v := range seen {
		assert.Equal(t, 1, v, "%q", k)
	}
}

// Concurrent Collect, Pipes, timeouts and Close: every segment accepted by
// Collect is settled exactly once, by a Collect or the expiry handler, and
// no handler runs once Close has returned.
func TestCollectorConcurrentStress(t *testing.T) {
	for _, limit := range []int{0, 8} {
		t.Run(fmt.Sprintf("limit %d", limit), func(t *testing.T) {
			collectorStress(t, limit)
		})
	}
}

// collectorStress runs TestCollectorConcurrentStress with the limit given,
// which is the default for 0.
func collectorStress(t *testing.T, limit int) {
	options := []sms.CollectorOption{}
	if limit != 0 {
		options = append(options, sms.WithReassemblyLimit(limit))
	}
	var closed atomic.Bool
	var late atomic.Int32
	var mu sync.Mutex
	accepted := map[string]int{}
	settled := map[string]int{}
	settle := func(segs []*tpdu.TPDU) {
		mu.Lock()
		defer mu.Unlock()
		for _, s := range segs {
			if s != nil {
				settled[string(s.UD)]++
			}
		}
	}
	c := sms.NewCollector(append(options,
		sms.WithReassemblyTimeout(2*time.Millisecond),
		sms.WithExpiryHandler(func(segs []*tpdu.TPDU, _ error) {
			if closed.Load() {
				late.Add(1)
			}
			settle(segs)
		}))...)
	const workers = 8
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			oa := fmt.Sprintf("%d", 1000+w)
			for m := 0; ; m++ {
				// segments 3, 1, 1 again and 2, with segment 2 left out of
				// every fifth message
				for _, seqno := range []byte{3, 1, 1, 2} {
					if seqno == 2 && m%5 == 0 {
						continue
					}
					ud := fmt.Sprintf("%d/%d/%d", w, m, seqno)
					out, err := c.Collect(deliverSegment(oa, byte(m), 3, seqno, ud))
					switch err {
					case sms.ErrClosed:
						return
					case sms.ErrDuplicateSegment:
						continue
					case nil:
					default:
						t.Errorf("collect: %v", err)
						return
					}
					mu.Lock()
					accepted[ud]++
					mu.Unlock()
					settle(out)
				}
			}
		}(w)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !closed.Load() {
			pipes := c.Pipes()
			n := 0
			for _, p := range pipes {
				assert.Len(t, p.Segments, 3)
				n += held([]sms.Pipe{p})
			}
			if limit > 0 {
				assert.LessOrEqual(t, n, limit)
			}
		}
	}()
	time.Sleep(200 * time.Millisecond)
	closeWithin(t, c, 5*time.Second)
	closed.Store(true)
	wg.Wait()
	assert.Zero(t, late.Load())
	mu.Lock()
	defer mu.Unlock()
	// A duplicate is accepted again once the reassembly holding the first
	// has expired, and then settled again.
	assert.NotEmpty(t, accepted)
	assert.Equal(t, accepted, settled)
}
