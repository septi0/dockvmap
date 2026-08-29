package service

import (
	"net/url"
	"strconv"
)

func validRegistryAddress(value string) bool {
	parsed, err := url.Parse("https://" + value)

	if err != nil || parsed.Host != value || parsed.User != nil || parsed.Path != "" {
		return false
	}

	host := parsed.Hostname()

	if host == "" {
		return false
	}

	port := parsed.Port()

	if port != "" {
		portNumber, err := strconv.Atoi(port)

		if err != nil || portNumber < 1 || portNumber > 65535 {
			return false
		}
	}

	return true
}
