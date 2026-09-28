// SPDX-License-Identifier: MIT

package charset

var (
	defaultDecoder    = generateDecoderFromRunes(defaultRunes)
	defaultExtDecoder = Decoder{
		0x0a: '\f',
		0x0d: '\n',
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
