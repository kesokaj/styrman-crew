package ingress

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(PictureExtractor{})
	httpcaddyfile.RegisterHandlerDirective("picture_extractor", parsePictureExtractor)
}

// PictureExtractor is a Caddy HTTP handler that extracts the "picture" claim
// from the AUTHP_ACCESS_TOKEN JWT cookie (set by caddy-security) and injects
// it as an X-Caddy-Picture request header. No signature verification is
// performed because caddy-security has already validated the token upstream.
type PictureExtractor struct{}

// CaddyModule returns the module information.
func (PictureExtractor) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.picture_extractor",
		New: func() caddy.Module { return new(PictureExtractor) },
	}
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler. No config needed.
func (p *PictureExtractor) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next() // consume directive name
	return nil
}

// parsePictureExtractor is the Caddyfile parser for the picture_extractor directive.
func parsePictureExtractor(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var p PictureExtractor
	err := p.UnmarshalCaddyfile(h.Dispenser)
	return p, err
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (p PictureExtractor) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	cookie, err := r.Cookie(AuthCookieName)
	if err == nil && cookie.Value != "" {
		parts := strings.SplitN(cookie.Value, ".", 3)
		if len(parts) >= 2 {
			// JWT payload is base64url-encoded (no padding)
			payload, err := base64.RawURLEncoding.DecodeString(parts[1])
			if err == nil {
				var claims map[string]any
				if json.Unmarshal(payload, &claims) == nil {
					if pic, ok := claims["picture"].(string); ok {
						r.Header.Set(HeaderPicture, pic)
					}
				}
			}
		}
	}
	return next.ServeHTTP(w, r)
}

// Interface guards
var (
	_ caddyhttp.MiddlewareHandler = (*PictureExtractor)(nil)
	_ caddyfile.Unmarshaler       = (*PictureExtractor)(nil)
)
