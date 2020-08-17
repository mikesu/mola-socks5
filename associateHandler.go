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
	ctx, ok := am.ctxMap[key]
	if ok {
		return ctx
	} else {
		return nil
	}
}

func (am *AssociateMap) PutAssociate(ass *AssociateLink) {
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

func (am *AssociateMap) RemoveAssociate(src *net.UDPAddr, dst Address) *AssociateLink {
	ass := am.GetAssociate(src, dst)
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

func (am *AssociateMap) GetAssociate(src *net.UDPAddr, dst Address) *AssociateLink {
	key := src.String() + dst.String()
	ass, ok := am.linkMap[key]
	if ok {
		return ass
	} else {
		return nil
	}
}

func associateHandle(ctx context.Context, conn net.Conn, request *Request) {
	ip, port, err := request.Address.ResolveIPAddr()
	if err != nil {
		NewReply(RepAddrTypeNotSupported).SendTo(conn)
		log.Println("asscociate addr error")
		return
	}
	if ip.Equal(net.IPv4zero) || ip.Equal(net.IPv6zero) {
		ip = conn.LocalAddr().(*net.TCPAddr).IP
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
		return
	}
	checkConn := func() <-chan error {
		errChan := make(chan error, 2)
		go func() {
			for {
				buf := make([]byte, msgMaxSize)
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
		if assCtx.replaced == false {
			associateMap.RemoveContext(ip, port)
		}
	case err = <-errChan:
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
		associate := associateMap.GetAssociate(relay.From, relay.Address)
		if associate == nil {
			go serveRelay(udpConn, relay)
		} else {
			associate.DataChan <- relay.Data
		}
	}
}

func serveRelay(conn *net.UDPConn, relay *Relay) {
	associate := new(AssociateLink)
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
		associate.Dst = ToAddress(ip, relay.Address.Port())
		associateMap.PutAssociate(associate)
	} else {
		ip = relay.Address.IP()
	}
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
