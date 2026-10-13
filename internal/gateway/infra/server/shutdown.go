package server

import (
	"context"
	"errors"
	"net/http"
	"time"
)

func shutdown(servers []*http.Server, grace time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	var result error
	for _, srv := range servers {
		if err := srv.Shutdown(ctx); err != nil {
			result = errors.Join(result, err, srv.Close())
		}
	}
	return result
}
