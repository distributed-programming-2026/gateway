package config

import (
	"encoding/base64"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func compileBasicAuth(r *Route) error {
	if r.BasicAuth == nil {
		return nil
	}
	if len(r.BasicAuth.Users) == 0 {
		return errors.New("basicAuth users required")
	}
	for _, user := range r.BasicAuth.Users {
		_, hash, ok := strings.Cut(user, ":")
		if !ok {
			return errors.New("invalid htpasswd record")
		}
		if strings.HasPrefix(hash, "{SHA}") {
			b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(hash, "{SHA}"))
			if err != nil || len(b) != 20 {
				return errors.New("invalid SHA password")
			}
		} else if _, err := bcrypt.Cost([]byte(hash)); err != nil {
			return errors.New("supported htpasswd hashes: SHA and bcrypt")
		}
	}
	return nil
}
