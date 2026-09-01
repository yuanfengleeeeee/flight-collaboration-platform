package event

import "testing"

func TestEquivalentJSONIgnoresObjectKeyOrdering(t *testing.T) {
	if !EquivalentJSON([]byte(`{"assignment_public_id":"a","expected_sync_version":1}`), []byte(`{"expected_sync_version":1,"assignment_public_id":"a"}`)) {
		t.Fatal("equivalent JSON objects should compare equal")
	}
}

func TestEquivalentJSONRejectsDifferentValues(t *testing.T) {
	if EquivalentJSON([]byte(`{"status":"accepted"}`), []byte(`{"status":"completed"}`)) {
		t.Fatal("different JSON values should not compare equal")
	}
}
