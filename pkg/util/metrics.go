package util

import (
	"encoding/json"
	"fmt"
	"maps"
)

var OperationNames = []string{
	"PutItem",
	"GetItem",
	"UpdateItem",
	"DeleteItem",
	"Scan",
	"Query",
	"BatchWriteItem",
	"BatchGetItem",
	"TransactWriteItems",
	"TransactGetItems",
}

type Metric struct {
	Namespace  string
	Name       string
	Dimensions map[string]string
	Extras     map[string]string
}

func (m *Metric) UnmarshalJSON(bytes []byte) error {
	var raw []any

	err := json.Unmarshal(bytes, &raw)
	if err != nil {
		return err
	}

	if len(raw) < 2 {
		return nil
	}

	m.Namespace = raw[0].(string)
	m.Name = raw[1].(string)

	for i := 2; i < len(raw)-1; i += 2 {
		key := raw[i].(string)
		value := raw[i+1].(string)
		m.Dimensions[key] = value
	}

	// check if last element is a map of extras
	if len(raw)%2 == 1 {
		extras := raw[len(raw)-1]
		if extrasMap, ok := extras.(map[string]any); ok {
			m.Extras = make(map[string]string)
			for k, v := range extrasMap {
				m.Extras[k] = v.(string)
			}
		}
	}

	return nil
}

func (m Metric) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.ToAny())
}

func (m *Metric) ToAny() []any {
	raw := []any{m.Namespace, m.Name}

	for k, v := range m.Dimensions {
		raw = append(raw, k, v)
	}

	if len(m.Extras) > 0 {
		raw = append(raw, m.Extras)
	}

	return raw
}

func AllMetricsV1(ns, dimSdk, dimSize, dimOs, dimArch, idPrefix string, extras map[string]string) []Metric {
	var out []Metric

	if extras == nil {
		extras = make(map[string]string)
	}

	for idx, op := range OperationNames {
		e := maps.Clone(extras)
		e["id"] = fmt.Sprintf("%s%d", idPrefix, idx)
		out = append(out, Metric{
			Namespace: ns,
			Name:      op,
			Dimensions: map[string]string{
				DimensionNameSDK:  dimSdk,
				DimensionNameSize: dimSize,
				DimensionNameOS:   dimOs,
				DimensionNameArch: dimArch,
			},
			Extras: e,
		})
	}

	return out
}

func AllMetricsV2(ns, dimSdk, dimSize, dimOs, dimArch, idPrefix string, extras map[string]string) []Metric {
	var out []Metric

	if extras == nil {
		extras = make(map[string]string)
	}

	for idx, op := range OperationNames {
		e := maps.Clone(extras)
		e["id"] = fmt.Sprintf("%s%d", idPrefix, idx)
		out = append(out, Metric{
			Namespace: ns,
			Name:      "client.call.duration",
			Dimensions: map[string]string{
				DimensionNameSDK:   dimSdk,
				DimensionNameSize:  dimSize,
				DimensionNameOS:    dimOs,
				DimensionNameArch:  dimArch,
				DimensionRpcMethod: op,
			},
			Extras: e,
		})
	}

	return out
}

/*
-
client.call.attempts
client.call.duration
client.call.attempt_duration
client.call.auth.resolve_identity_duration
client.call.auth.signing_duration
client.call.serialization_duration
client.call.deserialization_duration
client.call.resolve_endpoint_duration
client.call.serialization_duration
client.call.duration
client.call.auth.resolve_identity_duration
client.call.auth.signing_duration
client.call.deserialization_duration
client.call.resolve_endpoint_duration
client.call.attempts
client.call.attempt_duration
client.call.resolve_endpoint_duration
client.call.attempts
client.call.deserialization_duration
client.call.auth.resolve_identity_duration
client.call.serialization_duration
client.call.auth.resolve_identity_duration
client.call.resolve_endpoint_duration
client.call.auth.signing_duration
client.call.deserialization_duration
client.call.attempts
client.call.attempt_duration
client.call.auth.signing_duration
client.call.duration
client.call.serialization_duration
*/
