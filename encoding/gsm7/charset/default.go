// SPDX-License-Identifier: MIT

package charset

var (
	defaultDecoder = generateDecoderFromRunes(defaultRunes)
	// defaultExtDecoder is the extension table of 3GPP TS 23.038 V20.0.0
	// Section 6.2.1.1. It has no entry for 0x0D, which that table leaves
	// empty ("NOTE 2: Void"), so ESC 0x0D decodes as the main table's CR.
	defaultExtDecoder = Decoder{
		0x0a: '\f',
		0x14: '^',
		0x28: '{',
		0x29: '}',
		0x2f: '\\',
		0x3c: '[',
		0x3d: '~',
		0x3e: ']',
		0x40: '|',
		0x65: '€',
	}
	defaultEncoder    = generateEncoderFromRunes(defaultRunes)
	defaultExtEncoder = generateEncoder(defaultExtDecoder)
	defaultRunes      = []rune(
		"@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞ\x1bÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?" +
			"¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà")
)
