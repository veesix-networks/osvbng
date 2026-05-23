// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package paths

// WildcardType names the type tag in a path wildcard segment of the form
// `<*:type>`. Centralised so the OpenAPI URL converter, the osvbngcli help
// renderer, and any future consumer all agree on the set of recognised types.
type WildcardType string

const (
	WildcardIP       WildcardType = "ip"
	WildcardIPv4     WildcardType = "ipv4"
	WildcardIPv6     WildcardType = "ipv6"
	WildcardPrefix   WildcardType = "prefix"
	WildcardMAC      WildcardType = "mac"
	WildcardRD       WildcardType = "rd"
	WildcardProtocol WildcardType = "protocol"
	WildcardLSAType  WildcardType = "lsa-type"

	WildcardUint8  WildcardType = "uint8"
	WildcardUint16 WildcardType = "uint16"
	WildcardUint32 WildcardType = "uint32"
	WildcardUint64 WildcardType = "uint64"
	WildcardInt8   WildcardType = "int8"
	WildcardInt16  WildcardType = "int16"
	WildcardInt32  WildcardType = "int32"
	WildcardInt64  WildcardType = "int64"
)
