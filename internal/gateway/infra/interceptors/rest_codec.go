package interceptors

import (
	"encoding/json"
	"errors"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/getkin/kin-openapi/openapi3"
	"google.golang.org/protobuf/encoding/protojson"
)

func encodeEvent(request *pb.InterceptRequest) ([]byte, error) {
	b, err := (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(request)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return nil, err
	}
	var phase string
	var event json.RawMessage
	for key, value := range fields {
		if key != "context" {
			phase = key
			event = value
		}
	}
	payload := struct {
		EventType string          `json:"eventType"`
		Context   json.RawMessage `json:"context"`
		Event     json.RawMessage `json:"event"`
	}{phase, fields["context"], event}
	return json.Marshal(payload)
}
func decodeDecision(b []byte, schema *openapi3.Schema) (*pb.InterceptResponse, error) {
	var raw any
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, errors.New("malformed interceptor response")
	}
	if err := schema.VisitJSON(raw); err != nil {
		return nil, errors.New("invalid interceptor response schema")
	}
	var envelope struct {
		Payload map[string]json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return nil, err
	}
	var responseType string
	if err := json.Unmarshal(envelope.Payload["responseType"], &responseType); err != nil {
		return nil, err
	}
	delete(envelope.Payload, "responseType")
	value, err := json.Marshal(envelope.Payload)
	if err != nil {
		return nil, err
	}
	wrapped, err := json.Marshal(map[string]json.RawMessage{responseType: value})
	if err != nil {
		return nil, err
	}
	result := &pb.InterceptResponse{}
	if err := protojson.Unmarshal(wrapped, result); err != nil {
		return nil, errors.New("invalid interceptor decision")
	}
	return result, nil
}
