package socks5

import (
	"fmt"
	"log"
	"net"
	"strconv"
)

type Address []byte

func ToAddress(ip net.IP, port int) Address {
	var addr Address
	if ip.To4() != nil {
		addr = []byte{AtypIpv4}
	} else {
		addr = []byte{AtypIpv6}
	}
	addr = append(addr, ip...)
	addr = append(addr, uint8(port>>8), uint8(port&255))
	return addr
}

func GetAddress(bytes []byte) (Address, error) {
	size := 0
	switch bytes[0] {
	case AtypIpv4:
		size = 3 + net.IPv4len
	case AtypIpv6:
		size = 3 + net.IPv6len
	case AtypDomain:
		size = 4 + int(bytes[1])
	default:
		return nil, fmt.Errorf("unsupported address type: %v", bytes[0])
	}
	if len(bytes) < size {
		return nil, fmt.Errorf("address length is too short")
	}
	return bytes[:size], nil
}

func (a Address) String() string {
	return a.Addr() + ":" + strconv.Itoa(a.Port())
}

func (a Address) IP() net.IP {
	switch a[0] {
	case AtypIpv4:
		return net.IP(a[1 : net.IPv4len+1])
	case AtypIpv6:
		return net.IP(a[1 : net.IPv6len+1])
	}
	return nil
}

func (a Address) Addr() string {
	switch a[0] {
	case AtypIpv4:
		return net.IP(a[1 : net.IPv4len+1]).String()
	case AtypIpv6:
		return net.IP(a[1 : net.IPv6len+1]).String()
	case AtypDomain:
		return string(a[2 : a[1]+2])
	}
	return ""
}

func (a Address) Port() int {
	index := 0
	switch a[0] {
	case AtypIpv4:
		index = net.IPv4len + 1
	case AtypIpv6:
		index = net.IPv6len + 1
	case AtypDomain:
		index = int(a[1]) + 2
	}
	return (int(a[index]) << 8) | int(a[index+1])
}

func (a Address) Type() uint8 {
	return a[0]
}

func (a Address) ResolveIPAddr() (net.IP, int, error) {
	if a[0] == AtypDomain {
		addr, err := net.ResolveIPAddr("ip", a.Addr())
		if err != nil {
			log.Println("resolve ip error: ", err)
			return nil, 0, err
		}
		return addr.IP, a.Port(), nil
	} else {
		return a.IP(), a.Port(), nil
	}
}
