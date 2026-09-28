// SPDX-License-Identifier: MIT

package sms_test

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// longMsg needs two segments.
var longMsg = []byte(strings.Repeat("a", 200))

func concatRef(t *testing.T, pdus []tpdu.TPDU) int {
	t.Helper()
	require.Greater(t, len(pdus), 1)
	ci, ok := pdus[0].ConcatInfo()
	require.True(t, ok)
	for _, p := range pdus[1:] {
		c, ok := p.ConcatInfo()
		require.True(t, ok)
		require.Equal(t, ci.Ref, c.Ref)
	}
	return ci.Ref
}

// Consecutive concatenated messages from Encode get different references,
// as 3GPP TS 23.040 Section 9.2.3.24.1 requires to tell them apart, and the
// TP-MR goes on incrementing, as Section 9.2.3.6 requires.
func TestEncodeDefaultCounters(t *testing.T) {
	a, err := sms.Encode(longMsg, sms.To("1234"))
	require.NoError(t, err)
	b, err := sms.Encode(longMsg, sms.To("1234"))
	require.NoError(t, err)
	assert.NotEqual(t, concatRef(t, a), concatRef(t, b))
	assert.Equal(t, a[0].MR+1, a[1].MR)
	assert.Equal(t, a[1].MR+1, b[0].MR)
	assert.Equal(t, b[0].MR+1, b[1].MR)
	// Encoders created without counters share them with Encode.
	c, err := sms.NewEncoder(sms.AsSubmit).Encode(longMsg)
	require.NoError(t, err)
	d, err := sms.NewEncoder(sms.AsSubmit).Encode(longMsg)
	require.NoError(t, err)
	refs := []int{concatRef(t, a), concatRef(t, b), concatRef(t, c), concatRef(t, d)}
	assert.Equal(t, (refs[0]+1)&0xff, refs[1])
	assert.Equal(t, (refs[1]+1)&0xff, refs[2])
	assert.Equal(t, (refs[2]+1)&0xff, refs[3])
	assert.Equal(t, b[1].MR+1, c[0].MR)
	assert.Equal(t, c[1].MR+1, d[0].MR)
}

// The counters given by options are used instead of the shared ones, by
// NewEncoder and per call.
func TestEncoderCounterOptions(t *testing.T) {
	mr := sms.NewCounter(41)
	ref := sms.NewCounter(299)
	out, err := sms.Encode(longMsg, sms.WithMR(mr), sms.WithConcatRef(ref))
	require.NoError(t, err)
	assert.Equal(t, byte(42), out[0].MR)
	assert.Equal(t, byte(43), out[1].MR)
	assert.Equal(t, 300&0xff, concatRef(t, out))
	assert.Equal(t, 43, mr.Read())
	assert.Equal(t, 300, ref.Read())

	e := sms.NewEncoder(sms.AsSubmit, sms.WithMR(mr), sms.WithConcatRef(ref))
	assert.Same(t, mr, e.MsgCount)
	assert.Same(t, ref, e.ConcatRef)
	out, err = e.Encode(longMsg)
	require.NoError(t, err)
	assert.Equal(t, byte(44), out[0].MR)
	assert.Equal(t, 301&0xff, concatRef(t, out))

	other := &sms.Counter{}
	out, err = e.Encode([]byte("hi"), sms.WithMR(other))
	require.NoError(t, err)
	assert.Equal(t, byte(1), out[0].MR)
	// the per call counter did not replace that of the Encoder
	assert.Equal(t, 45, mr.Read())
	assert.Same(t, mr, e.MsgCount)
}

func TestNewCounter(t *testing.T) {
	c := sms.NewCounter(254)
	assert.Equal(t, 254, c.Read())
	assert.Equal(t, 255, c.Count())
	assert.Equal(t, 256, c.Count())
	assert.Equal(t, 256, c.Read())
	var z sms.Counter
	assert.Equal(t, 1, z.Count())
}

// firstRefEnv makes TestHelperFirstConcatRef print the reference of the first
// concatenated message of the process.
const firstRefEnv = "GO_SMS_TEST_FIRST_CONCAT_REF"

func TestHelperFirstConcatRef(t *testing.T) {
	if os.Getenv(firstRefEnv) == "" {
		t.Skip("run by TestDefaultConcatRefIsRandom")
	}
	out, err := sms.Encode(longMsg, sms.WithMR(&sms.Counter{}))
	if err != nil {
		fmt.Println("error", err)
		os.Exit(1)
	}
	ci, _ := out[0].ConcatInfo()
	fmt.Printf("ref=%d\n", ci.Ref)
	os.Exit(0)
}

// The shared reference counter starts at a random value, so that the
// references of different runs of a program, such as successive runs of a
// command line tool, are unlikely to repeat.
func TestDefaultConcatRefIsRandom(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the test binary")
	}
	refs := map[int]bool{}
	for i := 0; i < 5; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperFirstConcatRef$")
		cmd.Env = append(os.Environ(), firstRefEnv+"=1")
		b, err := cmd.Output()
		require.NoError(t, err)
		s := strings.TrimSpace(string(b))
		require.True(t, strings.HasPrefix(s, "ref="), s)
		ref, err := strconv.Atoi(strings.TrimPrefix(s, "ref="))
		require.NoError(t, err)
		refs[ref] = true
	}
	// Five draws of 256 values are all equal with a chance of 1 in 2^32.
	assert.Greater(t, len(refs), 1, "%v", refs)
}
