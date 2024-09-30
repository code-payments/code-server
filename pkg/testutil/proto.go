package testutil

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func ProtoEqual(a, b proto.Message) error {
	if proto.Equal(a, b) {
		return nil
	}

	aJSON, _ := protojson.Marshal(a)
	bJSON, _ := protojson.Marshal(b)

	return fmt.Errorf("expected: %v\nactual:   %v", string(aJSON), string(bJSON))
}

func ProtoSliceEqual[T proto.Message](a, b []T) error {
	if len(a) != len(b) {
		return fmt.Errorf("len(%d) != len(%d)", len(a), len(b))
	}

	for i := range a {
		if err := ProtoEqual(a[i], b[i]); err != nil {
			return fmt.Errorf("element mismatch at %d\n%w", i, err)
		}
	}

	return nil
}
