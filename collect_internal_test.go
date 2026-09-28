// SPDX-License-Identifier: MIT

package sms

import (
	"testing"
	"time"

	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Collector without options, including the zero value, has the default
// timeout and limit, and options of zero or less remove them.
func TestCollectorDefaults(t *testing.T) {
	var zero Collector
	for _, c := range []*Collector{NewCollector(), &zero} {
		c.Pipes() // initialises the zero value
		assert.Equal(t, DefaultReassemblyTimeout, c.timeout)
		assert.Equal(t, DefaultReassemblyLimit, c.limit)
		p := tpdu.TPDU{OA: tpdu.Address{Addr: "1234", TOA: 0x91}}
		p.SetUDH(tpdu.UserDataHeader{{ID: 0, Data: []byte{1, 2, 1}}})
		_, err := c.Collect(&p)
		require.NoError(t, err)
		// the reassembly has a timer
		c.mu.Lock()
		for _, pp := range c.pipes {
			assert.NotNil(t, pp.timer)
		}
		assert.Equal(t, 1, c.held)
		c.mu.Unlock()
		c.Close()
		assert.Zero(t, c.held)
	}
	for _, d := range []time.Duration{0, -time.Second} {
		c := NewCollector(WithReassemblyTimeout(d), WithReassemblyLimit(int(d)))
		assert.Negative(t, c.timeout)
		assert.Negative(t, c.limit)
	}
	c := NewCollector(WithReassemblyTimeout(time.Minute), WithReassemblyLimit(7))
	assert.Equal(t, time.Minute, c.timeout)
	assert.Equal(t, 7, c.limit)
}

// A timer that fires as its reassembly completes may call expire after the
// same key has started a new reassembly. expire must leave that one alone,
// so the timer identifies its reassembly by id as well as key.
func TestCollectorLateExpiryKeepsNewReassembly(t *testing.T) {
	abandoned := 0
	c := NewCollector(WithExpiryHandler(func([]*tpdu.TPDU, error) { abandoned++ }))
	segment := func(seqno byte) *tpdu.TPDU {
		p := tpdu.TPDU{OA: tpdu.Address{Addr: "1234", TOA: 0x91}, UD: []byte{seqno}}
		p.SetUDH(tpdu.UserDataHeader{{ID: 0, Data: []byte{7, 2, seqno}}})
		return &p
	}
	_, err := c.Collect(segment(1))
	require.NoError(t, err)
	c.mu.Lock()
	var key pipeKey
	var id uint64
	for k, p := range c.pipes {
		key, id = k, p.id
	}
	c.mu.Unlock()
	segments, err := c.Collect(segment(2))
	require.NoError(t, err)
	require.Len(t, segments, 2)
	// the same key starts a new reassembly
	_, err = c.Collect(segment(1))
	require.NoError(t, err)
	require.Len(t, c.Pipes(), 1)

	// the timer of the completed reassembly, as if it had fired before it
	// was stopped
	c.settling.Add(1)
	c.expire(key, id)
	assert.Len(t, c.Pipes(), 1)
	assert.Zero(t, abandoned)

	// while that of the new one abandons it
	c.mu.Lock()
	p := c.pipes[key]
	c.mu.Unlock()
	require.NotNil(t, p)
	require.NotEqual(t, id, p.id)
	require.True(t, p.timer.Stop())
	c.expire(key, p.id)
	assert.Empty(t, c.Pipes())
	assert.Equal(t, 1, abandoned)
	c.Close()
}
