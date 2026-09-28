// SPDX-License-Identifier: MIT

// Command readme holds the Go examples of the README, each a verbatim copy
// of a Go block of the README, so that they are compiled. A test checks that
// every Go block of the README is found here.
//
//lint:file-ignore U1000 README examples are compiled to keep snippets valid.
package main

import (
	"log"
	"time"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/gomaja/go-sms/encoding/tpdu"
)

func main() {
	log.SetFlags(0)
}

func encode(msg []byte) error {
	tpdus, err := sms.Encode(msg, sms.To("+15551234567"))
	if err != nil {
		return err
	}
	for _, p := range tpdus {
		b, err := p.MarshalBinary()
		if err != nil {
			return err
		}
		sendPDU(b) // send the binary TPDU...
	}
	return nil
}

func encoder(msgs <-chan []byte) error {
	e := sms.NewEncoder(sms.AsSubmit, sms.To("+15551234567"))
	for msg := range msgs {
		tpdus, err := e.Encode(msg)
		if err != nil {
			return err
		}
		for _, p := range tpdus {
			b, err := p.MarshalBinary()
			if err != nil {
				return err
			}
			sendPDU(b) // send the binary TPDU...
		}
	}
	return nil
}

func unmarshal(bintpdu []byte) (*tpdu.TPDU, error) {
	return sms.Unmarshal(bintpdu)
}

func decodeOne(pdu *tpdu.TPDU) ([]byte, error) {
	return sms.Decode([]*tpdu.TPDU{pdu})
}

func decodeMany(tpdus []*tpdu.TPDU) ([]byte, error) {
	return sms.Decode(tpdus)
}

func collect(bintpdus <-chan []byte) {
	c := sms.NewCollector()
	defer c.Close()
	for bintpdu := range bintpdus {
		pdu, err := sms.Unmarshal(bintpdu)
		if err != nil {
			log.Print(err)
			continue
		}
		tpdus, err := c.Collect(pdu)
		if err != nil {
			log.Print(err)
			continue
		}
		if tpdus == nil {
			continue // wait for the other segments
		}
		msg, err := sms.Decode(tpdus)
		if err != nil {
			log.Print(err)
			continue
		}
		handleMsg(tpdus[0].OA.Number(), msg)
	}
}

func collectSubmit(c *sms.Collector, originator string, bintpdu []byte) ([]*tpdu.TPDU, error) {
	pdu, err := sms.Unmarshal(bintpdu, sms.AsMO)
	if err != nil {
		return nil, err
	}
	return c.Collect(pdu, sms.WithOriginator(originator))
}

func collector() *sms.Collector {
	return sms.NewCollector(
		sms.WithReassemblyTimeout(time.Hour),
		sms.WithReassemblyLimit(64*1024),
		sms.WithExpiryHandler(func(segments []*tpdu.TPDU, reason error) {
			log.Printf("abandoned a message of %d segments: %v", len(segments), reason)
		}),
	)
}

func to() ([]tpdu.TPDU, error) {
	return sms.Encode([]byte("hello"), sms.To("12345"))
}

func urdu() ([]tpdu.TPDU, error) {
	return sms.Encode([]byte("hello ٻ"), sms.WithCharset(charset.Urdu))
}

func deliver() ([]tpdu.TPDU, error) {
	return sms.Encode([]byte("hello"), sms.AsDeliver, sms.From("12345"))
}

func counters(lastUsedTPMR int) *sms.Encoder {
	return sms.NewEncoder(
		sms.AsSubmit,
		sms.WithMR(sms.NewCounter(lastUsedTPMR)),
		sms.With16BitConcatRef,
	)
}

func unmarshalMO(bintpdu []byte) (*tpdu.TPDU, error) {
	return sms.Unmarshal(bintpdu, sms.AsMO)
}

func handleMsg(string, []byte) {
}

func sendPDU([]byte) {
}
