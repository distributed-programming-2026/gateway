package app

import (
	"crypto/sha1" // #nosec G505 -- Required to verify existing SHA htpasswd records.
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"golang.org/x/crypto/bcrypt"
)

func (g *Gateway) selectRoute(x *exchange) error {
	limit, count := 0, 0
	for {
		x.route = g.matchRoute(x.request)
		if x.route == nil {
			return failure("route_not_found", 404)
		}
		r := x.route
		if !r.MethodAllowed(x.request.Method) {
			return failure("method_not_allowed", 403)
		}
		if r.Rewrite != nil {
			setPath(x.request, r.Rewrite.RE.ReplaceAllString(x.request.URL.Path, r.Rewrite.New))
		}
		if r.InternalRedirect == nil || !r.InternalRedirect.RE.MatchString(x.request.URL.Path) {
			return nil
		}
		routeLimit := 10
		if r.InternalRedirect.MaxRedirectCount != nil {
			routeLimit = *r.InternalRedirect.MaxRedirectCount
		}
		if limit == 0 || routeLimit < limit {
			limit = routeLimit
		}
		if count >= limit {
			return failure("redirect_limit", 500)
		}
		count++
		setPath(x.request, r.InternalRedirect.RE.ReplaceAllString(x.request.URL.Path, r.InternalRedirect.New))
	}
}
func (g *Gateway) matchRoute(req *http.Request) *config.Route {
	for i := range g.conf.Routes {
		if g.conf.Routes[i].Matches(req) {
			return &g.conf.Routes[i]
		}
	}
	return nil
}
func setPath(req *http.Request, path string) { req.URL.Path = path; req.URL.RawPath = "" }
func authorized(req *http.Request, auth *config.BasicAuth) bool {
	user, password, ok := req.BasicAuth()
	if !ok {
		return false
	}
	for _, record := range auth.Users {
		username, hash, _ := strings.Cut(record, ":")
		if subtle.ConstantTimeCompare([]byte(user), []byte(username)) != 1 {
			continue
		}
		if strings.HasPrefix(hash, "{SHA}") {
			digest := sha1.Sum([]byte(password)) // #nosec G401 -- Legacy htpasswd verification; new hashes use bcrypt.
			if subtle.ConstantTimeCompare([]byte(base64.StdEncoding.EncodeToString(digest[:])), []byte(strings.TrimPrefix(hash, "{SHA}"))) == 1 {
				return true
			}
		} else if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil {
			return true
		}
	}
	return false
}
