// FILE: luminka/frame.go
// PURPOSE: Encode and decode the canonical binary websocket envelope.
// OWNS: Binary frame packing, header-length validation, and JSON header decoding.
// EXPORTS: encodeFrame, decodeFrame
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

func encodeFrame(header any, payload []byte) ([]byte, error) {
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	if uint64(len(headerBytes)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("frame header is too large")
	}
	frame := make([]byte, 4+len(headerBytes)+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(headerBytes)))
	copy(frame[4:], headerBytes)
	copy(frame[4+len(headerBytes):], payload)
	return frame, nil
}

func decodeFrame(frame []byte, header any) ([]byte, error) {
	if len(frame) < 4 {
		return nil, errors.New("frame is too short")
	}
	headerLen := binary.BigEndian.Uint32(frame[:4])
	if headerLen > uint32(len(frame)-4) {
		return nil, errors.New("frame header length exceeds frame body")
	}
	headerBytes := frame[4 : 4+headerLen]
	if err := json.Unmarshal(headerBytes, header); err != nil {
		return nil, err
	}
	payload := make([]byte, len(frame)-(4+int(headerLen)))
	copy(payload, frame[4+int(headerLen):])
	return payload, nil
}
