// FILE: luminka/frame_test.go
// PURPOSE: Verify binary websocket envelope packing and unpacking.
// OWNS: Round-trip and malformed-frame coverage for the transport codec.
// EXPORTS: TestEncodeDecodeFrame
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"bytes"
	"testing"
)

type testFrameHeader struct {
	Event string `json:"event"`
	ID    string `json:"id,omitempty"`
}

func TestEncodeDecodeFrame(t *testing.T) {
	tests := []struct {
		name    string
		header  testFrameHeader
		payload []byte
	}{
		{name: "header only", header: testFrameHeader{Event: "app_info", ID: "a1"}},
		{name: "header and payload", header: testFrameHeader{Event: "stream_chunk", ID: "s1"}, payload: []byte("hello world")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frame, err := encodeFrame(tc.header, tc.payload)
			if err != nil {
				t.Fatalf("encodeFrame() error = %v", err)
			}

			var got testFrameHeader
			payload, err := decodeFrame(frame, &got)
			if err != nil {
				t.Fatalf("decodeFrame() error = %v", err)
			}
			if got != tc.header {
				t.Fatalf("header = %#v, want %#v", got, tc.header)
			}
			if !bytes.Equal(payload, tc.payload) {
				t.Fatalf("payload = %q, want %q", string(payload), string(tc.payload))
			}
		})
	}
}

func TestDecodeFrameRejectsMalformedInputs(t *testing.T) {
	tests := []struct {
		name    string
		frame   []byte
		wantErr string
	}{
		{name: "short frame", frame: []byte{1, 2, 3}, wantErr: "frame is too short"},
		{name: "bad header length", frame: []byte{0, 0, 0, 8, '{', '}', 'x'}, wantErr: "frame header length exceeds frame body"},
		{name: "bad json header", frame: []byte{0, 0, 0, 10, '{', 'n', 'o', 't', '-', 'j', 's', 'o', 'n', '}'}, wantErr: "invalid character"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got testFrameHeader
			_, err := decodeFrame(tc.frame, &got)
			if err == nil {
				t.Fatal("decodeFrame() error = nil, want failure")
			}
			if got := err.Error(); got == "" || !containsString(got, tc.wantErr) {
				t.Fatalf("error = %q, want containing %q", got, tc.wantErr)
			}
		})
	}
}

func containsString(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
