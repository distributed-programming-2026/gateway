package app

import (
	"errors"
	"net/http"
	"strings"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"golang.org/x/net/http/httpguts"
)

var forbidden = map[string]bool{"Host": true, "Content-Length": true, "Transfer-Encoding": true, "Trailer": true, "Connection": true, "Keep-Alive": true, "Proxy-Connection": true, "Proxy-Authenticate": true, "Proxy-Authorization": true, "Te": true, "Upgrade": true}

func validateMutation(m *pb.HeaderMutation, trailer bool) error {
	if m == nil {
		return nil
	}
	valid := func(name string) bool {
		return httpguts.ValidHeaderFieldName(name) && !forbidden[http.CanonicalHeaderKey(name)] && (!trailer || !strings.EqualFold(name, "Authorization"))
	}
	for _, key := range m.Remove {
		if !valid(key) {
			return errors.New("invalid header removal")
		}
	}
	for _, list := range [][]*pb.HeaderValues{m.Set, m.Append} {
		seen := map[string]bool{}
		for _, h := range list {
			if h == nil || !valid(h.Key) || len(h.Values) == 0 {
				return errors.New("invalid header mutation")
			}
			key := http.CanonicalHeaderKey(h.Key)
			if seen[key] {
				return errors.New("duplicate mutation")
			}
			seen[key] = true
			for _, v := range h.Values {
				if !httpguts.ValidHeaderFieldValue(v) {
					return errors.New("invalid header value")
				}
			}
		}
	}
	return nil
}
func validateDecision(d *pb.InterceptResponse, phase string, route *config.Route, mode config.ProcessingMode) error {
	if d == nil || d.Result == nil {
		return errors.New("empty decision")
	}
	if immediate := d.GetImmediateResponse(); immediate != nil {
		return validateImmediate(immediate)
	}
	m := d.GetContinueResponse()
	if m == nil {
		return errors.New("empty continue")
	}
	if err := validatePhase(m, phase); err != nil {
		return err
	}
	request := strings.HasPrefix(phase, "request")
	isBody := strings.HasSuffix(phase, "Body")
	if m.StatusCode != nil && (*m.StatusCode < 200 || *m.StatusCode > 599) {
		return errors.New("invalid status")
	}
	for _, h := range []*pb.HeaderMutation{m.HeaderMutation, m.ResponseHeaderMutation} {
		if err := validateMutation(h, false); err != nil {
			return err
		}
	}
	if err := validateMutation(m.TrailerMutation, true); err != nil {
		return err
	}
	if m.BodyMutation != nil && route.UseHTTP2 {
		return errors.New("HTTP2 body mutation unsupported")
	}
	if m.BodyMutation != nil && isBody && ((request && mode.RequestBody != config.BodyBuffered) || (!request && mode.ResponseBody != config.BodyBuffered)) {
		return errors.New("body mutation requires buffer")
	}
	return validateRequestMutation(m.RequestMutation)
}

func validateImmediate(immediate *pb.ImmediateResponse) error {
	if immediate.StatusCode < 200 || immediate.StatusCode > 599 {
		return errors.New("invalid status")
	}
	for _, h := range immediate.Headers {
		if h == nil || !httpguts.ValidHeaderFieldName(h.Key) || !httpguts.ValidHeaderFieldValue(h.Value) || forbidden[http.CanonicalHeaderKey(h.Key)] {
			return errors.New("invalid immediate header")
		}
	}
	return nil
}

func validatePhase(m *pb.ContinueResponse, phase string) error {
	request := strings.HasPrefix(phase, "request")
	isHeaders := strings.HasSuffix(phase, "Headers")
	isBody := strings.HasSuffix(phase, "Body")
	isTrailers := strings.HasSuffix(phase, "Trailers")
	if (m.HeaderMutation != nil && !isHeaders) || (m.TrailerMutation != nil && !isTrailers) || (m.RequestMutation != nil && phase != config.RequestHeaders) || (m.StatusCode != nil && phase != config.ResponseHeaders) || (m.ResponseHeaderMutation != nil && !request) || (m.BodyMutation != nil && !isBody && !isHeaders) {
		return errors.New("mutation wrong phase")
	}
	return nil
}
