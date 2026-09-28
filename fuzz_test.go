// SPDX-License-Identifier: MIT

package sms_test

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/tpdu"
	"github.com/gomaja/go-sms/encoding/ucs2"
)

// fuzzMaxMsg keeps messages within the 255 segments of a concatenated
// message, at 67 UCS2 characters each.
const fuzzMaxMsg = 4000

// FuzzEncodeCollectDecode encodes a message, marshals and unmarshals its
// TPDUs, collects them shuffled and with duplicates, and requires the
// collected message to decode back to the message.
func FuzzEncodeCollectDecode(f *testing.F) {
	f.Add([]byte("hello"), uint8(0), uint8(0), uint64(1), uint8(0))
	f.Add([]byte(strings.Repeat("a", 200)), uint8(1), uint8(0), uint64(2), uint8(0xff))
	f.Add([]byte(strings.Repeat("ت€", 120)), uint8(0), uint8(13), uint64(3), uint8(0x05))
	f.Add([]byte(strings.Repeat("ğış", 70)), uint8(1), uint8(14), uint64(4), uint8(0x02))
	f.Add([]byte(strings.Repeat("😁ж", 90)), uint8(2), uint8(0), uint64(5), uint8(0x07))
	f.Add([]byte("\x00\xff\x1b\x80binary"), uint8(3), uint8(0), uint64(6), uint8(0x01))
	f.Add([]byte("hi\xff"), uint8(0), uint8(0), uint64(7), uint8(0))
	f.Add([]byte(""), uint8(1), uint8(0), uint64(8), uint8(0))
	f.Fuzz(func(t *testing.T, msg []byte, kind, nli uint8, seed uint64, flags uint8) {
		if len(msg) > fuzzMaxMsg {
			msg = msg[:fuzzMaxMsg]
		}
		deliver := kind&1 == 1
		options := []sms.EncoderOption{
			sms.WithMR(&sms.Counter{}),
			sms.WithConcatRef(sms.NewCounter(int(seed % 70000))),
		}
		if deliver {
			options = append(options, sms.AsDeliver, sms.From("+15550001"))
		} else {
			options = append(options, sms.To("+15550002"))
		}
		if flags&0x01 != 0 {
			options = append(options, sms.With16BitConcatRef)
		}
		var udh tpdu.UserDataHeader
		if flags&0x02 != 0 {
			udh = append(udh, tpdu.InformationElement{ID: 0x05, Data: []byte{0x0b, 0x84, 0x23, 0xf0}})
		}
		if flags&0x08 != 0 && kind%4 < 2 {
			// a national language IE the Encoder must replace
			udh = append(udh, tpdu.InformationElement{
				ID: tpdu.IEINationalLanguageLockingShift, Data: []byte{byte(1 + int(nli)%13)},
			})
		}
		if udh != nil {
			options = append(options, sms.WithTemplateOption(tpdu.WithUDH(udh)))
		}
		var want []byte
		switch kind % 4 {
		case 0, 1: // coded by the Encoder
			switch n := int(nli % 15); {
			case n == 14:
				options = append(options, sms.WithAllCharsets)
			case n != 0:
				options = append(options, sms.WithCharset(n))
			}
			want = msg
		case 2: // UCS2
			s := strings.ToValidUTF8(string(msg), "�")
			want = []byte(s)
			msg = ucs2.Encode([]rune(s))
			options = append(options, sms.AsUCS2)
		case 3: // 8 bit
			want = msg
			options = append(options, sms.As8Bit)
		}
		pdus, err := sms.Encode(msg, options...)
		if kind%4 < 2 && !utf8.Valid(msg) {
			if !errors.Is(err, tpdu.ErrInvalidUTF8) {
				t.Fatalf("invalid UTF-8 encoded: %v", err)
			}
			return
		}
		if errors.Is(err, tpdu.ErrTooManySegments) {
			return
		}
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		// marshal and unmarshal, as a receiver sees them
		received := make([]*tpdu.TPDU, len(pdus))
		for i := range pdus {
			b, err := pdus[i].MarshalBinary()
			if err != nil {
				t.Fatalf("marshal segment %d: %v", i+1, err)
			}
			var uopts []sms.UnmarshalOption
			if !deliver {
				uopts = append(uopts, sms.AsMO)
			}
			if received[i], err = sms.Unmarshal(b, uopts...); err != nil {
				t.Fatalf("unmarshal segment %d: %v", i+1, err)
			}
		}
		// shuffle, and duplicate some of those before the last
		rng := rand.New(rand.NewPCG(seed, uint64(flags)))
		order := rng.Perm(len(received))
		seq := make([]int, 0, 2*len(order))
		for i, o := range order {
			seq = append(seq, o)
			if len(order) > 1 && i < len(order)-1 && rng.IntN(4) == 0 {
				seq = append(seq, order[rng.IntN(i+1)])
			}
		}
		c := sms.NewCollector()
		defer c.Close()
		var cfg []sms.CollectOption
		if !deliver {
			cfg = append(cfg, sms.WithOriginator("+15550001"))
		}
		var got []*tpdu.TPDU
		seen := map[int]bool{}
		for i, o := range seq {
			out, err := c.Collect(received[o], cfg...)
			switch {
			case seen[o] && len(received) > 1:
				if err != sms.ErrDuplicateSegment {
					t.Fatalf("duplicate of segment %d: %v", o+1, err)
				}
				continue
			case err != nil:
				t.Fatalf("collect segment %d: %v", o+1, err)
			}
			seen[o] = true
			last := i == len(seq)-1
			if len(received) == 1 {
				last = true
			}
			if last != (out != nil) {
				t.Fatalf("segment %d of %d, collected %d, returned %d segments", o+1, len(received), i+1, len(out))
			}
			if out != nil {
				got = out
			}
		}
		if !sms.IsCompleteMessage(got) {
			t.Fatalf("incomplete message %v", got)
		}
		if n := len(c.Pipes()); n != 0 {
			t.Fatalf("%d pipes left", n)
		}
		dec, err := sms.Decode(got)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if string(dec) != string(want) {
			t.Fatalf("decoded %q, want %q", dec, want)
		}
	})
}

// FuzzCollect collects a stream of TPDUs described by the input, of any
// type, originator and concatenation IE, valid or not, and requires that the
// Collector never panics, never holds more segments than its limit, returns
// only complete messages, settles every segment it accepts exactly once, by
// Collect or the expiry handler, leaves no pipe once they expire, and runs
// no handler once Close has returned.
func FuzzCollect(f *testing.F) {
	f.Add([]byte{0x00, 0x01, 0x02, 0x02, 0x10, 0x11, 0x12, 0x13}, uint8(4))
	f.Add([]byte{0x40, 0x41, 0x42, 0x80, 0x81, 0xc0, 0xc1, 0xc2, 0xc3, 0x00}, uint8(0x83))
	f.Add([]byte("some arbitrary bytes that make a stream of segments"), uint8(0x42))
	f.Fuzz(func(t *testing.T, stream []byte, cfg uint8) {
		if len(stream) > 600 {
			stream = stream[:600]
		}
		limit := int(cfg&0x0f) + 1
		timeout := time.Duration(0)
		if cfg&0x80 != 0 {
			timeout = time.Millisecond
		}
		var mu sync.Mutex
		var closed bool
		settled := map[string]int{}
		settle := func(segs []*tpdu.TPDU) {
			for _, s := range segs {
				if s != nil {
					settled[string(s.UD)]++
				}
			}
		}
		c := sms.NewCollector(
			sms.WithReassemblyLimit(limit),
			sms.WithReassemblyTimeout(timeout),
			sms.WithExpiryHandler(func(segs []*tpdu.TPDU, reason error) {
				mu.Lock()
				defer mu.Unlock()
				if closed {
					t.Errorf("handler ran after Close")
				}
				if reason != sms.ErrReassemblyLimit && reason != sms.ErrReassemblyTimeout && reason != sms.ErrClosed {
					t.Errorf("reason %v", reason)
				}
				if sms.IsCompleteMessage(segs) {
					t.Errorf("abandoned a complete message")
				}
				settle(segs)
			}))
		accepted := map[string]int{}
		// the key of the reassembly each segment accepted belongs to
		keys := map[string]string{}
		for i, b := range stream {
			p := &tpdu.TPDU{UD: []byte(fmt.Sprint(i))}
			// bits 7-6: type, 5-4: originator, 3-2: total, 1-0: seqno
			switch b >> 6 {
			case 0, 1:
				p.OA = tpdu.Address{TOA: 0x91, Addr: fmt.Sprint(100 + (b>>4)&3)}
			case 2:
				_ = p.SetSmsType(tpdu.SmsSubmit)
				p.DA = tpdu.Address{TOA: 0x91, Addr: "200"}
			case 3:
				_ = p.SetSmsType(otherTypes[i%len(otherTypes)])
			}
			total, seqno := b>>2&3, b&3
			switch {
			case i%7 == 3:
				p.SetUDH(tpdu.UserDataHeader{{ID: tpdu.IEIConcat16Bit, Data: []byte{0, 1, total, seqno}}})
			case i%11 != 5:
				p.SetUDH(tpdu.UserDataHeader{{ID: tpdu.IEIConcat8Bit, Data: []byte{1, total, seqno}}})
			}
			var opts []sms.CollectOption
			if b>>6 == 2 && b&0x10 != 0 {
				opts = append(opts, sms.WithOriginator(fmt.Sprint(b>>5&1)))
			}
			out, err := c.Collect(p, opts...)
			if b>>6 == 3 && err != tpdu.ErrUnsupportedSmsType(p.SmsType()) {
				t.Fatalf("%s collected: %v", p.SmsType(), err)
			}
			if err == nil {
				mu.Lock()
				accepted[string(p.UD)]++
				mu.Unlock()
				keys[string(p.UD)] = fmt.Sprintf("%s %v %v %d", p.SmsType(), p.OA, p.DA, len(opts))
				if len(opts) > 0 {
					keys[string(p.UD)] += fmt.Sprint(b >> 5 & 1)
				}
			}
			if out != nil {
				if !sms.IsCompleteMessage(out) {
					t.Fatalf("incomplete message returned: %v", out)
				}
				for _, s := range out {
					if keys[string(s.UD)] != keys[string(out[0].UD)] {
						t.Fatalf("message of segments from %q and %q", keys[string(out[0].UD)], keys[string(s.UD)])
					}
				}
				mu.Lock()
				settle(out)
				mu.Unlock()
			}
			if i%5 == 0 {
				n := 0
				for _, pp := range c.Pipes() {
					for _, s := range pp.Segments {
						if s != nil {
							n++
						}
					}
				}
				if n > limit {
					t.Fatalf("%d segments held, limit %d", n, limit)
				}
			}
		}
		if timeout > 0 && cfg&0x40 != 0 {
			deadline := time.Now().Add(time.Second)
			for len(c.Pipes()) > 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if n := len(c.Pipes()); n > 0 {
				t.Fatalf("%d pipes left after they expired", n)
			}
		}
		c.Close()
		mu.Lock()
		closed = true
		defer mu.Unlock()
		if n := len(c.Pipes()); n > 0 {
			t.Fatalf("%d pipes left after Close", n)
		}
		for k, v := range accepted {
			if settled[k] != v {
				t.Fatalf("segment %s accepted %d times, settled %d", k, v, settled[k])
			}
		}
		if len(settled) != len(accepted) {
			t.Fatalf("settled %d segments, accepted %d", len(settled), len(accepted))
		}
	})
}

// FuzzDecodeUCS2Split decodes a UTF-16 stream split into UCS2 segments at
// any even offsets, and requires the message to be the stream decoded
// whole, with every unpaired surrogate replaced by U+FFFD, as
// unicode/utf16 decodes it: a surrogate pair split between segments is
// still one character, and a high surrogate that ends the message is
// U+FFFD.
func FuzzDecodeUCS2Split(f *testing.F) {
	f.Add([]byte{0xd8, 0x3d, 0xde, 0x01, 0x00, 0x41}, []byte{2})
	f.Add([]byte{0x00, 0x41, 0xd8, 0x3d}, []byte{1})
	f.Add([]byte{0x00, 0x41, 0xde, 0x01, 0x00, 0x42, 0x00, 0x43}, []byte{2, 0})
	f.Add([]byte{0xd8, 0x3d, 0xd8, 0x3d, 0xde, 0x01}, []byte{1, 1})
	f.Fuzz(func(t *testing.T, stream, cuts []byte) {
		stream = stream[:len(stream)&^1]
		units := make([]uint16, len(stream)/2)
		for i := range units {
			units[i] = uint16(stream[2*i])<<8 | uint16(stream[2*i+1])
		}
		want := string(utf16.Decode(units))
		var segs []*tpdu.TPDU
		rest := stream
		for _, c := range cuts {
			n := 2 * int(c)
			if n > len(rest) {
				n = len(rest)
			}
			segs = append(segs, &tpdu.TPDU{DCS: tpdu.DcsUCS2Data, UD: rest[:n]})
			rest = rest[n:]
		}
		segs = append(segs, &tpdu.TPDU{DCS: tpdu.DcsUCS2Data, UD: rest})
		msg, err := sms.Decode(segs)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !utf8.Valid(msg) {
			t.Fatalf("invalid UTF-8 % x", msg)
		}
		if string(msg) != want {
			t.Fatalf("decoded %q, want %q", msg, want)
		}
	})
}

// otherTypes are the TPDU types the Collector does not collect.
var otherTypes = []tpdu.SmsType{
	tpdu.SmsDeliverReport, tpdu.SmsSubmitReport, tpdu.SmsStatusReport, tpdu.SmsCommand,
}
