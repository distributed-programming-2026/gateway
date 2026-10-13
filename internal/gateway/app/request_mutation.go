package app

import (
	"errors"
	"net/url"
	"strings"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"golang.org/x/net/http/httpguts"
)

func validateRequestMutation(v *pb.RequestMutation) error {
	if v == nil {
		return nil
	}
	if v.Method != nil && !httpguts.ValidHeaderFieldName(*v.Method) {
		return errors.New("invalid method mutation")
	}
	if v.Path != nil && (!strings.HasPrefix(*v.Path, "/") || strings.ContainsAny(*v.Path, "?#\r\n")) {
		return errors.New("invalid path mutation")
	}
	if v.Authority != nil {
		u, err := url.Parse("http://" + *v.Authority)
		if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("invalid authority mutation")
		}
	}
	if v.RawQuery != nil && (strings.ContainsAny(*v.RawQuery, "#\r\n") || strings.HasPrefix(*v.RawQuery, "?")) {
		return errors.New("invalid query mutation")
	}
	return nil
}
