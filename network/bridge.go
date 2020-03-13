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
		return nil, fmt.Errorf("Create Bridge Network %v error %v", networkID, err)
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

	err = delIPTables(network.ID, network.IPRange)

	if err != nil {
		return fmt.Errorf("Del iptables error %v", err)
	}

	log.Debugf("Del iptables success")

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

	bridgeLinkAttr.Name = strings.Join([]string{"qsrveth", endpoint.ID[:5]}, "")
	endpoint.VethName = strings.Join([]string{"qsrveth", endpoint.ID[:5]}, "")

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

	// 设置目标 mac 地址
	endpoint.MacAddress = endpoint.Device.HardwareAddr

	return nil
}

// Disconnect 接触和 目标 Bridge 网络的连接
func (bridge *BridgeNetworkDriver) Disconnect(endpoint *container.Endpoint) error {

	// 删除 容器 bridge link 连接
	err := netlink.LinkDel(&endpoint.Device)

	if err != nil {
		log.Warnf("Del Bridge link %v error %v", endpoint.ID, err)
	}

	log.Debugf("Del bridge network connnet success")

	return nil
}

// initBridge 初始化 bridge
func (bridge *BridgeNetworkDriver) initBridge(network *container.Network) error {

	// 获取已经设置好的ID信息
	bridgeID := network.ID

	// 创建 bridge 网络接口
	if err := createBridgeInterface(bridgeID); err != nil {
		return fmt.Errorf("Create Bridge Interface %s error: %v", bridgeID, err)
	}

	// 设置 网关 网段 IP 信息
	gatewayIP := *network.IPRange
	gatewayIP.IP = net.ParseIP(network.GateWayIP)

	log.Debug("Get gate way ip %v", gatewayIP.IP.String())

	// 在 host os 上  ip set [interface]
	if err := setInterfaceIP(bridgeID, gatewayIP.String()); err != nil {
		return fmt.Errorf("Set IP Interface %s on Bridge Net %s error %v", gatewayIP.IP.String(), bridgeID, err)
	}

	log.Debugf("Set ip add success with %v", bridgeID)

	// 在 host os 上 set up Bridge 接口
	if err := setInterfaceUP(bridgeID); err != nil {
		return fmt.Errorf("Set Bridge up %s error: %v", bridgeID, err)
	}

	log.Debugf("Set ip up success with %v", bridgeID)

	// 创建 snat
	// 即 所有从 Bridge 出方向流量的 ip source 都设置为 bridge 网络
	if err := setIPTables(bridgeID, network.IPRange); err != nil {
		return fmt.Errorf("Set iptables for Bridge Net %s error %v", bridgeID, err)
	}

	log.Debugf("Set iptables success with %v", bridgeID)

	return nil
}

// createBridgeInterface 创建 Bridge
func createBridgeInterface(bridgeID string) error {

	// 判断目标 bridge 网络是否已经创建
	_, err := net.InterfaceByName(bridgeID)

	// 存在 其他报错的可能性
	// 只有 no such network interface 报错，才是表明未创建目标 bridge 网络
	if err == nil || !strings.Contains(err.Error(), "no such network interface") {
		return err
	}

	// 创建网络 link 配置
	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = bridgeID

	// 创建 Bridge 网络 Link
	bridgeLink := &netlink.Bridge{LinkAttrs: linkAttrs}

	// link add 在  host os 中 添加 目标 Bridge 网络 Link
	if err := netlink.LinkAdd(bridgeLink); err != nil {
		return fmt.Errorf("Create Bridge %v Link failed error %v", bridgeID, err)
	}
	return nil
}
