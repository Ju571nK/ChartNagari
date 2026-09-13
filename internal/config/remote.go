package config

import (
	"fmt"
	"net/url"
	"strings"
)

func ValidateRemoteSettings(values map[string]string) error {
	mode := values["REMOTE_ACCESS"]
	if mode != "" && mode != "false" && mode != "true" {
		return fmt.Errorf("REMOTE_ACCESS must be true or false")
	}
	if mode == "true" && len(strings.TrimSpace(values["API_TOKEN"])) < 32 {
		return fmt.Errorf("remote access requires an API token of at least 32 characters")
	}
	for _, origin := range strings.Split(values["REMOTE_ALLOWED_ORIGINS"], ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
			return fmt.Errorf("allowed origins must be HTTPS origins (HTTP only for loopback), without paths or credentials")
		}
	}
	return nil
}
