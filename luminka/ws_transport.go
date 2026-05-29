// FILE: luminka/ws_transport.go
// PURPOSE: Encode websocket responses as binary envelopes and decode inbound frames.
// OWNS: Websocket read and write helpers plus protocol response serialization.
// EXPORTS: WriteWSMessage, WriteWSFrame, WriteErrorResponse, WriteFSResponse, WriteFSStreamResponse, WriteStatResponse, WriteFSResponseWithTypes, WriteDataResponse, WriteExecResponse
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gorilla/websocket"
)

func readWSFrame(conn *WSConnection) (int, WSMessage, []byte, error) {
	if conn == nil || conn.conn == nil {
		return 0, WSMessage{}, nil, fmt.Errorf("websocket connection is required")
	}
	msgType, data, err := conn.conn.ReadMessage()
	if err != nil {
		return 0, WSMessage{}, nil, err
	}
	switch msgType {
	case websocket.BinaryMessage:
		// Normal binary frame — parse as Luminka protocol
		var request WSMessage
		payload, err := decodeFrame(data, &request)
		if err != nil {
			return msgType, WSMessage{}, nil, err
		}
		return msgType, request, payload, nil
	case websocket.CloseMessage:
		return msgType, WSMessage{}, nil, io.EOF
	case websocket.PingMessage:
		// Auto-respond with Pong and skip silently
		if pingConn, ok := conn.conn.(*websocket.Conn); ok {
			_ = pingConn.WriteMessage(websocket.PongMessage, nil)
		}
		return msgType, WSMessage{}, nil, nil
	default:
		// TextMessage or other — return the raw frame for the caller to reject
		return msgType, WSMessage{}, nil, nil
	}
}

func WriteWSMessage(conn *WSConnection, message WSMessage) error {
	return WriteWSFrame(conn, message, nil)
}

func WriteWSFrame(conn *WSConnection, message WSMessage, payload []byte) error {
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

func WriteErrorResponse(conn *WSConnection, id json.RawMessage, message string) error {
	return WriteWSMessage(conn, WSMessage{Event: "response:error", ID: id, Error: message})
}

func WriteFSResponse(conn *WSConnection, requestEvent string, id json.RawMessage, ok bool, errMsg string, data *string, files []string, exists *bool) error {
	response := WSMessage{Event: "response:" + requestEvent, ID: id, Ok: boolPtr(ok), Error: errMsg, Files: files, Exists: exists}
	if data != nil {
		response.Data = rawStringData(*data)
	}
	return WriteWSMessage(conn, response)
}

func WriteFSStreamResponse(conn *WSConnection, requestEvent string, id json.RawMessage, ok bool, errMsg, streamID string) error {
	return WriteWSMessage(conn, WSMessage{Event: "response:" + requestEvent, ID: id, Ok: boolPtr(ok), Error: errMsg, StreamID: streamID})
}

func WriteExecResponse(conn *WSConnection, event string, id json.RawMessage, ok bool, errMsg, stdout, stderr string, code *int) error {
	return WriteWSMessage(conn, WSMessage{Event: event, ID: id, Ok: boolPtr(ok), Error: errMsg, Stdout: stdout, Stderr: stderr, Code: code})
}

func WriteStatResponse(conn *WSConnection, requestEvent string, id json.RawMessage, ok bool, errMsg string, stat map[string]any) error {
	statData, err := json.Marshal(stat)
	if err != nil {
		return WriteFSResponse(conn, requestEvent, id, false, "failed to marshal stat", nil, nil, nil)
	}
	return WriteWSMessage(conn, WSMessage{Event: "response:" + requestEvent, ID: id, Ok: boolPtr(ok), Error: errMsg, Stat: statData})
}

func WriteFSResponseWithTypes(conn *WSConnection, requestEvent string, id json.RawMessage, ok bool, errMsg string, files, fileTypes []string) error {
	return WriteWSMessage(conn, WSMessage{Event: "response:" + requestEvent, ID: id, Ok: boolPtr(ok), Error: errMsg, Files: files, FileTypes: fileTypes})
}

func WriteDataResponse(conn *WSConnection, requestEvent string, id json.RawMessage, ok bool, errMsg, data string) error {
	return WriteWSMessage(conn, WSMessage{Event: "response:" + requestEvent, ID: id, Ok: boolPtr(ok), Error: errMsg, Data: rawStringData(data)})
}
