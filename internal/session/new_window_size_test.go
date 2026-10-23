package session

import "testing"

// Wire/bounds failures the real-PTY E2E cannot exercise: older senders omit
// the hint, older receivers ignore it, and stale or malformed hints must not
// allocate an unbounded emulator or change the established popup path.
func TestNewWindowSizeWireCompatibility(t *testing.T) {
	type oldIntent struct {
		CommandType string
		Args        []string
	}
	old := oldIntent{CommandType: "NewWindow", Args: []string{"legacy"}}
	data, err := encodePayload(old)
	if err != nil {
		t.Fatal(err)
	}
	var current ExecuteCommandPayload
	if err := decodePayload(data, &current); err != nil || current.InitialSize != nil {
		t.Fatalf("old sender: %+v, %v", current, err)
	}
	current.InitialSize = &NewWindowSize{Width: 62, Height: 22, Cols: 60, Rows: 20}
	data, err = encodePayload(current)
	if err != nil {
		t.Fatal(err)
	}
	var legacy oldIntent
	if err := decodePayload(data, &legacy); err != nil || legacy.CommandType != old.CommandType {
		t.Fatalf("old receiver: %+v, %v", legacy, err)
	}
	var roundtrip ExecuteCommandPayload
	if err := decodePayload(data, &roundtrip); err != nil || roundtrip.InitialSize == nil || *roundtrip.InitialSize != *current.InitialSize {
		t.Fatalf("size roundtrip: %+v, %v", roundtrip, err)
	}
}

func TestNewWindowSizeBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		hint *NewWindowSize
		want NewWindowSize
	}{
		{"legacy", nil, NewWindowSize{80, 24, 78, 22}},
		{"borderless", &NewWindowSize{30, 12, 30, 12}, NewWindowSize{30, 12, 30, 12}},
		{"framed", &NewWindowSize{32, 14, 30, 12}, NewWindowSize{32, 14, 30, 12}},
		{"stale-large", &NewWindowSize{100000, 100000, 99998, 99998}, NewWindowSize{60, 23, 58, 21}},
		{"zero", &NewWindowSize{30, 12, 0, 12}, NewWindowSize{80, 24, 78, 22}},
		{"negative", &NewWindowSize{-30, 12, 30, 12}, NewWindowSize{80, 24, 78, 22}},
		{"outside-frame", &NewWindowSize{30, 12, 31, 12}, NewWindowSize{80, 24, 78, 22}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			width, height, cols, rows := initialWindowSize(80, 24, LayoutReserve{Left: 20, Top: 1}, tc.hint)
			if got := (NewWindowSize{width, height, cols, rows}); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
