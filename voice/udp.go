package voice

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"time"

	"golang.org/x/crypto/nacl/secretbox"
)

// UDP represents a UDP connection
type UDP struct {
	conn *net.UDPConn

	SecretKey [32]byte
	Seq       uint16
	SSRC      uint32
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

	b := bytes.NewBuffer(sl)
	ipStr, err := b.ReadString(0)
	if err != nil {
		return
	}

	ip = net.ParseIP(ipStr[:len(ipStr)-1])
	port = binary.LittleEndian.Uint16(sl[len(sl)-2:])
	return
}

func (u *UDP) Write(b []byte) (int, error) {
	var (
		h  = u.generateHeader()
		pk = []byte{}
	)

	u.Seq++
	secretbox.Seal(pk, b, &h, &u.SecretKey)
	return u.conn.Write(pk)
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

	binary.BigEndian.PutUint32(b[off:], uint32(time.Now().Unix()))
	off += 4

	binary.BigEndian.PutUint32(b[off:], u.SSRC)
	return b
}
