package socks5

import (
	"fmt"
	"log"
	"net"
)

type Reply struct {
	Rep     uint8
	Address Address
}

func NewReply(rep uint8) *Reply {
	reply := new(Reply)
	reply.Rep = rep
	reply.Address, _ = GetAddress([]byte{AtypIpv4, 0, 0, 0, 0, 0, 0})
	return reply
}

func GetReply(conn net.Conn) (*Reply, error) {
	reply := new(Reply)
	msg, err := RcvMsg(conn)
	if err != nil {
		return nil, err
	}
	size := len(msg)
	if size < msgMinSize {
		log.Println("message length is too short: ", size)
		return nil, fmt.Errorf("message length is too short")
	}
	if msg[0] != Version {
		log.Println("unsupported version: ", msg[0])
		return nil, fmt.Errorf("unsupported version: %v", msg[0])
	}
	if msg[1] >= RepNotSupported {
		log.Println("unsupported reply field: ", msg[1])
		return nil, fmt.Errorf("unsupported reply field: %v", msg[1])
	}
	reply.Rep = msg[1]
	if msg[2] != RSV {
		log.Println("unsupported reserved: ", msg[2])
		return nil, fmt.Errorf("unsupported reserved: %v", msg[2])
	}
	reply.Address, err = GetAddress(msg[3:])
	if err != nil {
		log.Println("unsupported address type: ", msg[2])
		return nil, fmt.Errorf("unsupported address type: %v", msg[3])
	}
	return reply, nil
}

func (r *Reply) GetBytes() []byte {
	return append([]byte{Version, r.Rep, RSV}, r.Address...)
}

func (r *Reply) SendTo(conn net.Conn) error {
	err := WriteMsg(conn, r.GetBytes())
	if err != nil {
		log.Println("send reply error: ", err)
		return err
	}
	return nil
}
