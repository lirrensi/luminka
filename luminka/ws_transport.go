// FILE: luminka/ws_transport.go
// PURPOSE: Encode websocket responses as binary envelopes and decode inbound frames.
// OWNS: Websocket read and write helpers plus protocol response serialization.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
)

func readWSFrame(conn *wsConnection) (int, wsMessage, []byte, error) {
	if conn == nil || conn.conn == nil {
		return 0, wsMessage{}, nil, fmt.Errorf("websocket connection is required")
	}
	msgType, data, err := conn.conn.ReadMessage()
	if err != nil {
		return 0, wsMessage{}, nil, err
	}
	if msgType != websocket.BinaryMessage {
		return msgType, wsMessage{}, nil, nil
	}
	var request wsMessage
	payload, err := decodeFrame(data, &request)
	if err != nil {
		return msgType, wsMessage{}, nil, err
	}
	return msgType, request, payload, nil
}

func writeWSMessage(conn *wsConnection, message wsMessage) error {
	return writeWSFrame(conn, message, nil)
}

func writeWSFrame(conn *wsConnection, message wsMessage, payload []byte) error {
	if conn == nil || conn.conn == nil {
		return fmt.Errorf("websocket connection is required")
	}
	data, err := encodeFrame(message, payload)
	if err != nil {
		return err
	}
	conn.writeMu.Lock()
	defer conn.writeMu.Unlock()
	return conn.conn.WriteMessage(websocket.BinaryMessage, data)
}

func writeErrorResponse(conn *wsConnection, id json.RawMessage, message string) error {
	return writeWSMessage(conn, wsMessage{Event: "error", ID: id, Error: message})
}

func writeFSResponse(conn *wsConnection, id json.RawMessage, ok bool, errMsg string, data *string, files []string, exists *bool) error {
	response := wsMessage{Event: "fs_response", ID: id, Ok: boolPtr(ok), Error: errMsg, Files: files, Exists: exists}
	if data != nil {
		response.Data = *data
	}
	return writeWSMessage(conn, response)
}

func writeFSStreamResponse(conn *wsConnection, id json.RawMessage, ok bool, errMsg, streamID string) error {
	return writeWSMessage(conn, wsMessage{Event: "fs_response", ID: id, Ok: boolPtr(ok), Error: errMsg, StreamID: streamID})
}

func writeExecResponse(conn *wsConnection, event string, id json.RawMessage, ok bool, errMsg, stdout, stderr string, code *int) error {
	return writeWSMessage(conn, wsMessage{Event: event, ID: id, Ok: boolPtr(ok), Error: errMsg, Stdout: stdout, Stderr: stderr, Code: code})
}
