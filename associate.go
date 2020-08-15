package socks5

import (
	"net"
	"sync"
)

type Associate struct {
	Src      *net.UDPAddr
	Dst      Address
	Domain   Address
	DataChan chan []byte
}

type AssociateMap struct {
	lock   sync.Mutex
	assMap map[string]*Associate
}

func NewAssociateMap() *AssociateMap {
	assMap := new(AssociateMap)
	assMap.assMap = make(map[string]*Associate)
	return assMap
}

func (am *AssociateMap) PutAssociate(ass *Associate) {
	am.lock.Lock()
	defer am.lock.Unlock()
	src := ass.Src.String()
	if ass.Dst != nil {
		am.assMap[src+ass.Dst.String()] = ass
	}
	if ass.Domain != nil {
		am.assMap[src+ass.Domain.String()] = ass
	}
}

func (am *AssociateMap) GetAssociate(src *net.UDPAddr, dst Address) *Associate {
	key := src.String() + dst.String()
	ass, ok := am.assMap[key]
	if ok {
		return ass
	} else {
		return nil
	}
}
