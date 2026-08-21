package ipmatch

import (
	"fmt"
	"net/netip"
	"strings"
)

type Set struct {
	prefixes []netip.Prefix
}

func Parse(values []string) (Set, error) {
	prefixes := make([]netip.Prefix, 0, len(values))

	for _, raw := range values {
		value := strings.TrimSpace(raw)

		if value == "" {
			continue
		}

		prefix, err := parseOne(value)

		if err != nil {
			return Set{}, fmt.Errorf("invalid IP or CIDR %q: %w", raw, err)
		}

		prefixes = append(prefixes, prefix)
	}

	return Set{prefixes: prefixes}, nil
}

func parseOne(value string) (netip.Prefix, error) {
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return prefix, nil
	}

	addr, err := netip.ParseAddr(value)

	if err != nil {
		return netip.Prefix{}, err
	}

	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

func (s Set) Contains(addr netip.Addr) bool {
	for _, prefix := range s.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}

	return false
}

func (s Set) Empty() bool {
	return len(s.prefixes) == 0
}
