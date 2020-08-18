package socks5

import (
	"fmt"
	"log"
	"net"
)

type Request struct {
	Command uint8
	Address Address
}

func GetRequest(conn net.Conn) (*Request, error) {
	request := new(Request)
	msg, err := RcvMsg(conn)
	if err != nil {
		return nil, err
	}
	size := len(msg)
	if size < msgMinSize {
		for _, v := range msg {
			log.Print(v, ",")
		}
		log.Println("message length is too short: ", size)
		return nil, fmt.Errorf("message length is too short")
	}
	if msg[0] != Version {
		return nil, fmt.Errorf("unsupported version: %v", msg[0])
	}
	switch msg[1] {
	case CmdConnect:
		request.Command = msg[1]
	case CmdBind:
		request.Command = msg[1]
	case CmdAssociate:
		request.Command = msg[1]
	default:
		log.Println("unsupported command: ", msg[1])
		NewReply(RepCommandNotSupported).SendTo(conn)
		return nil, fmt.Errorf("unsupported command: %v", msg[1])
	}
	if msg[2] != RSV {
		log.Println("unsupported reserved: ", msg[2])
		return nil, fmt.Errorf("unsupported reserved: %v", msg[2])
	}
	request.Address, err = GetAddress(msg[3:])
	if err != nil {
		log.Println("unsupported address type: ", msg[3])
		NewReply(RepAddrTypeNotSupported).SendTo(conn)
		return nil, fmt.Errorf("unsupported address type: %v", msg[3])
	}
	return request, nil
}

func (r *Request) GetBytes() []byte {
	return append([]byte{Version, r.Command, RSV}, r.Address...)
}
