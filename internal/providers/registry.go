package providers

import "sort"

// Spec contains server-side policy for a source-defined media provider.
type Spec struct {
	ID              string
	AllowedDomains  []string
	AuthCookieNames []string
}

var registry = map[string]Spec{
	"youtube": {
		ID:              "youtube",
		AllowedDomains:  []string{"youtube.com"},
		AuthCookieNames: []string{"SAPISID", "SID", "HSID", "SSID", "LOGIN_INFO", "__Secure-3PSID", "__Secure-1PSID"},
	},
	"tiktok": {
		ID:             "tiktok",
		AllowedDomains: []string{"tiktok.com"},
	},
	"instagram": {
		ID:             "instagram",
		AllowedDomains: []string{"instagram.com"},
	},
	"x": {
		ID:             "x",
		AllowedDomains: []string{"x.com", "twitter.com"},
	},
}

func Lookup(id string) (Spec, bool) {
	spec, ok := registry[id]
	if !ok {
		return Spec{}, false
	}
	spec.AllowedDomains = append([]string(nil), spec.AllowedDomains...)
	spec.AuthCookieNames = append([]string(nil), spec.AuthCookieNames...)
	return spec, true
}

func ValidID(id string) bool {
	_, ok := registry[id]
	return ok
}

func IDs() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
