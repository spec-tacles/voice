package udp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"time"

	"golang.org/x/crypto/nacl/secretbox"
)

// Connection represents a UDP connection
type Connection struct {
	udp *net.UDPConn

	SecretKey [32]byte
	Seq       uint16
	SSRC      uint32
}

// New makes a new connection
func New() *Connection {
	return &Connection{}
}

// Connect establishes a UDP connection
func (c *Connection) Connect(ssrc uint32, ip net.IP, port uint16) error {
	addr := &net.UDPAddr{
		IP:   ip,
		Port: int(port),
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}

	c.SSRC = ssrc
	c.udp = conn
	return nil
}

// DiscoverIP discovers our local IP
func (c *Connection) DiscoverIP() (ip net.IP, port uint16, err error) {
	if c.udp == nil {
		err = errors.New("attempted to discover IP before connecting to UDP")
		return
	}

	sl := make([]byte, 70)
	binary.LittleEndian.PutUint32(sl[:4], c.SSRC)

	_, err = c.udp.Write(sl)
	if err != nil {
		return
	}

	_, err = c.udp.Read(sl)
	if err != nil {
		return
	}

	b := bytes.NewBuffer(sl)
	ipStr, err := b.ReadString(0)
	if err != nil {
		return
	}

	ip = net.ParseIP(ipStr[:len(ip)-1])
	port = binary.LittleEndian.Uint16(sl[len(sl)-2:])
	return
}

func (c *Connection) Write(b []byte) (int, error) {
	var (
		h  = c.generateHeader()
		pk = []byte{}
	)

	c.Seq++
	secretbox.Seal(pk, b, &h, &c.SecretKey)
	return c.udp.Write(pk)
}

func (c *Connection) generateHeader() [24]byte {
	if c.SSRC == 0 {
		panic("attempted to generate packet header before SSRC was available")
	}

	var (
		b   = [24]byte{0x80, 0x78}
		off = 2
	)

	binary.BigEndian.PutUint16(b[off:], c.Seq)
	off += 2

	binary.BigEndian.PutUint32(b[off:], uint32(time.Now().Unix()))
	off += 4

	binary.BigEndian.PutUint32(b[off:], c.SSRC)
	return b
}
