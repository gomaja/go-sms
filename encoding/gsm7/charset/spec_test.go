// SPDX-License-Identifier: MIT

package charset_test

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// specTables reads the tables of 3GPP TS 23.038 V20.0.0 from testdata, as the
// printed text of each cell, indexed by table name and septet.
func specTables(t *testing.T) map[string]*[128]string {
	data, err := os.ReadFile("testdata/ts23038-v20-tables.txt")
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
			require.Nil(t, tables[fields[1]], "table %s repeated", fields[1])
			require.True(t, cur == nil || row == 16, "table before %s is short", fields[1])
			cur = &[128]string{}
			tables[fields[1]] = cur
			row = 0
			continue
		}
		require.Len(t, fields, 8, "row %q", line)
		require.Less(t, row, 16, "row %q", line)
		for col, cell := range fields {
			cur[col*16+row] = cell
		}
		row++
	}
	require.Equal(t, 16, row)
	return tables
}

// specCell returns the character a cell of the specification defines, if
// any.
func specCell(cell string) (r rune, ok bool, known bool) {
	switch cell {
	case "·":
		return 0, false, true
	case "1)":
		// the escape to the extension table in the locking shift tables,
		// and reserved for another extension table in the extension tables
		return 0, false, true
	case "3)":
		// Page Break (Note 3 of 6.2.1.1 and Annex A.2)
		return '\f', true, true
	case "4)":
		// a control character with no symbol (Note 4 of Annex A.2)
		return 0, false, true
	case "SP":
		return ' ', true, true
	case "LF":
		return '\n', true, true
	case "CR":
		return '\r', true, true
	}
	if utf8.RuneCountInString(cell) == 1 {
		r, _ := utf8.DecodeRuneInString(cell)
		return r, true, true
	}
	if len(cell) == 4 {
		if v, err := strconv.ParseUint(cell, 16, 32); err == nil {
			return rune(v), true, true
		}
	}
	return 0, false, false
}

// specDeviation is a cell where the tables differ from the printed text of
// the specification, which is evidently in error there. Each is commented at
// its cell in the table.
type specDeviation struct {
	printed string
	want    rune
}

var specDeviations = map[string]map[byte]specDeviation{
	// A.3.7 prints 0CAA, which it also prints at 0x3D, and has no 0CA1
	"Kannada": {0x24: {"0CAA", '\u0ca1'}},
	// A.3.9 prints a stray U+0B3C before 0B33
	"Oriya": {0x47: {"\u0b3c0B33", '\u0b33'}},
	// A.2.10 prints the code point with a letter O
	"PunjabiExt": {0x23: {"OA6D", '\u0a6d'}},
	// A.2.11 prints 0BEF, which it also prints at 0x25, and has no 0BEE
	"TamilExt": {0x24: {"0BEF", '\u0bee'}},
	// A.2.12 prints Arabic code points among the Telugu digits
	"TeluguExt": {0x22: {"06CC", '\u0c6c'}, 0x23: {"06CD", '\u0c6d'}},
}

// TestTablesMatchSpecification checks every cell of every table against the
// tables of 3GPP TS 23.038 V20.0.0, and that each encoder is the inverse of
// its decoder.
func TestTablesMatchSpecification(t *testing.T) {
	spec := specTables(t)
	type table struct {
		dec charset.Decoder
		enc charset.Encoder
	}
	tables := map[string]table{
		"Default":    {charset.DefaultDecoder(), charset.DefaultEncoder()},
		"DefaultExt": {charset.DefaultExtDecoder(), charset.DefaultExtEncoder()},
	}
	for nli := charset.Start; nli < charset.End; nli++ {
		name := charsetName[nli]
		tables[name+"Ext"] = table{charset.NewExtDecoder(nli), charset.NewExtEncoder(nli)}
		if nli == charset.Spanish {
			// there is no Spanish locking shift table (A.3.2 is Void), so
			// the default alphabet is used (Table 6.2.1.2.4.1)
			assert.Equal(t, charset.DefaultDecoder(), charset.NewDecoder(nli))
			assert.Equal(t, charset.DefaultEncoder(), charset.NewEncoder(nli))
			continue
		}
		tables[name] = table{charset.NewDecoder(nli), charset.NewEncoder(nli)}
	}
	require.Len(t, spec, len(tables))

	for name, tab := range tables {
		t.Run(name, func(t *testing.T) {
			cells, ok := spec[name]
			require.True(t, ok, "no table %s in the specification", name)
			want := charset.Decoder{}
			for g, cell := range cells {
				r, ok, known := specCell(cell)
				if d, dev := specDeviations[name][byte(g)]; dev {
					assert.Equal(t, d.printed, cell, "printed cell 0x%02x", g)
					r, ok, known = d.want, true, true
				}
				require.True(t, known, "cell 0x%02x %q", g, cell)
				if ok {
					want[byte(g)] = r
				}
			}
			assert.Equal(t, want, tab.dec)

			inverse := charset.Encoder{}
			for g, r := range tab.dec {
				if e, ok := inverse[r]; !ok || g < e {
					inverse[r] = g
				}
			}
			assert.Equal(t, inverse, tab.enc)
		})
	}
}
