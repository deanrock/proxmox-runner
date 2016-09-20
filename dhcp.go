package main

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	dhcp "github.com/krolaw/dhcp4"
)

type lease struct {
	nic    string    // Client's CHAddr
	expiry time.Time // When the lease expires
}

type LeaseMap struct {
	sync.RWMutex
	m map[int]lease
}

type DHCPHandler struct {
	ip            net.IP        // Server IP to use
	options       dhcp.Options  // Options to send to DHCP Clients
	start         net.IP        // Start of IP range to distribute
	leaseRange    int           // Number of IPs to distribute (starting from start)
	leaseDuration time.Duration // Lease period
	leases        LeaseMap      // Map to keep track of leases
}

func (h *DHCPHandler) IPAddressForMAC(mac string) (string, error) {
	h.leases.RLock()
	defer h.leases.RUnlock()
	now := time.Now()

	for i, l := range h.leases.m {
		if l.nic == mac {
			if l.expiry.Before(now) {
				fmt.Println("lease", l, "is already expired")
				continue
			}
			return fmt.Sprintf("192.168.150.%d", i+2), nil
		}
	}

	return "", errors.New("Lease with specified MAC not found")
}

func (h *DHCPHandler) ServeDHCP(p dhcp.Packet, msgType dhcp.MessageType, options dhcp.Options) (d dhcp.Packet) {
	switch msgType {

	case dhcp.Discover:
		free, nic := -1, p.CHAddr().String()
		h.leases.RLock()
		defer h.leases.RUnlock()
		for i, v := range h.leases.m { // Find previous lease
			if v.nic == nic {
				free = i
				goto reply
			}
		}
		if free = h.freeLease(); free == -1 {
			return
		}
	reply:
		fmt.Println("=> DHCP offer")
		return dhcp.ReplyPacket(p, dhcp.Offer, h.ip, dhcp.IPAdd(h.start, free), h.leaseDuration,
			h.options.SelectOrderOrAll(options[dhcp.OptionParameterRequestList]))

	case dhcp.Request:
		if server, ok := options[dhcp.OptionServerIdentifier]; ok && !net.IP(server).Equal(h.ip) {
			return nil // Message not for this dhcp server
		}
		reqIP := net.IP(options[dhcp.OptionRequestedIPAddress])
		if reqIP == nil {
			reqIP = net.IP(p.CIAddr())
		}

		if len(reqIP) == 4 && !reqIP.Equal(net.IPv4zero) {
			if leaseNum := dhcp.IPRange(h.start, reqIP) - 1; leaseNum >= 0 && leaseNum < h.leaseRange {
				h.leases.Lock()
				defer h.leases.Unlock()
				if l, exists := h.leases.m[leaseNum]; !exists || l.nic == p.CHAddr().String() {
					h.leases.m[leaseNum] = lease{nic: p.CHAddr().String(), expiry: time.Now().Add(h.leaseDuration)}
					fmt.Println("=> DHCP lease num", leaseNum, "for interface", h.leases.m[leaseNum].nic)
					return dhcp.ReplyPacket(p, dhcp.ACK, h.ip, net.IP(options[dhcp.OptionRequestedIPAddress]), h.leaseDuration,
						h.options.SelectOrderOrAll(options[dhcp.OptionParameterRequestList]))
				}
			}
		}
		return dhcp.ReplyPacket(p, dhcp.NAK, h.ip, nil, 0, nil)

	case dhcp.Release, dhcp.Decline:
		nic := p.CHAddr().String()
		h.leases.Lock()
		defer h.leases.Unlock()
		for i, v := range h.leases.m {
			if v.nic == nic {
				delete(h.leases.m, i)
				break
			}
		}
	}
	return nil
}

func (h *DHCPHandler) freeLease() int {
	now := time.Now()
	b := rand.Intn(h.leaseRange) // Try random first
	for _, v := range [][]int{[]int{b, h.leaseRange}, []int{0, b}} {
		for i := v[0]; i < v[1]; i++ {
			if l, ok := h.leases.m[i]; !ok || l.expiry.Before(now) {
				return i
			}
		}
	}
	return -1
}
