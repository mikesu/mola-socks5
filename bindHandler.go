package socks5

import (
	"context"
	"fmt"
	"log"
	"net"
)

func bindHandle(ctx context.Context, conn net.Conn, req *Request) {
	ip, port, err := req.Address.ResolveIPAddr()
	if err != nil {
		log.Println("resolve ip error: ", err)
		NewReply(RepHostUnreachable).SendTo(conn)
		return
	}
	log.Println("bind request addr: ", ip.String(), port)
	listener, err := net.ListenTCP("tcp", nil)
	if err != nil {
		log.Println("listen tcp error: ", err)
		NewReply(RepServerFailure).SendTo(conn)
		return
	}
	defer listener.Close()
	local := listener.Addr().(*net.TCPAddr)
	reply := new(Reply)
	reply.Address = ToAddress(local.IP, local.Port)
	reply.Rep = RepSuccess
	err = reply.SendTo(conn)
	if err != nil {
		log.Println("send reply: ", err)
		return
	}
	log.Println("send bind reply success: ", local.String())
	accept := func() chan error {
		errChan := make(chan error, 2)
		go func() {
			target, err := listener.Accept()
			if err != nil {
				errChan <- err
				return
			}
			defer target.Close()
			remoteAddr := target.RemoteAddr().(*net.TCPAddr)
			log.Println("bind accept success: ", remoteAddr.String())
			if remoteAddr.IP.Equal(ip) && remoteAddr.Port == port {
				reply := new(Reply)
				reply.Address = ToAddress(ip, port)
				reply.Rep = RepSuccess
				err = reply.SendTo(conn)
				if err != nil {
					errChan <- err
					return
				}
				go copyByte(target, conn, errChan)
				copyByte(conn, target, errChan)
			} else {
				NewReply(RepNotAllow).SendTo(conn)
				errChan <- fmt.Errorf("bind request addr not match")
			}
		}()
		return errChan
	}
	errChan := accept()
	select {
	case <-ctx.Done():
		log.Println("ctx done: ", ctx.Err())
	case err := <-errChan:
		log.Println("accept error: ", err)
	}
}
