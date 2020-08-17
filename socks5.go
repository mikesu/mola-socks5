package socks5

import (
	"context"
	"io"
	"log"
	"net"
	"time"
)

const (
	Version = uint8(5)
	RSV     = uint8(0)
	FRAG    = uint8(0)

	MethodNoAcceptable = uint8(255)
	MethodNoAuth       = uint8(0)

	CmdConnect   = uint8(1)
	CmdBind      = uint8(2)
	CmdAssociate = uint8(3)

	AtypIpv4   = uint8(1)
	AtypDomain = uint8(3)
	AtypIpv6   = uint8(4)
)

const (
	RepSuccess uint8 = iota
	RepServerFailure
	RepNotAllow
	RepNetworkUnreachable
	RepHostUnreachable
	RepConnectionRefused
	RepTtlExpired
	RepCommandNotSupported
	RepAddrTypeNotSupported
	RepNotSupported
)

const msgMaxSize = 262
const bufSize = 4096

var listenAddr string

func Run(addr string) {
	log.Println("Run socks5 on: ", listenAddr)
	listenAddr = addr
	go serveUdp(context.Background())
	serveTcp(context.Background())
}

func serveTcp(ctx context.Context) {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Println("local tcp listen error: ", err)
		return
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("local accept error: ", err)
			continue
		}
		go serveTcpConn(ctx, conn)
	}
}

func serveTcpConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	err := Negotiation(conn)
	if err != nil {
		log.Println("negotiation error: ", err)
		return
	}
	request, err := GetRequest(conn)
	if err != nil {
		log.Println("get request error: ", err)
		return
	}
	switch request.Command {
	case CmdConnect:
		connectHandle(ctx, conn, request)
	case CmdAssociate:
		associateHandle(ctx, conn, request)
	default:
		NewReply(RepCommandNotSupported).SendTo(conn)
		log.Println("command not support:", request.Command)
	}
}

func WriteMsg(conn net.Conn, msg []byte) error {
	err := conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err != nil {
		log.Println("write msg SetDeadline error: ", err)
		return err
	}
	_, err = conn.Write(msg)
	if err != nil {
		log.Println("write msg error: ", err)
		return err
	}
	err = conn.SetDeadline(time.Time{})
	if err != nil {
		log.Println("read msg cancel SetDeadline error: ", err)
		return err
	}
	return nil
}

func RcvMsg(conn net.Conn) ([]byte, error) {
	buf := make([]byte, msgMaxSize)
	err := conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		log.Println("read msg SetDeadline error: ", err)
		return nil, err
	}
	size, err := conn.Read(buf)
	if err != nil {
		log.Println("read msg error: ", err)
		return nil, err
	}
	err = conn.SetDeadline(time.Time{})
	if err != nil {
		log.Println("read msg cancel SetDeadline error: ", err)
		return nil, err
	}
	return buf[:size], nil
}

func exchangeData(targetConn net.Conn, localConn net.Conn) chan error {
	errChan := make(chan error, 2)
	go copyByte(targetConn, localConn, errChan)
	go copyByte(localConn, targetConn, errChan)
	return errChan
}

// Memory optimized io.Copy function specified for this library
func copyByte(dst io.Writer, src io.Reader, errChan chan<- error) {
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		_, err := wt.WriteTo(dst)
		if err != nil {
			errChan <- err
		}
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		_, err := rt.ReadFrom(src)
		if err != nil {
			errChan <- err
		}
	}

	// fallback to standard io.CopyBuffer
	buf := make([]byte, bufSize)
	_, err := io.CopyBuffer(dst, src, buf)
	if err != nil {
		errChan <- err
	}
}
