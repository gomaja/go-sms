// SPDX-License-Identifier: MIT

package sms_test

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// held returns the number of segments held in the pipes.
func held(pipes []sms.Pipe) int {
	n := 0
	for _, p := range pipes {
		for _, s := range p.Segments {
			if s != nil {
				n++
			}
		}
	}
	return n
}

// When a segment would take the segments held beyond the limit, the oldest
// reassemblies are abandoned until it does not.
func TestCollectorLimit(t *testing.T) {
	e := newExpiry()
	c := sms.NewCollector(sms.WithReassemblyLimit(4), sms.WithExpiryHandler(e.handler))
	defer closeWithin(t, c, time.Second)
	collect := func(oa string, seqno byte) []*tpdu.TPDU {
		out, err := c.Collect(deliverSegment(oa, 1, 3, seqno, oa+fmt.Sprint(seqno)))
		require.NoError(t, err)
		return out
	}
	collect("1", 1)
	collect("2", 1)
	collect("1", 2)
	collect("3", 1)
	e.none(t, 10*time.Millisecond)
	assert.Equal(t, 4, held(c.Pipes()))
	// the fifth segment abandons the oldest reassembly, of 1
	collect("4", 1)
	call := e.wait(t, time.Second)
	assert.Equal(t, sms.ErrReassemblyLimit, call.reason)
	assert.Equal(t, []*tpdu.TPDU{deliverSegment("1", 1, 3, 1, "11"), deliverSegment("1", 1, 3, 2, "12"), nil}, call.segments)
	pipes := c.Pipes()
	require.Len(t, pipes, 3)
	for i, oa := range []string{"2", "3", "4"} {
		assert.Equal(t, oa, pipes[i].Address.Addr)
	}
	assert.Equal(t, 3, held(pipes))
	// a segment that completes a reassembly is not held
	collect("2", 2)
	collect("2", 3)
	assert.Equal(t, 2, held(c.Pipes()))
	e.none(t, 10*time.Millisecond)
}

// The oldest reassembly may be that of the segment, which then starts a new
// one.
func TestCollectorLimitEvictsOwnReassembly(t *testing.T) {
	e := newExpiry()
	c := sms.NewCollector(sms.WithReassemblyLimit(2), sms.WithExpiryHandler(e.handler))
	defer closeWithin(t, c, time.Second)
	for _, s := range []struct {
		oa    string
		seqno byte
	}{{"1", 1}, {"2", 1}, {"1", 2}} {
		out, err := c.Collect(deliverSegment(s.oa, 1, 3, s.seqno, s.oa+fmt.Sprint(s.seqno)))
		require.NoError(t, err)
		require.Nil(t, out)
	}
	call := e.wait(t, time.Second)
	assert.Equal(t, sms.ErrReassemblyLimit, call.reason)
	assert.Equal(t, []*tpdu.TPDU{deliverSegment("1", 1, 3, 1, "11"), nil, nil}, call.segments)
	pipes := c.Pipes()
	require.Len(t, pipes, 2)
	assert.Equal(t, "2", pipes[0].Address.Addr)
	assert.Equal(t, []*tpdu.TPDU{nil, deliverSegment("1", 1, 3, 2, "12"), nil}, pipes[1].Segments)
}

// A limit of zero or less removes the limit.
func TestCollectorNoLimit(t *testing.T) {
	for _, n := range []int{0, -1} {
		var mu sync.Mutex
		reasons := map[error]int{}
		c := sms.NewCollector(sms.WithReassemblyLimit(n),
			sms.WithExpiryHandler(func(_ []*tpdu.TPDU, reason error) {
				mu.Lock()
				reasons[reason]++
				mu.Unlock()
			}))
		for i := 0; i < sms.DefaultReassemblyLimit+100; i++ {
			_, err := c.Collect(deliverSegment(fmt.Sprint(i), 1, 2, 1, "x"))
			require.NoError(t, err)
		}
		mu.Lock()
		assert.Empty(t, reasons)
		mu.Unlock()
		assert.Len(t, c.Pipes(), sms.DefaultReassemblyLimit+100)
		closeWithin(t, c, 5*time.Second)
		assert.Equal(t, map[error]int{sms.ErrClosed: sms.DefaultReassemblyLimit + 100}, reasons)
	}
}

// By default a Collector holds at most DefaultReassemblyLimit segments, so a
// flood of first segments, each of a message of 255, does not grow it
// without bound: 51200 of them used to leave 51200 pipes and about 120 MiB.
func TestCollectorBoundedByDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("collects 51200 segments")
	}
	var mu sync.Mutex
	abandoned := 0
	for i, c := range []*sms.Collector{
		sms.NewCollector(sms.WithExpiryHandler(func(_ []*tpdu.TPDU, reason error) {
			if reason == sms.ErrReassemblyLimit {
				mu.Lock()
				abandoned++
				mu.Unlock()
			}
		})),
		{}, // the zero value, which has no handler
	} {
		runtime.GC()
		var before runtime.MemStats
		runtime.ReadMemStats(&before)
		for a := 0; a < 200; a++ {
			for ref := 0; ref < 256; ref++ {
				_, err := c.Collect(deliverSegment(fmt.Sprint(1000+a), byte(ref), 255, 1, "x"))
				require.NoError(t, err)
			}
		}
		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		pipes := c.Pipes()
		assert.Len(t, pipes, sms.DefaultReassemblyLimit)
		assert.Equal(t, fmt.Sprint(1000+199), pipes[len(pipes)-1].Address.Addr)
		// DefaultReassemblyLimit documents about 12 MiB for this, which
		// holds only if nothing keeps the abandoned reassemblies.
		growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
		assert.Less(t, growth, int64(20<<20), "heap growth %d", growth)
		if i == 0 {
			mu.Lock()
			assert.Equal(t, 200*256-sms.DefaultReassemblyLimit, abandoned)
			mu.Unlock()
		}
		closeWithin(t, c, 5*time.Second)
	}
}

// The zero value of a Collector is ready to use, with the default settings.
func TestCollectorZeroValue(t *testing.T) {
	var c sms.Collector
	out, err := c.Collect(deliverSegment("1234", 7, 2, 1, "a"))
	require.NoError(t, err)
	require.Nil(t, out)
	assert.Len(t, c.Pipes(), 1)
	out, err = c.Collect(deliverSegment("1234", 7, 2, 2, "b"))
	require.NoError(t, err)
	assert.Equal(t, "ab", decoded(t, out))
	c.Close()
	c.Close()
	_, err = c.Collect(deliverSegment("1234", 7, 2, 2, "b"))
	assert.Equal(t, sms.ErrClosed, err)

	var d sms.Collector
	d.Close()
	assert.Empty(t, d.Pipes())
}

// TestCollectorReleasesAbandoned checks the Collector keeps nothing of a
// reassembly it has abandoned, so the segments are unreachable once the
// expiry handler returns, as of the first garbage collection after it. A
// reassembly has a timer, and a stopped timer can stay in the runtime for a
// while, so it must not hold the reassembly, or a flood that evicts many
// reassemblies keeps them all for as long, beyond the memory the limit
// allows.
func TestCollectorReleasesAbandoned(t *testing.T) {
	var abandoned, freed atomic.Int64
	c := sms.NewCollector(sms.WithExpiryHandler(func(segments []*tpdu.TPDU, reason error) {
		if reason != sms.ErrReassemblyLimit {
			return
		}
		for _, s := range segments {
			if s != nil {
				abandoned.Add(1)
				runtime.SetFinalizer(s, func(*tpdu.TPDU) { freed.Add(1) })
			}
		}
	}))
	defer closeWithin(t, c, 5*time.Second)
	// The default limit holds 4096 reassemblies, each with a running timer,
	// and the next 1000 abandon the oldest.
	for i := 0; i < sms.DefaultReassemblyLimit+1000; i++ {
		_, err := c.Collect(deliverSegment(fmt.Sprint(i), byte(i), 2, 1, "x"))
		require.NoError(t, err)
	}
	require.Equal(t, int64(1000), abandoned.Load())
	runtime.GC()
	// finalizers run on their own goroutine, after the collection
	for i := 0; i < 200 && freed.Load() < abandoned.Load(); i++ {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	assert.Equal(t, abandoned.Load(), freed.Load())
}
