package socks5

import (
	"context"
	"log"
	"net"
	"strings"
)

func connectHandle(ctx context.Context, conn net.Conn, req *Request) {
	ip, port, err := req.Address.ResolveIPAddr()
	if err != nil {
		log.Println("resolve ip error: ", err)
		NewReply(RepHostUnreachable).SendTo(conn)
		return
	}
	target, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP: ip, Port: port})
	if err != nil {
		msg := err.Error()
		resp := RepHostUnreachable
		if strings.Contains(msg, "refused") {
			resp = RepConnectionRefused
		} else if strings.Contains(msg, "network is unreachable") {
			resp = RepNetworkUnreachable
		}
		log.Println("dial tcp error: ", err)
		NewReply(resp).SendTo(conn)
		return
	}
	defer target.Close()
	local := target.LocalAddr().(*net.TCPAddr)
	reply := new(Reply)
	reply.Address = ToAddress(local.IP, local.Port)
	reply.Rep = RepSuccess
	err = reply.SendTo(conn)
	if err != nil {
		log.Println("send reply: ", err)
		return
	}
	errChan := exchangeData(target, conn)
	select {
	case <-ctx.Done():
		log.Println("ctx done: ", ctx.Err())
	case err := <-errChan:
		log.Println("exchange data done: ", err)
	}
}
