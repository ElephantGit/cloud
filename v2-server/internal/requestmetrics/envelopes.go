package requestmetrics

import "encoding/json"

func object(raw json.RawMessage) bool {
	var value map[string]json.RawMessage
	return json.Unmarshal(raw, &value) == nil && value != nil
}

func array(raw json.RawMessage) bool {
	var value []json.RawMessage
	return json.Unmarshal(raw, &value) == nil && value != nil
}

// validProtocolJSON permits provider extensions but requires the successful wire envelope.
// A provider can return an HTTP 200 diagnostic that does not contain a model response.
func validProtocolJSON(data []byte, protocol string) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(data, &value) != nil || value == nil {
		return false
	}
	if protocol == "anthropic-messages" {
		var kind string
		return json.Unmarshal(value["type"], &kind) == nil && kind == "message" && array(value["content"])
	}
	var choices []map[string]json.RawMessage
	if json.Unmarshal(value["choices"], &choices) != nil || len(choices) == 0 {
		return false
	}
	for _, choice := range choices {
		if !object(choice["message"]) {
			return false
		}
	}
	return true
}
