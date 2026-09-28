// SPDX-License-Identifier: MIT

package charset

var (
	turkishDecoder    = generateDecoderFromRunes(turkishRunes)
	turkishExtDecoder = Decoder{
		0x0a: '\f',
		0x14: '^',
		0x28: '{',
		0x29: '}',
		0x2f: '\\',
		0x3c: '[',
		0x3d: '~',
		0x3e: ']',
		0x40: '|',
		0x47: 'Ğ',
		0x49: 'İ',
		0x53: 'Ş',
		0x63: 'ç',
		0x65: '€',
		0x67: 'ğ',
		0x69: 'ı',
		0x73: 'ş',
	}
	turkishEncoder    = generateEncoderFromRunes(turkishRunes)
	turkishExtEncoder = generateEncoder(turkishExtDecoder)
	turkishRunes      = []rune(
		"@£$¥€éùıòÇ\nĞğ\rÅåΔ_ΦΓΛΩΠΨΣΘΞ\x1bŞşßÉ !\"#¤%&'()*+,-./0123456789:;<=>?" +
			"İABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§çabcdefghijklmnopqrstuvwxyzäöñüà")
)
