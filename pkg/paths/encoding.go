package paths

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"
)

type WildcardType string

const (
	WildcardGeneric WildcardType = "*"
	WildcardIP      WildcardType = "*:ip"
	WildcardIPv4    WildcardType = "*:ipv4"
	WildcardIPv6    WildcardType = "*:ipv6"
	WildcardMAC     WildcardType = "*:mac"
	WildcardString  WildcardType = "*:string"
	WildcardUint8   WildcardType = "*:uint8"
	WildcardUint16  WildcardType = "*:uint16"
	WildcardUint32  WildcardType = "*:uint32"
	WildcardUint64  WildcardType = "*:uint64"
	WildcardInt8    WildcardType = "*:int8"
	WildcardInt16   WildcardType = "*:int16"
	WildcardInt32   WildcardType = "*:int32"
	WildcardInt64   WildcardType = "*:int64"
)

func parseWildcard(part string) (WildcardType, bool) {
	if !strings.HasPrefix(part, "<") || !strings.HasSuffix(part, ">") {
		return "", false
	}

	inner := part[1 : len(part)-1]
	return WildcardType(inner), true
}

func EncodeIP(ip string) (string, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("invalid IP address: %s", ip)
	}

	ip16 := parsed.To16()
	if ip16 == nil {
		return "", fmt.Errorf("failed to convert IP to 16 bytes: %s", ip)
	}

	return hex.EncodeToString(ip16), nil
}

func DecodeIP(encoded string) (string, error) {
	if len(encoded) != 32 {
		return "", fmt.Errorf("invalid encoded IP length: %d", len(encoded))
	}

	bytes, err := hex.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("invalid hex encoding: %w", err)
	}

	if len(bytes) != 16 {
		return "", fmt.Errorf("decoded bytes length is not 16: %d", len(bytes))
	}

	ip := net.IP(bytes)
	return ip.String(), nil
}

func EncodeMAC(mac string) (string, error) {
	parsed, err := net.ParseMAC(mac)
	if err != nil {
		return "", fmt.Errorf("invalid MAC address: %s", mac)
	}

	return hex.EncodeToString(parsed), nil
}

func DecodeMAC(encoded string) (string, error) {
	if len(encoded) != 12 {
		return "", fmt.Errorf("invalid encoded MAC length: %d", len(encoded))
	}

	bytes, err := hex.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("invalid hex encoding: %w", err)
	}

	mac := net.HardwareAddr(bytes)
	return mac.String(), nil
}

func encodeValue(value string, wildcardType WildcardType) (string, error) {
	switch wildcardType {
	case WildcardGeneric, WildcardString:
		return value, nil

	case WildcardIP, WildcardIPv4, WildcardIPv6:
		ip := net.ParseIP(value)
		if ip == nil {
			return "", fmt.Errorf("invalid IP address: %s", value)
		}

		if wildcardType == WildcardIPv4 && ip.To4() == nil {
			return "", fmt.Errorf("expected IPv4 address, got: %s", value)
		}
		if wildcardType == WildcardIPv6 && ip.To4() != nil {
			return "", fmt.Errorf("expected IPv6 address, got: %s", value)
		}

		return EncodeIP(value)

	case WildcardMAC:
		return EncodeMAC(value)

	case WildcardUint8, WildcardUint16, WildcardUint32, WildcardUint64:
		return value, nil

	case WildcardInt8, WildcardInt16, WildcardInt32, WildcardInt64:
		return value, nil

	default:
		return "", fmt.Errorf("unknown wildcard type: %s", wildcardType)
	}
}

func decodeValue(value string, wildcardType WildcardType) (string, error) {
	switch wildcardType {
	case WildcardGeneric, WildcardString:
		return value, nil

	case WildcardIP, WildcardIPv4, WildcardIPv6:
		if len(value) == 32 {
			return DecodeIP(value)
		}
		return value, nil

	case WildcardMAC:
		if len(value) == 12 {
			return DecodeMAC(value)
		}
		return value, nil

	case WildcardUint8, WildcardUint16, WildcardUint32, WildcardUint64:
		return value, nil

	case WildcardInt8, WildcardInt16, WildcardInt32, WildcardInt64:
		return value, nil

	default:
		return "", fmt.Errorf("unknown wildcard type: %s", wildcardType)
	}
}

func Build(pattern string, values ...string) (string, error) {
	parts := strings.Split(pattern, ".")
	wildcardIdx := 0

	for i, part := range parts {
		wildcardType, isWildcard := parseWildcard(part)
		if !isWildcard {
			continue
		}

		if wildcardIdx >= len(values) {
			return "", fmt.Errorf("not enough values for pattern wildcards")
		}

		value := values[wildcardIdx]
		wildcardIdx++

		encoded, err := encodeValue(value, wildcardType)
		if err != nil {
			return "", err
		}

		parts[i] = encoded
	}

	if wildcardIdx != len(values) {
		return "", fmt.Errorf("too many values for pattern wildcards")
	}

	return strings.Join(parts, "."), nil
}

func Extract(path string, pattern string) ([]string, error) {
	pathParts := strings.Split(path, ".")
	patternParts := strings.Split(pattern, ".")

	if len(pathParts) != len(patternParts) {
		return nil, fmt.Errorf("path does not match pattern")
	}

	var values []string
	for i, part := range patternParts {
		wildcardType, isWildcard := parseWildcard(part)
		if !isWildcard {
			continue
		}

		value := pathParts[i]
		decoded, err := decodeValue(value, wildcardType)
		if err != nil {
			return nil, err
		}

		values = append(values, decoded)
	}

	return values, nil
}

func ShouldEncode(pattern string, segmentIndex int) (WildcardType, bool) {
	parts := strings.Split(pattern, ".")
	if segmentIndex >= len(parts) {
		return "", false
	}

	wildcardType, isWildcard := parseWildcard(parts[segmentIndex])
	if !isWildcard {
		return "", false
	}

	if wildcardType == WildcardGeneric || wildcardType == WildcardString {
		return wildcardType, false
	}

	return wildcardType, true
}
