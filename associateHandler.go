package socks5

import (
	"context"
	"log"
	"net"
	"strconv"
	"sync"
)

var associateMap *AssociateMap
var udpAddr *net.UDPAddr

type AssociateLink struct {
	Src      *net.UDPAddr
	Dst      Address
	Domain   Address
	DataChan chan []byte
}

type AssociateContext struct {
	ctx      context.Context
	cancel   func()
	replaced bool
	ip       net.IP
	port     int
}

type AssociateMap struct {
	assLock sync.Mutex
	linkMap map[string]*AssociateLink
	ctxLock sync.Mutex
	ctxMap  map[string]*AssociateContext
}

func NewAssociateMap() *AssociateMap {
	assMap := new(AssociateMap)
	assMap.linkMap = make(map[string]*AssociateLink)
	assMap.ctxMap = make(map[string]*AssociateContext)
	return assMap
}

func (am *AssociateMap) PutContext(assCtx *AssociateContext) {
	key := assCtx.ip.String() + ":" + strconv.Itoa(assCtx.port)
	am.ctxLock.Lock()
	defer am.ctxLock.Unlock()
	am.ctxMap[key] = assCtx
}

func (am *AssociateMap) RemoveContext(ip net.IP, port int) *AssociateContext {
	key := ip.String() + ":" + strconv.Itoa(port)
	ctx, ok := am.ctxMap[key]
	if ok {
		am.ctxLock.Lock()
		delete(am.ctxMap, key)
		am.ctxLock.Unlock()
		return ctx
	} else {
		return nil
	}
}

func (am *AssociateMap) GetContext(ip net.IP, port int) *AssociateContext {
	key := ip.String() + ":" + strconv.Itoa(port)
	assCtx, ok := am.ctxMap[key]
	if ok {
		return assCtx
	} else if port != 0 {
		return am.GetContext(ip, 0)
	} else {
		return nil
	}
}

func (am *AssociateMap) PutLink(ass *AssociateLink) {
	am.assLock.Lock()
	defer am.assLock.Unlock()
	src := ass.Src.String()
	if ass.Dst != nil {
		am.linkMap[src+ass.Dst.String()] = ass
	}
	if ass.Domain != nil {
		am.linkMap[src+ass.Domain.String()] = ass
	}
}

func (am *AssociateMap) RemoveLink(src *net.UDPAddr, dst Address) *AssociateLink {
	ass := am.GetLink(src, dst)
	if ass == nil {
		return nil
	} else {
		am.assLock.Lock()
		if ass.Dst != nil {
			delete(am.linkMap, src.String()+ass.Dst.String())
		}
		if ass.Domain != nil {
			delete(am.linkMap, src.String()+ass.Domain.String())
		}
		am.assLock.Unlock()
		return ass
	}
}

func (am *AssociateMap) GetLink(src *net.UDPAddr, dst Address) *AssociateLink {
	key := src.String() + dst.String()
	ass, ok := am.linkMap[key]
	if ok {
		return ass
	} else {
		return nil
	}
}

func associateHandle(ctx context.Context, conn net.Conn, request *Request) {
	log.Println("associateHandle", request.Address.String())
	ip, port, err := request.Address.ResolveIPAddr()
	if err != nil {
		NewReply(RepAddrTypeNotSupported).SendTo(conn)
		log.Println("asscociate addr error")
		return
	}
	if ip.Equal(net.IPv4zero) || ip.Equal(net.IPv6zero) {
		ip = conn.RemoteAddr().(*net.TCPAddr).IP
	}
	assCtx := associateMap.GetContext(ip, port)
	if assCtx != nil {
		assCtx.replaced = true
		assCtx.cancel()
	}
	newCtx, cancel := context.WithCancel(ctx)
	assCtx = &AssociateContext{ctx: newCtx, cancel: cancel, replaced: false, ip: ip, port: port}
	associateMap.PutContext(assCtx)

	reply := new(Reply)
	reply.Rep = RepSuccess
	reply.Address = ToAddress(udpAddr.IP, udpAddr.Port)
	err = reply.SendTo(conn)
	if err != nil {
		log.Println("send reply error", err)
		return
	}
	checkConn := func() <-chan error {
		errChan := make(chan error, 2)
		go func() {
			buf := make([]byte, msgMaxSize)
			for {
				_, err := conn.Read(buf)
				if err != nil {
					errChan <- err
				}
			}
		}()
		return errChan
	}
	errChan := checkConn()
	select {
	case <-newCtx.Done():
		log.Println("ctx done : ", newCtx.Err())
		if assCtx.replaced == false {
			associateMap.RemoveContext(ip, port)
		}
	case err = <-errChan:
		log.Println("checkConn error: ", err)
		associateMap.RemoveContext(ip, port)
	}
}

func serveUdp(ctx context.Context) {
	associateMap = NewAssociateMap()
	var err error
	udpAddr, err = net.ResolveUDPAddr("udp", listenAddr)
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
		log.Println("get udp relay : ", relay.Address.String())
		associate := associateMap.GetLink(relay.From, relay.Address)
		if associate == nil {
			go serveRelay(udpConn, relay)
		} else {
			associate.DataChan <- relay.Data
		}
	}
}

func serveRelay(udpConn *net.UDPConn, relay *Relay) {
	assCtx := associateMap.GetContext(relay.From.IP, relay.From.Port)
	if assCtx == nil {
		return
	}
	assLink := new(AssociateLink)
	assLink.Src = relay.From
	if relay.Address.Type() == AtypDomain {
		assLink.Domain = relay.Address
	} else {
		assLink.Dst = relay.Address
	}
	assLink.DataChan = make(chan []byte, 50)
	associateMap.PutLink(assLink)
	var ip net.IP
	if relay.Address.Type() == AtypDomain {
		addr, err := net.ResolveIPAddr("ip", relay.Address.Addr())
		if err != nil {
			log.Println("resolve ip error: ", err)
			return
		}
		ip = addr.IP
		assLink.Dst = ToAddress(ip, relay.Address.Port())
		associateMap.PutLink(assLink)
	} else {
		ip = relay.Address.IP()
	}
	target, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: ip, Port: relay.Address.Port()})
	if err != nil {
		return
	}
	defer target.Close()
	assLink.DataChan <- relay.Data

	rcvData := func() (<-chan []byte, <-chan error) {
		errChan := make(chan error, 2)
		dataChan := make(chan []byte, 2)
		go func() {
			for {
				buf := make([]byte, msgMaxSize)
				_, err := target.Read(buf)
				if err != nil {
					errChan <- err
				} else {
					dataChan <- buf
				}
			}
		}()
		return dataChan, errChan
	}
	dataChan, errChan := rcvData()
	for {
		select {
		case <-assCtx.ctx.Done():
			return
		case err := <-errChan:
			log.Println("", err)
			return
		case data := <-assLink.DataChan:
			_, err := target.Write(data)
			if err != nil {
				log.Println("send udp to server error: ", err)
				return
			}
		case data := <-dataChan:
			relay := new(Relay)
			relay.Address = assLink.Dst
			relay.Data = data
			_, err = udpConn.WriteToUDP(relay.GetBytes(), assLink.Src)
			if err != nil {
				log.Println("send udp to Client error: ", err)
				return
			}
		}
	}

}
