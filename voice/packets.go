package voice

import (
	"encoding/json"
	"net"
)

// Packet OP codes
const (
	OpIdentify = iota
	OpSelectProtocol
	OpReady
	OpHeartbeat
	OpSessionDescription
	OpSpeaking
	OpHeartbeatAck
	OpResume
	OpHello
	OpResumed
	_
	_
	_
	OpClientDisconnect
)

// SendablePacket represents a sendable WebSocket packet
type SendablePacket struct {
	OP   int         `json:"op"`
	Data interface{} `json:"d"`
}

// ReceivablePacket represents a receivable WebSocket packet
type ReceivablePacket struct {
	OP   int             `json:"op"`
	Data json.RawMessage `json:"d"`
}

// Identify represents a voice identify payload
type Identify struct {
	ServerID  uint64 `json:"server_id,string"`
	UserID    uint64 `json:"user_id,string"`
	SessionID string `json:"session_id"`
	Token     string `json:"token"`
}

// SelectProtocolPayload represents a select protocol payload
type SelectProtocolPayload struct {
	Protocol string             `json:"protocol"`
	Data     SelectProtocolData `json:"data"`
}

// SelectProtocolData represents the data in a select protocol payload
type SelectProtocolData struct {
	Address net.IP `json:"address"`
	Port    uint16 `json:"port"`
	Mode    string `json:"mode"`
}

// ReadyPayload represents a voice ready payload
type ReadyPayload struct {
	SSRC              uint32   `json:"ssrc"`
	IP                net.IP   `json:"ip"`
	Port              uint16   `json:"port"`
	Modes             []string `json:"modes"`
	HeartbeatInterval int      `json:"heartbeat_interval"`
}

// HelloPayload represents a voice hello payload
type HelloPayload struct {
	HeartbeatInterval float64 `json:"heartbeat_interval"`
}

// HeartbeatPayload represents a voice heartbeat payload
type HeartbeatPayload int

// SessionDescriptionPayload represents a session description payload
type SessionDescriptionPayload struct {
	Mode      string   `json:"mode"`
	SecretKey [32]byte `json:"secret_key"`
}

// SpeakingPayload represents a speaking payload
type SpeakingPayload struct {
	Speaking bool   `json:"speaking"`
	Delay    int    `json:"delay"`
	SSRC     uint32 `json:"ssrc"`
}

// ResumePayload represents a session resume payload
type ResumePayload struct {
	ServerID  uint64 `json:"server_id"`
	SessionID string `json:"session_id"`
	Token     string `json:"token"`
}
