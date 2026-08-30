package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

const autoProxySentinel = "auto"

func expandAutoProxies(values []string) ([]string, error) {
	out := make([]string, 0, len(values))

	for _, raw := range values {
		if !strings.EqualFold(strings.TrimSpace(raw), autoProxySentinel) {
			out = append(out, raw)
			continue
		}

		gw, err := defaultGateway()

		if err != nil {
			return nil, fmt.Errorf("%q: %w", autoProxySentinel, err)
		}

		slog.Info("trusted_proxies: resolved \"auto\" to the default gateway", "gateway", gw)
		out = append(out, gw.String())
	}

	return out, nil
}

func defaultGateway() (netip.Addr, error) {
	const routeFile = "/proc/net/route"

	data, err := os.ReadFile(routeFile)

	if err != nil {
		return netip.Addr{}, err
	}

	lines := strings.Split(string(data), "\n")

	for _, line := range lines[1:] {
		fields := strings.Fields(line)

		if len(fields) < 3 || fields[1] != "00000000" {
			continue
		}

		gw, err := strconv.ParseUint(fields[2], 16, 32)

		if err != nil || gw == 0 {
			continue
		}

		return netip.AddrFrom4([4]byte{byte(gw), byte(gw >> 8), byte(gw >> 16), byte(gw >> 24)}), nil
	}

	return netip.Addr{}, errors.New("no IPv4 default route in " + routeFile)
}
