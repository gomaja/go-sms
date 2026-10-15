// SPDX-License-Identifier: MIT

// Package cbs implements the CBS Data Coding Scheme of 3GPP TS 23.038
// V20.0.0 Section 5, which codes Cell Broadcast messages and USSD strings,
// and provides helpers for coding USSD text with it. Section 5 states:
// "These codings may also be used for USSD and MMI/display purposes."
// TS 29.002 V19.1.0 Section 17.7.4 defines USSD-DataCodingScheme with the
// comment: "The structure of the USSD-DataCodingScheme is defined by the
// Cell Broadcast Data Coding Scheme as described in TS 3GPP TS 23.038".
//
// Cell Broadcast message pages (TS 23.041) are not implemented. The text
// helpers use the USSD packing of TS 23.038 Section 6.1.2.3.1, the default
// alphabet and extension table of Sections 6.2.1 and 6.2.1.1, and UCS2 of
// Section 6.2.3. They do not use national language tables. Compressed text,
// 8 bit data, user data headers, I1 protocol messages and WAP are unsupported.
// Reserved codings are decoded as GSM7 and refused on encode.
//
// USSD length limits are left to the caller, including the 160-octet
// maxUSSD-StringLength in TS 29.002 Section 17.7.4 and TS 23.038 Section 5's
// statement: "The message can then consist of up to 182 user characters".
// Empty text is accepted; no maximum text or encoded length is imposed.
package cbs
