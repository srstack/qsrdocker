package network

import (
	"fmt"
	"net"
	"qsrdocker/container"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// BridgeNetworkDriver 基于 linux bridge 的网络驱动
type BridgeNetworkDriver struct {
}

// Name 返回网络驱动名
func (bridge *BridgeNetworkDriver) Name() string {
	return "Bridge"
}

// Create 创建网络驱动
func (bridge *BridgeNetworkDriver) Create(subnet string, networkID string) (*container.Network, error) {

	// 解析 网段信息
	// 得到 网段 和 网关IP
	gwip, ipRange, _ := net.ParseCIDR(subnet)
	ipRange.IP = gwip

	// 创建 network 结构体
	nw := &container.Network{
		ID:        networkID,
		IPRange:   ipRange,
		Driver:    bridge.Name(),
		GateWayIP: gwip.String(),
	}

	// 初始化 bridge 网络
	err := bridge.initBridge(nw)
	if err != nil {
		log.Errorf("Create Bridge Network %v error %v", networkID, err)
	}

	log.Debugf("Create Bridge Network %v success", networkID)

	return nw, err
}

// Delete 删除 Bridge 网络
func (bridge *BridgeNetworkDriver) Delete(network *container.Network) error {

	// 获取 link 连接结构
	bridgeLink, err := netlink.LinkByName(network.ID)

	if err != nil {
		return fmt.Errorf("Get Bridge link error %v", err)
	}

	// 删除目标 link
	return netlink.LinkDel(bridgeLink)
}

// Connect 连接到目标 Bridge 网络
func (bridge *BridgeNetworkDriver) Connect(network *container.Network, endpoint *container.Endpoint) error {

	// 获取 bridge link
	bridgeLink, err := netlink.LinkByName(network.ID)
	if err != nil {
		return fmt.Errorf("Get Bridge link error %v", err)
	}

	// 创建新的 连接属性  接口的配置
	// LinkAttrs represents data shared by most link types
	bridgeLinkAttr := netlink.NewLinkAttrs()

	// 接口名 去 endpint 的前五位
	// 即 container ID 的前五位
	bridgeLinkAttr.Name = endpoint.ID[:5]

	// 设置 veth配置的 master 属性，指向目标 bridge 网络
	// 即将另一端挂在 linux bridge 网络上
	bridgeLinkAttr.MasterIndex = bridgeLink.Attrs().Index

	// 设置 veth
	endpoint.Device = netlink.Veth{
		LinkAttrs: bridgeLinkAttr,
		PeerName:  fmt.Sprintf("bridge-%s", endpoint.ID[:5]),
	}

	// 调用 link add 方法，创建 link 连接
	// 在系统上完成创建
	if err = netlink.LinkAdd(&endpoint.Device); err != nil {
		return fmt.Errorf("Add Bridge Link %v error %v", endpoint.ID[:5], err)
	}

	// 调用 link set up 将上文中创建的 link 连接启动
	// ip set [link_id] up
	if err = netlink.LinkSetUp(&endpoint.Device); err != nil {
		return fmt.Errorf("Set Up Bridge Link %v error %v", endpoint.ID[:5], err)
	}
	return nil
}

// Disconnect 接触和 目标 Bridge 网络的连接
func (bridge *BridgeNetworkDriver) Disconnect(network *container.Network, endpoint *container.Endpoint) error {
	return nil
}

// initBridge 初始化 bridge
func (bridge *BridgeNetworkDriver) initBridge(network *container.Network) error {

	// 获取已经设置好的ID信息
	bridgeID := network.ID

	//chun
	if err := createBridgeInterface(bridgeID); err != nil {
		return fmt.Errorf("Error add bridge： %s, Error: %v", bridgeName, err)
	}

	// Set bridge IP
	gatewayIP := *n.IpRange
	gatewayIP.IP = n.IpRange.IP

	if err := setInterfaceIP(bridgeName, gatewayIP.String()); err != nil {
		return fmt.Errorf("Error assigning address: %s on bridge: %s with an error of: %v", gatewayIP, bridgeName, err)
	}

	if err := setInterfaceUP(bridgeName); err != nil {
		return fmt.Errorf("Error set bridge up: %s, Error: %v", bridgeName, err)
	}

	// Setup iptables
	if err := setupIPTables(bridgeName, n.IpRange); err != nil {
		return fmt.Errorf("Error setting iptables for %s: %v", bridgeName, err)
	}

	return nil
}

func createBridgeInterface(bridgeName string) error {
	_, err := net.InterfaceByName(bridgeName)
	if err == nil || !strings.Contains(err.Error(), "no such network interface") {
		return err
	}

	// create *netlink.Bridge object
	la := netlink.NewLinkAttrs()
	la.Name = bridgeName

	br := &netlink.Bridge{LinkAttrs: la}
	if err := netlink.LinkAdd(br); err != nil {
		return fmt.Errorf("Bridge creation failed for bridge %s: %v", bridgeName, err)
	}
	return nil
}
