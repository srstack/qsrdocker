package network

import (
	"net"
	"testing"
)

func TestCreate(t *testing.T) {
	_, ipnet, _ := net.ParseCIDR("192.168.0.0/24")
	ip, _ := ipAllocator.Allocate(ipnet)
	t.Logf("get way ip: %v", ip.String())
}

func TestAllocate(t *testing.T) {
	_, ipnet, _ := net.ParseCIDR("192.168.0.0/24")
	ip, _ := ipAllocator.Allocate(ipnet)
	t.Logf("alloc ip: %v", ip.String())
}

func TestRelease(t *testing.T) {
	ip, ipnet, _ := net.ParseCIDR("192.168.0.1/24")
	ipAllocator.Release(ipnet, &ip)
	t.Logf("release ip: %v", ip.String())
}
