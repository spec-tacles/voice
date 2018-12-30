package voice

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"time"

	"golang.org/x/crypto/nacl/secretbox"
)

// Audio constants
const (
	Channels      = 2
	FrameSize     = 960
	SampleRate    = 48000
	MaxBytes      = FrameSize * 4
	FrameDuration = (FrameSize / (SampleRate / 1000)) * time.Millisecond
)

// UDP represents a UDP connection
type UDP struct {
	conn *net.UDPConn

	FrameTicker *time.Ticker
	SecretKey   [32]byte
	Seq         uint16
	TS          uint32
	SSRC        uint32
}

// NewUDP makes a new UDP connection
func NewUDP() *UDP {
	return &UDP{
		FrameTicker: time.NewTicker(FrameDuration),
	}
}

// Connect establishes a UDP connection
func (u *UDP) Connect(ssrc uint32, ip net.IP, port uint16) error {
	addr := &net.UDPAddr{
		IP:   ip,
		Port: int(port),
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}

	u.SSRC = ssrc
	u.conn = conn
	return nil
}

// DiscoverIP discovers our local IP
func (u *UDP) DiscoverIP() (ip net.IP, port uint16, err error) {
	if u.conn == nil {
		err = errors.New("attempted to discover IP before connecting to UDP")
		return
	}

	sl := make([]byte, 70)
	binary.LittleEndian.PutUint32(sl[:4], u.SSRC)

	_, err = u.conn.Write(sl)
	if err != nil {
		return
	}

	_, err = u.conn.Read(sl)
	if err != nil {
		return
	}

	ipSpace := sl[4:68]
	ip = net.ParseIP(string(ipSpace[:bytes.IndexByte(ipSpace, 0)]))
	port = binary.LittleEndian.Uint16(sl[68:])
	return
}

// Close closes this UDP connection
func (u *UDP) Close() {
	u.FrameTicker.Stop()
	u.conn.Close()
}

func (u *UDP) Write(b []byte) (int, error) {
	<-u.FrameTicker.C
	h := u.generateHeader()

	u.Seq++
	u.TS += FrameSize
	return u.conn.Write(secretbox.Seal(h[:12], b, &h, &u.SecretKey))
}

func (u *UDP) generateHeader() [24]byte {
	if u.SSRC == 0 {
		panic("attempted to generate packet header before SSRC was available")
	}

	var (
		b   = [24]byte{0x80, 0x78}
		off = 2
	)

	binary.BigEndian.PutUint16(b[off:], u.Seq)
	off += 2

	binary.BigEndian.PutUint32(b[off:], u.TS)
	off += 4

	binary.BigEndian.PutUint32(b[off:], u.SSRC)
	return b
}
