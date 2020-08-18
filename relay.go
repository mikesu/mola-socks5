package socks5

import (
	"fmt"
	"log"
	"net"
)

type Relay struct {
	Frag    uint8
	Address Address
	From    *net.UDPAddr
	Data    []byte
}

func GetRelay(conn *net.UDPConn) (*Relay, error) {
	relay := new(Relay)
	buf := make([]byte, 1500)
	size, from, err := conn.ReadFromUDP(buf)
	if err != nil {
		log.Println("read udp error: ", err)
		return nil, err
	}
	if size < msgMinSize {
		log.Println("udp length is too short: ", size)
		return nil, fmt.Errorf("message length is too short")
	}
	msg := buf[:size]
	if msg[0] != RSV || msg[1] != RSV {
		log.Println("unsupported reserved: ", msg[0], msg[1])
		return nil, fmt.Errorf("unsupported reserved: %v,%v", msg[0], msg[1])
	}
	if msg[2] != FRAG {
		log.Println("unsupported fragment: ", msg[2])
		return nil, fmt.Errorf("unsupported fragment: %v", msg[2])
	}
	relay.From = from
	relay.Address, err = GetAddress(msg[3:])
	if err != nil {
		log.Println("unsupported address type: ", msg[3])
		return nil, fmt.Errorf("unsupported address type: %v", msg[3])
	}
	addressSize := len(relay.Address)
	if size > addressSize+3 {
		relay.Data = msg[addressSize+3:]
	}
	return relay, nil
}

func (r *Relay) GetBytes() []byte {
	return append(append([]byte{RSV, RSV, FRAG}, r.Address...), r.Data...)
}
