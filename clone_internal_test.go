// SPDX-License-Identifier: MIT

package sms

import (
	"reflect"
	"testing"
	"time"

	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/stretchr/testify/assert"
)

// checkNoSharedMemory fails if a and b, of the same type, share any memory,
// and if they hold a reference cloneTPDU does not know how to copy, so that a
// field added to tpdu.TPDU cannot go uncloned.
func checkNoSharedMemory(t *testing.T, path string, a, b reflect.Value) {
	t.Helper()
	switch a.Kind() {
	case reflect.Slice:
		if a.IsNil() != b.IsNil() {
			t.Errorf("%s: nil %v cloned as nil %v", path, a.IsNil(), b.IsNil())
			return
		}
		if a.Len() > 0 && a.Pointer() == b.Pointer() {
			t.Errorf("%s: backing array shared", path)
		}
		for i := 0; i < a.Len() && i < b.Len(); i++ {
			checkNoSharedMemory(t, path+"[]", a.Index(i), b.Index(i))
		}
	case reflect.Struct:
		if a.Type() == reflect.TypeOf(time.Time{}) {
			// Its location is shared by design and never changed.
			return
		}
		for i := 0; i < a.NumField(); i++ {
			checkNoSharedMemory(t, path+"."+a.Type().Field(i).Name, a.Field(i), b.Field(i))
		}
	case reflect.Array:
		for i := 0; i < a.Len(); i++ {
			checkNoSharedMemory(t, path+"[]", a.Index(i), b.Index(i))
		}
	case reflect.Map, reflect.Pointer, reflect.Interface, reflect.Chan,
		reflect.Func, reflect.UnsafePointer:
		t.Errorf("%s: %s is not handled by cloneTPDU", path, a.Kind())
	}
}

func TestCloneTPDU(t *testing.T) {
	full := tpdu.TPDU{
		Direction:  tpdu.MO,
		FirstOctet: 0x41,
		OA:         tpdu.Address{TOA: 0x91, Addr: "1234"},
		DA:         tpdu.Address{TOA: 0x91, Addr: "5678"},
		PIExt:      []byte{1, 2},
		SCTS:       tpdu.Timestamp{Time: time.Date(2026, 9, 28, 1, 2, 3, 0, time.UTC)},
		UDH: tpdu.UserDataHeader{
			{ID: 0, Data: []byte{1, 2, 1}},
			{ID: 5, Data: []byte{}},
			{ID: 6},
		},
		UD: []byte("hello"),
	}
	for _, in := range []tpdu.TPDU{
		full,
		{},
		{UDH: tpdu.UserDataHeader{}, UD: tpdu.UserData{}, PIExt: []byte{}},
	} {
		out := cloneTPDU(&in)
		assert.Equal(t, in, out)
		checkNoSharedMemory(t, "TPDU", reflect.ValueOf(in), reflect.ValueOf(out))
	}
}
