package voice

import (
	"math/rand"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Connection represents a voice Websocket connection
type Connection struct {
	ws  *websocket.Conn
	mux sync.RWMutex

	UDP       *UDP
	ServerID  uint64
	UserID    uint64
	SessionID string
	Token     string

	SSRC              uint32
	HeartbeatInterval time.Duration
	HeartbeatAcked    bool
}

// New makes a new connection
func New() *Connection {
	return &Connection{
		mux: sync.RWMutex{},
		UDP: &UDP{},
	}
}

// Connect establishes a connection to the Discord voice servers
func (c *Connection) Connect(endpoint string, payload *IdentifyPayload) error {
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		return err
	}

	conn.SetCloseHandler(c.closeHandler)
	c.ws = conn
	go c.listen()

	return c.Identify(payload)
}

// Identify identifies a new session with the voice gateway. Attempts to resume before identifying.
func (c *Connection) Identify(payload *IdentifyPayload) error {
	if c.SessionID != "" {
		return c.Resume()
	}

	if payload == nil {
		payload = &IdentifyPayload{
			ServerID:  c.ServerID,
			UserID:    c.UserID,
			SessionID: c.SessionID,
			Token:     c.Token,
		}
	} else {
		c.ServerID = payload.ServerID
		c.UserID = payload.UserID
		c.SessionID = payload.SessionID
		c.Token = payload.Token
	}

	return c.Send(OpIdentify, payload)
}

// Heartbeat sends a heartbeat to the voice server
func (c *Connection) Heartbeat() error {
	if !c.HeartbeatAcked {
		return c.Reconnect()
	}

	c.HeartbeatAcked = false
	return c.Send(OpHeartbeat, HeartbeatPayload(rand.Int()))
}

// SelectProtocol sends a select protocol packet to the voice server
func (c *Connection) SelectProtocol(payload *SelectProtocolPayload) error {
	return c.Send(OpSelectProtocol, payload)
}

// SetSpeaking sets the speaking status
func (c *Connection) SetSpeaking(speaking bool, delay int) error {
	return c.Send(OpSpeaking, &SpeakingPayload{
		Speaking: speaking,
		Delay:    delay,
		SSRC:     c.SSRC,
	})
}

// Resume resumes the websocket session
func (c *Connection) Resume() error {
	return c.Send(OpResume, &ResumePayload{
		ServerID:  c.ServerID,
		SessionID: c.SessionID,
		Token:     c.Token,
	})
}

// Send a packet
func (c *Connection) Send(op int, d interface{}) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	return c.ws.WriteJSON(&Packet{
		OP: op,
		D:  d,
	})
}

// Reconnect to the gateway
func (c *Connection) Reconnect() error {
	addr := c.ws.RemoteAddr()
	err := c.ws.Close()
	if err != nil {
		return err
	}

	return c.Connect(addr.String(), nil)
}

func (c *Connection) listen() {
	var err error

	for {
		var d = &Packet{}
		err = c.ws.ReadJSON(d)
		if err != nil {
			break
		}

		switch d.OP {
		case OpReady:
			pk := d.D.(ReadyPayload)

			c.SSRC = pk.SSRC
			c.UDP.Connect(pk.SSRC, pk.IP, pk.Port)
			ip, port, _ := c.UDP.DiscoverIP()

			c.SelectProtocol(&SelectProtocolPayload{
				Protocol: "udp",
				Data: SelectProtocolData{
					Address: ip,
					Port:    port,
					Mode:    "xsalsa20_poly1305",
				},
			})
		case OpHello:
			pk := d.D.(HelloPayload)
			c.HeartbeatInterval = time.Duration(float64(pk.HeartbeatInterval) * .75)
			go c.heartbeater()
		case OpHeartbeat:
			c.Heartbeat()
		case OpHeartbeatAck:
			c.HeartbeatAcked = true
		case OpSessionDescription:
			pk := d.D.(SessionDescriptionPayload)
			c.UDP.SecretKey = pk.SecretKey
		}
	}
}

func (c *Connection) heartbeater() {
	ticker := time.NewTicker(c.HeartbeatInterval)
	defer ticker.Stop()

	for range ticker.C {
		c.Heartbeat()
	}
}

func (c *Connection) closeHandler(code int, text string) error {
	switch code {
	case 4006, 4009:
		c.SessionID = ""
		fallthrough
	case 4014, 4015:
		defer c.Reconnect()
	}

	msg := websocket.FormatCloseMessage(code, text)
	return c.ws.WriteMessage(websocket.CloseMessage, msg)
}
