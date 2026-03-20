package chttp

// cookie cookie object.
//
// See: https://chromedevtools.github.io/devtools-protocol/tot/Network#type-cookie
type cookie struct {
	Name     string  `json:"name"`     // Cookie name.
	Value    string  `json:"value"`    // Cookie value.
	Domain   string  `json:"domain"`   // Cookie domain.
	Path     string  `json:"path"`     // Cookie path.
	Expires  float64 `json:"expires"`  // Cookie expiration date as the number of seconds since the UNIX epoch.
	Size     int64   `json:"size"`     // Cookie size.
	HTTPOnly bool    `json:"httpOnly"` // True if cookie is http-only.
	Secure   bool    `json:"secure"`   // True if cookie is secure.
	Session  bool    `json:"session"`  // True in case of session cookie.
	// SameSite           CookieSameSite      `json:"sameSite,omitempty,omitzero"`     // Cookie SameSite type.
	// Priority           CookiePriority      `json:"priority"`                        // Cookie Priority
	// SourceScheme       CookieSourceScheme  `json:"sourceScheme"`                    // Cookie source scheme type.
	SourcePort int64 `json:"sourcePort"` // Cookie source port. Valid values are {-1, [1, 65535]}, -1 indicates an unspecified port. An unspecified port value allows protocol clients to emulate legacy cookie scope for the port. This is a temporary ability and it will be removed in the future.
	// PartitionKey       *CookiePartitionKey `json:"partitionKey,omitempty,omitzero"` // Cookie partition key.
	PartitionKeyOpaque bool `json:"partitionKeyOpaque"` // True if cookie partition key is opaque.
}

// getCookiesResponses is the response from Storage.getCookies
type getCookiesResponses struct {
	Cookies []*cookie `json:"cookies"`
}

// getVersionResponse is the response from Browser.getVersion
type getVersionResponse struct {
	ProtocolVersion string `json:"protocolVersion"`
	Product         string `json:"product"`
	Revision        string `json:"revision"`
	UserAgent       string `json:"userAgent"`
	JSVersion       string `json:"jsVersion"`
}
