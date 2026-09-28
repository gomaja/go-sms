// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runMainEnv makes the test binary run main, so tests can run the tool as
// a process and check its exit status.
const runMainEnv = "GO_SMS_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
	}
	os.Exit(m.Run())
}

func TestNewCount(t *testing.T) {
	patterns := []struct {
		name string
		msg  string
		nli  int
		out  Count
		err  error
	}{
		{
			"std",
			"content of the SMS",
			0,
			Count{"7BIT", 1, 18, 18, 160, 142},
			nil,
		},
		{
			"empty",
			"",
			0,
			Count{"7BIT", 1, 0, 0, 160, 160},
			nil,
		},
		{
			"grin",
			"hello 😁",
			0,
			Count{"UCS-2", 1, 8, 8, 70, 62},
			nil,
		},
		{
			"urdu locking",
			"hi ت",
			13,
			Count{"7BIT", 1, 4, 4, 155, 151},
			nil,
		},
		{
			"urdu extended",
			"hi ؎",
			13,
			Count{"7BIT_EX", 1, 5, 5, 155, 150},
			nil,
		},
		{
			"urdu locking and extended",
			"hi ت؎",
			13,
			Count{"7BIT_EX", 1, 6, 6, 152, 146},
			nil,
		},
		{
			// The escape sequence of the euro sign is not split, so the
			// first segment holds 152 septets, not 153.
			"escape not split",
			strings.Repeat("a", 152) + "€" + strings.Repeat("a", 10),
			0,
			Count{"7BIT_EX", 2, 164, 12, 153, 141},
			nil,
		},
		{
			// The surrogate pair is not split, so the first segment holds
			// 66 code units, not 67.
			"surrogate pair not split",
			strings.Repeat("ж", 66) + "😁" + strings.Repeat("ж", 10),
			0,
			Count{"UCS-2", 2, 78, 12, 67, 55},
			nil,
		},
		{
			"three full segments",
			strings.Repeat("a", 3*153),
			0,
			Count{"7BIT", 3, 459, 153, 153, 0},
			nil,
		},
	}

	for _, p := range patterns {
		f := func(t *testing.T) {
			out, err := NewCount(p.msg, p.nli)
			assert.Equal(t, p.err, err)
			assert.Equal(t, p.out, out)
		}
		t.Run(p.name, f)
	}
}

func TestNewCountInvalidLanguage(t *testing.T) {
	for _, nli := range []int{-1, 14, 99} {
		_, err := NewCount("hi ş", nli)
		assert.True(t, errors.Is(err, errInvalidLanguage), "%d: %v", nli, err)
	}
	for nli := 0; nli <= 13; nli++ {
		_, err := NewCount("hi ş", nli)
		assert.NoError(t, err, "%d", nli)
	}
}

func TestRun(t *testing.T) {
	patterns := []struct {
		name   string
		args   []string
		status int
		stdout string
		stderr string
	}{
		{"std", []string{"-message", "content of the SMS"}, 0,
			"encoding: 7BIT\nmessages: 1\ntotal length: 18\nlast PDU length: 18\nper_message: 160\nremaining: 142\n", ""},
		{"language", []string{"-message", "hi ş", "-language", "1"}, 0,
			"encoding: 7BIT\nmessages: 1\ntotal length: 4\nlast PDU length: 4\nper_message: 155\nremaining: 151\n", ""},
		{"invalid utf8", []string{"-message", "hi\xff"}, 1, "", "invalid UTF8"},
		{"language 99", []string{"-message", "hi ş", "-language", "99"}, 2, "", "invalid language 99"},
		{"language -1", []string{"-message", "hi ş", "-language", "-1"}, 2, "", "invalid language -1"},
		{"no message", nil, 2, "", "Usage: smscounter"},
		{"extra argument", []string{"-message", "hi", "extra"}, 2, "", "Usage: smscounter"},
		{"unknown flag", []string{"-x"}, 2, "", "flag provided but not defined"},
	}
	for _, p := range patterns {
		f := func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			status := run(p.args, &stdout, &stderr)
			assert.Equal(t, p.status, status)
			assert.Equal(t, p.stdout, stdout.String())
			if p.stderr == "" {
				assert.Empty(t, stderr.String())
			} else {
				assert.Contains(t, stderr.String(), p.stderr)
			}
		}
		t.Run(p.name, f)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

// A failure to write the count is reported and fails the run.
func TestRunWriteError(t *testing.T) {
	var stderr bytes.Buffer
	status := run([]string{"-message", "hi"}, errWriter{}, &stderr)
	assert.Equal(t, 1, status)
	assert.Equal(t, "smscounter: write failed\n", stderr.String())
}

// The tool as a process: its exit status and output.
func TestMainProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the test binary")
	}
	patterns := []struct {
		args   []string
		status int
		stdout string
		stderr string
	}{
		{[]string{"-message", "hello"}, 0, "encoding: 7BIT\n", ""},
		{[]string{"-message", "hi\xff"}, 1, "", "smscounter: invalid UTF8"},
		{[]string{"-message", "hi", "-language", "14"}, 2, "", "invalid language 14"},
		{nil, 2, "", "Usage: smscounter"},
	}
	for _, p := range patterns {
		cmd := exec.Command(os.Args[0], p.args...)
		cmd.Env = append(os.Environ(), runMainEnv+"=1")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		status := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			status = ee.ExitCode()
		} else {
			require.NoError(t, err)
		}
		assert.Equal(t, p.status, status, "%v", p.args)
		assert.True(t, strings.HasPrefix(stdout.String(), p.stdout), "%v: %q", p.args, stdout.String())
		assert.Contains(t, stderr.String(), p.stderr, "%v", p.args)
	}
}
