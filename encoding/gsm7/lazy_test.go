// SPDX-License-Identifier: MIT

package gsm7

import (
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gomaja/go-sms/encoding/gsm7/charset"
)

// TestImportAllocations checks that initialising this package and the charset
// package builds no character set table. It runs the test binary again with
// GODEBUG=inittrace=1, which prints the heap allocated by the initialisation
// of each package that has initialisation work.
func TestImportAllocations(t *testing.T) {
	switch runtime.GOOS {
	case "js", "wasip1", "ios", "android":
		t.Skip("test binaries cannot run themselves on this platform")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "GODEBUG="+os.Getenv("GODEBUG")+",inittrace=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run test binary: %v\n%s", err, out)
	}
	if !regexp.MustCompile(`(?m)^init \S+ @`).Match(out) {
		t.Fatalf("no inittrace output:\n%s", out)
	}
	// Initialising this package allocates only the empty noExt map, and
	// coverage registration adds up to 128 bytes and one allocation to an
	// instrumented package. Building even the smallest table, the default
	// extension table, allocates more.
	extraBytes, extraAllocs := 0, 0
	if testing.CoverMode() != "" {
		extraBytes, extraAllocs = 128, 1
	}
	for _, pkg := range []struct {
		path                string
		maxBytes, maxAllocs int
	}{
		{reflect.TypeOf(charset.Decoder{}).PkgPath(), 0, 0},
		{reflect.TypeOf(Decoder{}).PkgPath(), 64, 1},
	} {
		pkg.maxBytes += extraBytes
		pkg.maxAllocs += extraAllocs
		re := regexp.MustCompile(`(?m)^init ` + regexp.QuoteMeta(pkg.path) + ` @.* ms clock, ([0-9]+) bytes, ([0-9]+) allocs$`)
		line := re.FindSubmatch(out)
		if line == nil {
			// a package without initialisation work has no line
			continue
		}
		t.Logf("%s", line[0])
		bytes, _ := strconv.Atoi(string(line[1]))
		allocs, _ := strconv.Atoi(string(line[2]))
		if bytes > pkg.maxBytes || allocs > pkg.maxAllocs {
			t.Errorf("initialisation allocates more than %d bytes or %d allocations:\n%s", pkg.maxBytes, pkg.maxAllocs, line[0])
		}
	}
}

func TestLazyTablesConcurrent(t *testing.T) {
	var calls [charset.End]atomic.Int32
	l := lazyTables[charset.Decoder]{build: func(nli int) charset.Decoder {
		calls[nli].Add(1)
		runtime.Gosched()
		return charset.Decoder{0: rune(nli)}
	}}
	const workers = 64
	ids := []int{-1000, -1}
	for i := 0; i <= charset.End; i++ {
		ids = append(ids, i)
	}
	ids = append(ids, 1000)
	results := make([][]charset.Decoder, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for w := range results {
		results[w] = make([]charset.Decoder, len(ids))
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			<-start
			for j, id := range ids {
				results[w][j] = l.get(id)
			}
		}(w)
	}
	close(start)
	wg.Wait()
	for id := range calls {
		if n := calls[id].Load(); n != 1 {
			t.Errorf("build(%d) called %d times, want 1", id, n)
		}
	}
	for w := range results {
		for j, id := range ids {
			want := id
			if want < 0 || want >= charset.End {
				want = charset.Default
			}
			m := results[w][j]
			if len(m) != 1 || m[0] != rune(want) {
				t.Errorf("get(%d) = %v, want identifier %d", id, m, want)
			}
			if reflect.ValueOf(m).UnsafePointer() != reflect.ValueOf(l.get(want)).UnsafePointer() {
				t.Errorf("worker %d get(%d) did not share its table", w, id)
			}
		}
	}
}

// TestLazyTablesGetAllocations checks that getting a table that has been
// built allocates nothing.
func TestLazyTablesGetAllocations(t *testing.T) {
	for nli := charset.Default; nli < charset.End; nli++ {
		for name, get := range map[string]func(){
			"decoders":    func() { _ = decoders.get(nli) },
			"extDecoders": func() { _ = extDecoders.get(nli) },
			"encoders":    func() { _ = encoders.get(nli) },
			"extEncoders": func() { _ = extEncoders.get(nli) },
		} {
			if n := testing.AllocsPerRun(100, get); n != 0 {
				t.Errorf("%s.get(%d) allocates %v times", name, nli, n)
			}
		}
	}
}
