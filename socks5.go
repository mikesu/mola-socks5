package socks5

import (
	"io"
	"log"
	"net"
	"strings"
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

const bufSize = 4096

var associateMap *AssociateMap
var udpAddr *net.UDPAddr

func Run(address string) {
	log.Println("Run socks5 on: ", address)
	go serveTcp(address)
	serveUdp(address)
}

func serveTcp(address string) {
	listener, err := net.Listen("tcp", address)
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
		go serveTcpConn(conn)
	}
}

func serveUdp(address string) {
	associateMap = NewAssociateMap()
	var err error
	udpAddr, err = net.ResolveUDPAddr("udp", address)
	if err != nil {
		log.Println("resolve udp addr error: ", err)
		return
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Println("local tcp listen error: ", err)
		return
	}
	for {
		relay, err := GetRelay(udpConn)
		if err != nil {
			log.Println("get udp relay error: ", err)
			continue
		}
		associate := associateMap.GetAssociate(relay.From, relay.Address)
		if associate == nil {
			go serveRelay(udpConn, relay)
		} else {
			associate.DataChan <- relay.Data
		}
	}
}

func serveTcpConn(conn net.Conn) {
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
		connectHandle(conn, request)
	case CmdAssociate:
		associateHandle(conn)
	default:
		NewReply(RepCommandNotSupported).SendTo(conn)
		log.Println("command not support:", request.Command)
	}
}

func connectHandle(conn net.Conn, req *Request) {
	var ip net.IP
	if req.Address.Type() == AtypDomain {
		addr, err := net.ResolveIPAddr("ip", req.Address.Addr())
		if err != nil {
			log.Println("resolve ip error: ", err)
			NewReply(RepHostUnreachable).SendTo(conn)
			return
		}
		ip = addr.IP
	} else {
		ip = req.Address.IP()
	}
	target, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP: ip, Port: req.Address.Port()})
	if err != nil {
		msg := err.Error()
		resp := RepHostUnreachable
		if strings.Contains(msg, "refused") {
			resp = RepConnectionRefused
		} else if strings.Contains(msg, "network is unreachable") {
			resp = RepNetworkUnreachable
		}
		log.Println("dial error: ", err)
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
	exchangeData(target, conn)
}

func associateHandle(conn net.Conn) {
	reply := new(Reply)
	reply.Rep = RepSuccess
	reply.Address = ToAddress(udpAddr.IP, udpAddr.Port)
	err := reply.SendTo(conn)
	if err != nil {
		return
	}
	for {
		_, err := RcvMsg(conn)
		if err != nil {
			return
		}
	}
}

func serveRelay(conn *net.UDPConn, relay *Relay) {
	associate := new(Associate)
	associate.Src = relay.From
	if relay.Address.Type() == AtypDomain {
		associate.Domain = relay.Address
	} else {
		associate.Dst = relay.Address
	}
	associate.DataChan = make(chan []byte, 50)
	associateMap.PutAssociate(associate)
	var ip net.IP
	if relay.Address.Type() == AtypDomain {
		addr, err := net.ResolveIPAddr("ip", relay.Address.Addr())
		if err != nil {
			log.Println("resolve ip error: ", err)
			return
		}
		ip = addr.IP
	} else {
		ip = relay.Address.IP()
	}
	associate.Dst = ToAddress(ip, relay.Address.Port())
	associateMap.PutAssociate(associate)
	target, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: ip, Port: relay.Address.Port()})
	if err != nil {
		return
	}
	defer target.Close()
	associate.DataChan <- relay.Data
	go func() {
		for {
			select {
			case data := <-associate.DataChan:
				_, err := target.Write(data)
				if err != nil {
					log.Println("send udp to server error: ", err)
					break
				}
			}
		}
	}()
	for {
		data, err := RcvMsg(target)
		if err != nil {
			log.Println("read udp error: ", err)
			break
		}
		relay := new(Relay)
		relay.Address = associate.Dst
		relay.Data = data
		_, err = conn.WriteToUDP(relay.GetBytes(), associate.Src)
		if err != nil {
			log.Println("send udp to Client error: ", err)
			break
		}
	}

}

func RcvMsg(conn net.Conn) ([]byte, error) {
	buf := make([]byte, 261)
	size, err := conn.Read(buf)
	if err != nil {
		log.Println("read msg error: ", err)
		return nil, err
	}
	return buf[:size], nil
}

func exchangeData(peerStream net.Conn, localConn net.Conn) {
	defer localConn.Close()
	defer peerStream.Close()
	go func() {
		defer localConn.Close()
		defer peerStream.Close()
		_, err := byteCopy(peerStream, localConn)
		log.Println(localConn.RemoteAddr(), " =>", peerStream.RemoteAddr(), "exchangeData exit:", err)
	}()
	_, err := byteCopy(localConn, peerStream)
	log.Println(peerStream.RemoteAddr(), " =>", localConn.RemoteAddr(), "exchangeData exit:", err)
}

// Memory optimized io.Copy function specified for this library
func byteCopy(dst io.Writer, src io.Reader) (written int64, err error) {
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}

	// fallback to standard io.CopyBuffer
	buf := make([]byte, bufSize)
	return io.CopyBuffer(dst, src, buf)
}
