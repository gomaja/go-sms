// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runMainEnv makes the test binary run main, so tests can run the tool as
// a process and check what it writes.
const runMainEnv = "GO_SMS_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// shownTable is a table as the tool writes it.
type shownTable struct {
	header string
	nli    int
	shift  bool
	cells  [128]string // trimmed, indexed by septet
}

// runTool runs the tool and parses the tables it writes.
func runTool(t *testing.T) []shownTable {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), runMainEnv+"=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run())
	require.Empty(t, stderr.String())

	headerRE := regexp.MustCompile(`^(.+) (Locking|Shift) \(NLI=(\d+)\)$`)
	lines := strings.Split(stdout.String(), "\n")
	var tables []shownTable
	for len(lines) > 1 {
		m := headerRE.FindStringSubmatch(lines[0])
		require.NotNil(t, m, "table header %q", lines[0])
		nli, err := strconv.Atoi(m[3])
		require.NoError(t, err)
		tab := shownTable{header: lines[0], nli: nli, shift: m[2] == "Shift"}
		require.Equal(t, "      0x0_ 0x1_ 0x2_ 0x3_ 0x4_ 0x5_ 0x6_ 0x7_ ", lines[1], tab.header)
		for row := 0; row < 16; row++ {
			line := lines[2+row]
			prefix := fmt.Sprintf("0x_%x: ", row)
			require.True(t, strings.HasPrefix(line, prefix), "%s row %q", tab.header, line)
			// each cell is 5 characters wide
			cells := []rune(strings.TrimPrefix(line, prefix))
			require.Len(t, cells, 8*5, "%s row %q", tab.header, line)
			for col := 0; col < 8; col++ {
				tab.cells[col*16+row] = strings.TrimSpace(string(cells[col*5 : col*5+5]))
			}
		}
		require.Empty(t, lines[18], tab.header)
		tables = append(tables, tab)
		lines = lines[19:]
	}
	require.Equal(t, []string{""}, lines)
	return tables
}

// specTables reads the tables of 3GPP TS 23.038 V20.0.0 from the testdata of
// the charset package, as the printed text of each cell, indexed by table
// name and septet.
func specTables(t *testing.T) map[string]*[128]string {
	data, err := os.ReadFile("../../encoding/gsm7/charset/testdata/ts23038-v20-tables.txt")
	require.NoError(t, err)
	tables := map[string]*[128]string{}
	var cur *[128]string
	row := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if fields[0] == "table" {
			cur = &[128]string{}
			tables[fields[1]] = cur
			row = 0
			continue
		}
		require.Len(t, fields, 8, "row %q", line)
		for col, cell := range fields {
			cur[col*16+row] = cell
		}
		row++
	}
	return tables
}

// specNames names the tables of the specification by national language
// identifier.
var specNames = map[int]string{
	charset.Default:    "Default",
	charset.Turkish:    "Turkish",
	charset.Spanish:    "Spanish",
	charset.Portuguese: "Portuguese",
	charset.Bengali:    "Bengali",
	charset.Gujaranti:  "Gujarati",
	charset.Hindi:      "Hindi",
	charset.Kannada:    "Kannada",
	charset.Malayalam:  "Malayalam",
	charset.Oriya:      "Oriya",
	charset.Punjabi:    "Punjabi",
	charset.Tamil:      "Tamil",
	charset.Telugu:     "Telugu",
	charset.Urdu:       "Urdu",
}

// specDeviations are the cells where the tables have the character the
// specification evidently means rather than what it prints, as listed and
// explained in the specDeviations of the charset package tests.
var specDeviations = map[string]map[byte]struct {
	printed string
	want    rune
}{
	"Kannada":    {0x24: {"0CAA", '\u0ca1'}},
	"Oriya":      {0x47: {"\u0b3c0B33", '\u0b33'}},
	"PunjabiExt": {0x23: {"OA6D", '\u0a6d'}},
	"TamilExt":   {0x24: {"0BEF", '\u0bee'}},
	"TeluguExt":  {0x22: {"06CC", '\u0c6c'}, 0x23: {"06CD", '\u0c6d'}},
}

// canonical returns a cell as the tool is expected to show it: the name of a
// control character or of the escape, or the Unicode code point of a
// character, or "" for a cell with no character.
func canonical(r rune) string {
	switch r {
	case ' ':
		return "SP"
	case '\n':
		return "LF"
	case '\r':
		return "CR"
	case '\f':
		return "FF"
	}
	return fmt.Sprintf("%U", r)
}

// specCell returns what the tool should show for a printed cell of the
// specification.
func specCell(t *testing.T, cell string) string {
	switch cell {
	case "·":
		return ""
	case "1)":
		// Note 1 at 0x1B: the escape to the extension table in the
		// locking shift tables (Section 6.2.1 and Annex A.3), and reserved
		// for the extension to another extension table in the single
		// shift tables (Section 6.2.1.1 and Annex A.2)
		return "ESC"
	case "3)":
		// Page Break (Note 3 of Section 6.2.1.1 and Annex A.2)
		return "FF"
	case "4)":
		// a control character with no symbol (Note 4 of Annex A.2)
		return ""
	case "SP", "LF", "CR":
		return cell
	}
	if utf8.RuneCountInString(cell) == 1 {
		r, _ := utf8.DecodeRuneInString(cell)
		return canonical(r)
	}
	v, err := strconv.ParseUint(cell, 16, 32)
	require.NoError(t, err, "cell %q", cell)
	require.Len(t, cell, 4, "cell %q", cell)
	return canonical(rune(v))
}

// shownCell returns a cell the tool wrote in the form of specCell.
func shownCell(t *testing.T, cell string) string {
	switch cell {
	case "", "ESC", "SP", "LF", "CR", "FF":
		return cell
	}
	// characters below U+0400, and €, are shown as themselves, and the
	// others as 4 hex digits
	if utf8.RuneCountInString(cell) == 1 {
		r, _ := utf8.DecodeRuneInString(cell)
		if r != '€' {
			require.Less(t, r, rune(0x400), "cell %q", cell)
		}
		return canonical(r)
	}
	v, err := strconv.ParseUint(cell, 16, 32)
	require.NoError(t, err, "cell %q", cell)
	require.Len(t, cell, 4, "cell %q", cell)
	require.GreaterOrEqual(t, v, uint64(0x400), "cell %q", cell)
	return canonical(rune(v))
}

// TestTablesMatchSpecification checks that the tool shows the locking and
// single shift tables of every national language identifier, and every cell
// of each as the tables of 3GPP TS 23.038 V20.0.0 print it.
func TestTablesMatchSpecification(t *testing.T) {
	spec := specTables(t)
	tables := runTool(t)
	require.Len(t, tables, 2*charset.End)
	for i, tab := range tables {
		t.Run(tab.header, func(t *testing.T) {
			assert.Equal(t, i/2, tab.nli)
			assert.Equal(t, i%2 == 1, tab.shift)
			name := specNames[tab.nli]
			if tab.shift {
				name += "Ext"
			} else if tab.nli == charset.Spanish {
				// there is no Spanish locking shift table (A.3.2 is Void),
				// so the default alphabet is used (Table 6.2.1.2.4.1)
				name = specNames[charset.Default]
			}
			cells := spec[name]
			require.NotNil(t, cells, "no table %s in the specification", name)
			for g := byte(0); g < 0x80; g++ {
				var want string
				if d, ok := specDeviations[name][g]; ok {
					assert.Equal(t, d.printed, cells[g], "printed cell 0x%02x", g)
					want = canonical(d.want)
				} else {
					want = specCell(t, cells[g])
				}
				assert.Equal(t, want, shownCell(t, tab.cells[g]), "cell 0x%02x", g)
			}
		})
	}
}

// TestEscape checks that every table shows the escape at 0x1B, and nowhere
// else, although no table has a character there.
func TestEscape(t *testing.T) {
	for _, tab := range runTool(t) {
		for g, cell := range tab.cells {
			if g == 0x1b {
				assert.Equal(t, "ESC", cell, "%s cell 0x%02x", tab.header, g)
			} else {
				assert.NotEqual(t, "ESC", cell, "%s cell 0x%02x", tab.header, g)
			}
		}
	}
}
