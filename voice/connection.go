package voice

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Connection represents a voice WebSocket connection
type Connection struct {
	*Identify
	ws  *websocket.Conn
	mux sync.RWMutex

	UDP *UDP

	SSRC            uint32
	heartbeatTicker *time.Ticker
	heartbeatAcked  bool
}

// New makes a new connection
func New() *Connection {
	return &Connection{
		mux:            sync.RWMutex{},
		UDP:            &UDP{},
		heartbeatAcked: true,
	}
}

// Connect establishes a connection to the Discord voice servers
func (c *Connection) Connect(endpoint string, identify *Identify) error {
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		return err
	}

	conn.SetCloseHandler(c.closeHandler)
	c.ws = conn
	c.Identify = identify
	go c.listen()

	return nil
}

func (c *Connection) heartbeat() error {
	if !c.heartbeatAcked {
		return c.Reconnect()
	}

	c.heartbeatAcked = false
	return c.Send(OpHeartbeat, HeartbeatPayload(time.Now().Unix()))
}

// SetSpeaking sets the speaking status
func (c *Connection) SetSpeaking(speaking bool, delay int) error {
	return c.Send(OpSpeaking, &SpeakingPayload{
		Speaking: speaking,
		Delay:    delay,
		SSRC:     c.SSRC,
	})
}

// Resume resumes the WebSocket session
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

	return c.ws.WriteJSON(&SendablePacket{
		OP:   op,
		Data: d,
	})
}

// Close closes the WebSocket connection with the (optionally) provided code and text
func (c *Connection) Close(code int, text string) (err error) {
	if code != 0 {
		msg := websocket.FormatCloseMessage(code, text)
		if err = c.ws.WriteMessage(websocket.CloseMessage, msg); err != nil {
			return
		}
	}

	return c.ws.Close()
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
		rp := &ReceivablePacket{}
		err = c.ws.ReadJSON(rp)
		if err != nil {
			break
		}

		switch rp.OP {
		case OpReady:
			pk := &ReadyPayload{}
			err = json.Unmarshal(rp.Data, pk)
			if err != nil {
				return
			}

			c.SSRC = pk.SSRC
			c.UDP.Connect(pk.SSRC, pk.IP, pk.Port)
			ip, port, _ := c.UDP.DiscoverIP()

			c.Send(OpSelectProtocol, &SelectProtocolPayload{
				Protocol: "udp",
				Data: SelectProtocolData{
					Address: ip,
					Port:    port,
					Mode:    "xsalsa20_poly1305",
				},
			})
		case OpHello:
			pk := &HelloPayload{}
			err = json.Unmarshal(rp.Data, pk)
			if err != nil {
				return
			}

			go c.heartbeater(time.Duration(pk.HeartbeatInterval*.75) * time.Millisecond)
			c.Send(OpIdentify, c.Identify)
		case OpHeartbeatAck:
			c.heartbeatAcked = true
		case OpSessionDescription:
			pk := &SessionDescriptionPayload{}
			err = json.Unmarshal(rp.Data, pk)
			if err != nil {
				return
			}

			c.UDP.SecretKey = pk.SecretKey
		}
	}
}

func (c *Connection) heartbeater(d time.Duration) {
	c.heartbeatTicker = time.NewTicker(d)

	for range c.heartbeatTicker.C {
		c.heartbeat()
	}
}

func (c *Connection) closeHandler(code int, text string) error {
	if c.heartbeatTicker != nil {
		c.heartbeatTicker.Stop()
	}

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
