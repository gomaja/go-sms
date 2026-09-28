# go-sms

A Go library for encoding and decoding SMS TPDUs.

[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/gomaja/go-sms)
[![Go Report Card](https://goreportcard.com/badge/github.com/gomaja/go-sms)](https://goreportcard.com/report/github.com/gomaja/go-sms)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://github.com/gomaja/go-sms/blob/main/LICENSE)

go-sms is a Go library for encoding and decoding SMS TPDUs, as specified in 3GPP TS 23.040 and 3GPP TS 23.038.

The initial impetus was to provide functionality to send and receive SMSs via a
GSM modem, but the library may be generally useful anywhere encoding and
decoding SMS TPDUs or their fields is required.

go-sms requires Go 1.23 or later.

## Installation

go-sms publishes no tagged releases, so depend on its main branch:

```shell
go get github.com/gomaja/go-sms@main
```

## Standards scope

The core TPDU and field encoders track 3GPP TS 23.040 V19.0.0 and
3GPP TS 23.038 V20.0.0. PDU mode framing for modem exchange tracks
3GPP TS 27.005 V19.0.0. International number type handling follows the
address format in 3GPP TS 23.040, with ISDN/E.164 numbering-plan values
aligned to ITU-T E.164 (02/2026).

The package does not implement the RP/CP transport procedures in
3GPP TS 24.011, SMS over IP in 3GPP TS 24.341, or the `sms:` URI scheme in
RFC 5724.

Compressed TP-UD content signalled by the TP-DCS compression bit is rejected by
the high-level message decoder until 3GPP TS 23.042 decompression is
implemented.

## Features

Supports the following functionality:

- Creation of SMS TPDUs from UTF-8 strings, including emoji's 😁
- Segmentation of long messages into several concatenated SMS TPDUs
- Automatic selection of alphabet and language when encoding
- Decoding of SMS TPDUs into UTF-8 strings
- Reassembly of concatenated SMS TPDUs into a long message
- Support for all GSM character sets
- Encoding and decoding SMS TPDUs in PDU mode for exchange with GSM modems

## Usage

The examples below are compiled as part of the tests, in
[internal/readme](internal/readme/readme.go), so they match the API. They use
these imports:

```go
import (
	"log"
	"time"

	"github.com/gomaja/go-sms"
	"github.com/gomaja/go-sms/encoding/gsm7/charset"
	"github.com/gomaja/go-sms/encoding/tpdu"
)
```

### Encode

Creating the TPDUs to contain a message is referred to as encoding.

A one-off message can be encoded using *sms.Encode*, which creates
SMS-SUBMIT TPDUs:

```go
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
```

Sending multiple messages is performed by an *sms.Encoder*, which holds the
options, such as the destination, for every message. An Encoder may be used by
several goroutines at once:

```go
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
```

Each TPDU carries a TP-MR, and each concatenated message a reference number
shared by its segments. Unless given counters with *WithMR* and
*WithConcatRef*, Encoders, and *sms.Encode*, draw both from counters shared by
all of them, so consecutive messages do not reuse a reference. The shared
reference counter starts at a random value, so that successive runs of a
program are unlikely to reuse one either. An MS continues the TP-MR of its
(U)SIM, which *NewCounter* provides:

```go
func counters(lastUsedTPMR int) *sms.Encoder {
	return sms.NewEncoder(
		sms.AsSubmit,
		sms.WithMR(sms.NewCounter(lastUsedTPMR)),
		sms.With16BitConcatRef,
	)
}
```

### Unmarshal

Reassembling received TPDUs into a complete message is a multi-step process.
The first step is to unmarshal the binary SMS TPDU into a TPDU object using
*sms.Unmarshal*:

```go
func unmarshal(bintpdu []byte) (*tpdu.TPDU, error) {
	return sms.Unmarshal(bintpdu)
}
```

### Decode

A single segment TPDU can be decoded using *sms.Decode*:

```go
func decodeOne(pdu *tpdu.TPDU) ([]byte, error) {
	return sms.Decode([]*tpdu.TPDU{pdu})
}
```

For concatenated messages, the set of TPDUs containing a message is reassembled
into a complete message using *sms.Decode*:

```go
func decodeMany(tpdus []*tpdu.TPDU) ([]byte, error) {
	return sms.Decode(tpdus)
}
```

### Collect

The segments of concatenated messages must be collected before they can be
decoded. The Collector collects received segments and returns the complete set
once the final segment is received. It keeps apart the segments of messages
from different originators, as 3GPP TS 23.040 Section 9.2.3.24.1 requires,
which, for an SMS-DELIVER, is its TP-OA:

```go
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
```

The originator of an SMS-SUBMIT is not in the TPDU, but given by the layer
that carried it, such as the SM-RP-OA of MAP, so it must be given to Collect:

```go
func collectSubmit(c *sms.Collector, originator string, bintpdu []byte) ([]*tpdu.TPDU, error) {
	pdu, err := sms.Unmarshal(bintpdu, sms.AsMO)
	if err != nil {
		return nil, err
	}
	return c.Collect(pdu, sms.WithOriginator(originator))
}
```

A Collector abandons a reassembly that is not complete within its reassembly
timeout, 24 hours by default, from its first segment, and holds at most 4096
segments by default, abandoning the oldest reassemblies to make room, so its
memory is bounded. Closing it abandons those it holds. The segments of an
abandoned reassembly are passed to the expiry handler, if any, with nil for
each one missing:

```go
func collector() *sms.Collector {
	return sms.NewCollector(
		sms.WithReassemblyTimeout(time.Hour),
		sms.WithReassemblyLimit(64*1024),
		sms.WithExpiryHandler(func(segments []*tpdu.TPDU, reason error) {
			log.Printf("abandoned a message of %d segments: %v", len(segments), reason)
		}),
	)
}
```

### Options

The core API is aimed at the most common use cases, those performed to the
mobile station. e.g. By default, *sms.Encode* creates an SMS-SUBMIT TPDU and
only uses the default character set.  By default, *sms.Decode* uses all
character sets.  By default, *sms.Unmarshal* assumes the TPDU is mobile
terminating.

The behaviour of the core API functions can be altered for other use cases
using optional parameters.

e.g. to specify the destination number for a SMS-SUBMIT message:

```go
func to() ([]tpdu.TPDU, error) {
	return sms.Encode([]byte("hello"), sms.To("12345"))
}
```

or to encode a message using a particular character set, if necessary:

```go
func urdu() ([]tpdu.TPDU, error) {
	return sms.Encode([]byte("hello ٻ"), sms.WithCharset(charset.Urdu))
}
```

or to specify the encoding of a SMS-DELIVER message:

```go
func deliver() ([]tpdu.TPDU, error) {
	return sms.Encode([]byte("hello"), sms.AsDeliver, sms.From("12345"))
}
```

or to unmarshal a TPDU from the mobile station:

```go
func unmarshalMO(bintpdu []byte) (*tpdu.TPDU, error) {
	return sms.Unmarshal(bintpdu, sms.AsMO)
}
```

The full set of supplied options:

Option | Category | Description
---|---|---
*WithReassemblyTimeout(duration)*|Collector|Limit the time allowed to collect the segments of a message, from the first (default 24 hours, 0 for none)
*WithReassemblyLimit(n)*|Collector|Limit the number of segments held, abandoning the oldest reassemblies (default 4096, 0 for none)
*WithExpiryHandler(handler)*|Collector|Pass the segments of each abandoned reassembly to the handler, with the reason
*WithOriginator(originator)*|Collect|Identify the originator of an SMS-SUBMIT, which Collect requires
*WithTemplate(tpdu)*|Encode|Use the provided TPDU as the template for encoded TPDUs.
*WithTemplateOption(tpdu.Option)*|Encode|Apply the provided option to the template TPDU during encoding.
*To(number)*|Encode|Set the DA of the encoded TPDU to the number provided
*From(number)*|Encode|Set the OA of the encoded TPDU to the number provided
*AsSubmit*|Encode|Encode the TPDU as a SMS-SUBMIT (default for *sms.Encode*)
*AsDeliver*|Encode|Encode the TPDU as a SMS-DELIVER (default for *sms.NewEncoder*)
*As8Bit*|Encode|Force the encoding of user data as 8-bit
*AsUCS2*|Encode|Force the encoding of user data as UCS-2, from UTF-16
*WithMR(counter)*|Encode|Draw the TP-MR of each TPDU from the counter
*WithConcatRef(counter)*|Encode|Draw the reference of each concatenated message from the counter
*With16BitConcatRef*|Encode|Use 16-bit rather than 8-bit concatenation references
*WithAllCharsets*|Decode,Encode|Make all GSM7 character sets available
*WithDefaultCharset*|Decode,Encode|Make only the default character set available
*WithCharset(nli...)*|Decode,Encode|Make the specified character set(s) available
*WithLockingCharset(nli...)*|Decode,Encode|Make the specified character set(s) available as a locking character set
*WithShiftCharset(nli...)*|Decode,Encode|Make the specified character set(s) available as a shift character set
*AsMO*|Unmarshal|Treat the TPDU as originating from the mobile station
*AsMT*|Unmarshal|Treat the TPDU as terminating at the mobile station (default)
*AsRPAck*|Unmarshal|Treat an SMS-DELIVER-REPORT or SMS-SUBMIT-REPORT as carried by an RP-ACK, without a TP-FCS (default)
*AsRPError*|Unmarshal|Treat an SMS-DELIVER-REPORT or SMS-SUBMIT-REPORT as carried by an RP-ERROR, with a TP-FCS

## Tools

The [cmd](cmd) directory contains basic command line tools to exercise, debug and
demonstrate the core functionality of the library, including:

- encoding messages into SMS-SUBMIT TPDUs [(smssubmit)](cmd/smssubmit/smssubmit.go)
- decoding SMS-DELIVER TPDUs into messages [(smsdeliver)](cmd/smsdeliver/smsdeliver.go)
- decoding arbitrary TPDUs [(smsdecode)](cmd/smsdecode/smsdecode.go)
- counting the TPDUs a message needs [(smscounter)](cmd/smscounter/smscounter.go)
- displaying supported character sets [(charsets)](cmd/charsets/charsets.go).

The following examples demonstrate the example commands, and their code provides some examples of using the library.

smssubmit, smsdeliver, smsdecode and smscounter report errors on stderr and
exit with status 0 on success, 1 if they cannot process their input, and 2 if
their arguments are not valid.

### Submit Encoding

Creating an SMS to send:

```shell
$ smssubmit -number 12345 -message "Hello world"
Submit TPDU:
010105912143f500000bc8329bfd06dddf723619
```

Long messages are split into a concatenated message spanning several TPDUs.
Their concatenation reference, here ac, the fourth octet of each TP-UD, is
drawn at random by each run:

```shell
$ smssubmit -number 12345 -message "this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you might think 😁"
Submit TPDU 1:
410105912143f500088c050003ac0301007400680069007300200069007300200061002000760065007200790020006c006f006e00670020006d0065007300730061006700650020007400680061007400200064006f006500730020006e006f0074002000660069007400200069006e00200061002000730069006e0067006c006500200053004d00530020006d0065007300730061
Submit TPDU 2:
410205912143f500088c050003ac030200670065002c0020006100740020006c0065006100730074002000690074002000770069006c006c002000690066002000490020006b00650065007000200061006400640069006e00670020006d006f0072006500200074006f0020006900740020006100730020003100360030002000630068006100720061006300740065007200730020
Submit TPDU 3:
410305912143f5000844050003ac0303006900730020006d006f007200650020007400680061006e00200079006f00750020006d00690067006800740020007400680069006e006b0020d83dde01
```

The -language option, a National Language Identifier from 1 to 13, makes
that character set available too.

### Deliver Decoding

Decoding an SMS received from a GSM modem in PDU mode:

```shell
$ smsdeliver -p 07911614220991F1040B911605935713F200008140806113912304D7F79B0E
+61503975312: Woot
```

### Concatenated Message Decoding

Concatenating and displaying a message split into multiple SMS-DELIVER TPDUs.

```shell
$ smsdeliver 400B911605935713F20008814080611373238C050003C00301007400680069007300200069007300200061002000760065007200790020006C006F006E00670020006D0065007300730061006700650020007400680061007400200064006F006500730020006E006F0074002000660069007400200069006E00200061002000730069006E0067006C006500200053004D00530020006D0065007300730061 400B911605935713F20008814080611373238C050003C0030200670065002C0020006100740020006C0065006100730074002000690074002000770069006C006C002000690066002000490020006B00650065007000200061006400640069006E00670020006D006F0072006500200074006F0020006900740020006100730020003100360030002000630068006100720061006300740065007200730020 440B911605935713F200088140806113832344050003C00303006900730020006D006F007200650020007400680061006E00200079006F00750020006D00690067006800740020007400680069006E006B0020D83DDE01
+61503975312: this is a very long message that does not fit in a single SMS message, at least it will if I keep adding more to it as 160 characters is more than you might think 😁
```

A message left incomplete is reported, and makes smsdeliver exit with status 1:

```shell
$ smsdeliver 400B911605935713F20008814080611373238C050003C00301007400680069007300200069007300200061002000760065007200790020006C006F006E00670020006D0065007300730061006700650020007400680061007400200064006F006500730020006E006F0074002000660069007400200069006E00200061002000730069006E0067006C006500200053004D00530020006D0065007300730061
smsdeliver: incomplete message from +61503975312, reference 192: have segments 1 of 3, missing 2, 3
```

### General TPDU Decoding

Decoding the Submit TPDU created above:

```shell
$ smsdecode -o 010105912143f500000bc8329bfd06dddf723619
TPDU: SMS-SUBMIT
TP-MTI: 0x01 SMS-SUBMIT
TP-RD: false
TP-VPF: 0x00 Not Present
TP-RP: false
TP-UDHI: false
TP-SRR: false
TP-MR: 1
TP-DA: +12345
TP-PID: 0x00
TP-DCS: 0x00 7bit
TP-VP: Not Present
TP-UD: 00000000  48 65 6c 6c 6f 20 77 6f  72 6c 64                 |Hello world|
```

Decoding the Deliver TPDU above:

```shell
$ smsdecode -p 07911614220991F1040B911605935713F200008140806113912304D7F79B0E
SMSC: +61412290191
TPDU: SMS-DELIVER
TP-MTI: 0x00 SMS-DELIVER
TP-MMS: 0x01 No more messages are waiting
TP-LP: false
TP-RP: false
TP-UDHI: false
TP-SRI: false
TP-OA: +61503975312
TP-PID: 0x00
TP-DCS: 0x00 7bit
TP-SCTS: 2018-04-08 16:31:19 +0800
TP-UD: 00000000  57 6f 6f 74                                       |Woot|
```

Decoding the first Deliver TPDU of the concatenated message above:

```shell
$ smsdecode 400B911605935713F20008814080611373238C050003C00301007400680069007300200069007300200061002000760065007200790020006C006F006E00670020006D0065007300730061006700650020007400680061007400200064006F006500730020006E006F0074002000660069007400200069006E00200061002000730069006E0067006C006500200053004D00530020006D0065007300730061
TPDU: SMS-DELIVER
TP-MTI: 0x00 SMS-DELIVER
TP-MMS: 0x00 More messages are waiting
TP-LP: false
TP-RP: false
TP-UDHI: true
TP-SRI: false
TP-OA: +61503975312
TP-PID: 0x00
TP-DCS: 0x08 UCS-2
TP-SCTS: 2018-04-08 16:31:37 +0800
TP-UDH: ID: 0  Data: [192 3 1]
TP-UD: 00000000  00 74 00 68 00 69 00 73  00 20 00 69 00 73 00 20  |.t.h.i.s. .i.s. |
       00000010  00 61 00 20 00 76 00 65  00 72 00 79 00 20 00 6c  |.a. .v.e.r.y. .l|
       00000020  00 6f 00 6e 00 67 00 20  00 6d 00 65 00 73 00 73  |.o.n.g. .m.e.s.s|
       00000030  00 61 00 67 00 65 00 20  00 74 00 68 00 61 00 74  |.a.g.e. .t.h.a.t|
       00000040  00 20 00 64 00 6f 00 65  00 73 00 20 00 6e 00 6f  |. .d.o.e.s. .n.o|
       00000050  00 74 00 20 00 66 00 69  00 74 00 20 00 69 00 6e  |.t. .f.i.t. .i.n|
       00000060  00 20 00 61 00 20 00 73  00 69 00 6e 00 67 00 6c  |. .a. .s.i.n.g.l|
       00000070  00 65 00 20 00 53 00 4d  00 53 00 20 00 6d 00 65  |.e. .S.M.S. .m.e|
       00000080  00 73 00 73 00 61                                 |.s.s.a|
```

Decoding the second Submit TPDU in the concatenated message above:

```shell
$ smsdecode -o 410205912143f500088c050003ac030200670065002c0020006100740020006c0065006100730074002000690074002000770069006c006c002000690066002000490020006b00650065007000200061006400640069006e00670020006d006f0072006500200074006f0020006900740020006100730020003100360030002000630068006100720061006300740065007200730020
TPDU: SMS-SUBMIT
TP-MTI: 0x01 SMS-SUBMIT
TP-RD: false
TP-VPF: 0x00 Not Present
TP-RP: false
TP-UDHI: true
TP-SRR: false
TP-MR: 2
TP-DA: +12345
TP-PID: 0x00
TP-DCS: 0x08 UCS-2
TP-VP: Not Present
TP-UDH: ID: 0  Data: [172 3 2]
TP-UD: 00000000  00 67 00 65 00 2c 00 20  00 61 00 74 00 20 00 6c  |.g.e.,. .a.t. .l|
       00000010  00 65 00 61 00 73 00 74  00 20 00 69 00 74 00 20  |.e.a.s.t. .i.t. |
       00000020  00 77 00 69 00 6c 00 6c  00 20 00 69 00 66 00 20  |.w.i.l.l. .i.f. |
       00000030  00 49 00 20 00 6b 00 65  00 65 00 70 00 20 00 61  |.I. .k.e.e.p. .a|
       00000040  00 64 00 64 00 69 00 6e  00 67 00 20 00 6d 00 6f  |.d.d.i.n.g. .m.o|
       00000050  00 72 00 65 00 20 00 74  00 6f 00 20 00 69 00 74  |.r.e. .t.o. .i.t|
       00000060  00 20 00 61 00 73 00 20  00 31 00 36 00 30 00 20  |. .a.s. .1.6.0. |
       00000070  00 63 00 68 00 61 00 72  00 61 00 63 00 74 00 65  |.c.h.a.r.a.c.t.e|
       00000080  00 72 00 73 00 20                                 |.r.s. |
```

A TPDU from the mobile station is decoded with -o, and an SMS-DELIVER-REPORT
or SMS-SUBMIT-REPORT carried by an RP-ERROR, which has a TP-FCS, with -e.

## Subpackages

The [tpdu](encoding/tpdu) package [![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/gomaja/go-sms/encoding/tpdu) provides the core TPDU types and conversions to and from their binary form.

The [pdumode](encoding/pdumode) package [![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/gomaja/go-sms/encoding/pdumode) provides encoding and decoding of PDUs exchanged with GSM modems in PDU mode.

A number of packages provide functionality to encode and decode TPDU fields:

The [bcd](encoding/bcd) package [![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/gomaja/go-sms/encoding/bcd) provides conversions to and from BCD format.

The [gsm7](encoding/gsm7) package [![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/gomaja/go-sms/encoding/gsm7) provides conversions to and from 7bit packed user data.

The [charset](encoding/gsm7/charset) package [![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/gomaja/go-sms/encoding/gsm7/charset) provides the character sets used to encode user data in GSM 7bit format as specified in 3GPP TS 23.038.

The [semioctet](encoding/semioctet) package [![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/gomaja/go-sms/encoding/semioctet) provides conversions to and from semioctet format.

The [ucs2](encoding/ucs2) package [![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/gomaja/go-sms/encoding/ucs2) provides conversions between runes and the UTF-16 octets of UCS2 user data.
