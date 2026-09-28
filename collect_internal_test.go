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
