package v1alpha1

import "encoding/json"

// convertViaJSON copies between two structurally-compatible types by
// round-tripping through JSON. It is used for v1alpha1 <-> v1alpha2 conversion
// where the JSON shapes are identical except for fields intentionally dropped
// in v1alpha2 (those keys are simply ignored on unmarshal).
func convertViaJSON(src, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
