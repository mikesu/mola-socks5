package socks5

import (
	"fmt"
	"log"
	"net"
)

func Negotiation(conn net.Conn) error {
	msg, err := RcvMsg(conn)
	if err != nil {
		return err
	}

	if msg[0] != Version {
		err := fmt.Errorf("unsupported SOCKS version: %v", msg[0])
		log.Printf("[ERR] socks: %v", err)
		return err
	}
	nMethods := int(msg[1])
	methods := msg[2 : nMethods+2]
	for _, method := range methods {
		if method == MethodNoAuth {
			sendNegotiation(conn, MethodNoAuth)
			return nil
		}
	}
	sendNegotiation(conn, MethodNoAcceptable)
	return fmt.Errorf("unsupported auth methods")
}

func sendNegotiation(conn net.Conn, method uint8) {
	negotiation := []byte{Version, method}
	err := WriteMsg(conn, negotiation)
	if err != nil {
		log.Printf("send negotiation error: %v", err)
	}
}
