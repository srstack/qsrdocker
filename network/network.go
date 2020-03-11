package network

import (
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"qsrdocker/container"
	"runtime"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

var (
	// NetworkDriverMap 网络驱动
	NetworkDriverMap = map[string]networkDriver{
		"none":      nil, // none 网络
		"host":      nil,
		"container": nil,
		"bridge":    &BridgeNetworkDriver{},
	}
)

// networkDriver 网络 driver 接口  Host None Container Bridge
type networkDriver interface {
	// 驱动名称
	Name() string
	// 创建目标驱动的网络
	Create(subnet string, networkID string) (*container.Network, error)
	// 删除目标驱动的网络
	Delete(network *container.Network) error
	// 连接网络端点EndPoint到网络
	Connect(network *container.Network, endpoint *container.Endpoint) error
	// 断开网络端点EndPoint到网络
	Disconnect(endpoint *container.Endpoint) error
}

// CreateNetwork 创建网络
func CreateNetwork(driver, subnet, networkID string) error {

	// 判断 driver 是否存在
	if _, exists := NetworkDriverMap[strings.ToLower(driver)]; !exists {
		return fmt.Errorf("Driver %v is not match", driver)
	}

	// 初始化 iptables
	if err := IPtablesInit(); err != nil {
		return fmt.Errorf("Can't Init iptables error : %v", err)
	}

	// 判断网络ID是否已经存在
	nwFilePath := path.Join(container.NetFileDir, strings.Join([]string{networkID, ".json"}, ""))
	if exists, _ := container.PathExists(nwFilePath); exists {
		return fmt.Errorf("Network Name %v exists", networkID)
	}

	// 讲网段字符串转化为 net.IPNet 对象
	_, cidr, _ := net.ParseCIDR(subnet)

	log.Debugf("Get CIDR %v", cidr.String())

	// 创建目标网段
	if err := ipAllocator.Create(cidr); err != nil {
		return fmt.Errorf("Create Network error %v", err)
	}

	log.Debugf("Create network cidr %v  success", cidr.String())

	// 从 IP manager 获取 网关IP
	// 目标网段的第一个 IP
	gwIP, err := ipAllocator.Allocate(cidr)
	if err != nil {
		return err
	}

	log.Debugf("Get gate way ip %v in %v", gwIP.String(), cidr.String())

	// 讲网关IP设置为 网段 默认 IP  cidr.IP
	cidr.IP = gwIP

	// 调用目标网络驱动的 create 方法创建网络
	nw, err := NetworkDriverMap[strings.ToLower(driver)].Create(cidr.String(), networkID)

	if err != nil {
		return err
	}

	return nw.Dump()
}

// DeleteNetwork 删除网络
func DeleteNetwork(networkID string) error {
	nw := &container.Network{
		ID: networkID,
	}

	if err := nw.Load(); err != nil {
		return fmt.Errorf("Get NetWork %v Info err: %v", networkID, err)
	}

	gwip := net.ParseIP(nw.GateWayIP)

	// 回收 IP 网段全部 地址
	if err := ipAllocator.Release(nw.IPRange, &gwip); err != nil {
		return fmt.Errorf("Remove Network %v Gateway ip %v error: %v", networkID, nw.IPRange.IP, err)
	}

	// 执行 网络驱动 删除
	if err := NetworkDriverMap[strings.ToLower(nw.Driver)].Delete(nw); err != nil {
		return fmt.Errorf("Remove Network %v Driver error: %v", networkID, err)
	}

	log.Debugf("Del network %v success", networkID)

	// 删除配置文件
	return nw.Remove()
}

// Connect 连接容器和已创建网络
func Connect(networkID, netDriver string, portSlice []string, containerInfo *container.ContainerInfo) error {
	if networkID == "" {
		containerInfo.NetWorks = &container.Endpoint{
			ID:      fmt.Sprintf("%s-%s", containerInfo.ID, netDriver),
			Network: &container.Network{Driver: netDriver},
		}
		return nil
	}

	nw := &container.Network{
		ID: networkID,
	}

	if err := nw.Load(); err != nil {
		return fmt.Errorf("Get NetWork %v Info err: %v", networkID, err)
	}

	// 分配容器IP地址
	ip, err := ipAllocator.Allocate(nw.IPRange)
	if err != nil {
		return err
	}

	// 创建网络端点
	ep := &container.Endpoint{
		ID:        fmt.Sprintf("%s-%s", containerInfo.ID, networkID),
		IPAddress: ip,
		Network:   nw,
	}

	// 解析 Ports
	// hostPort:containerPort、ip:hostPort:containerPort
	// [80:80, 127.1.2.3:3306:3306]
	// portSlice 存在可能是 start 操作
	if portSlice != nil {
		ports := map[string][]*container.Port{}

		for _, portPair := range portSlice {
			// 按照 ： 拆分
			portPairSlice := strings.Split(portPair, ":")
			portPairSlice = container.RemoveNullSliceString(portPairSlice)
			port := &container.Port{}

			// 根据长度判断 host ip
			// 80:80
			if len(portPairSlice) == 2 {
				port.HostIP = "0.0.0.0"
				port.HostPort = portPairSlice[0]
				if !strings.Contains(portPairSlice[1], "/") {
					portPairSlice[1] = fmt.Sprintf("%s/tcp", portPairSlice[1])
				}
				ports[portPairSlice[1]] = append(ports[portPairSlice[1]], port)
			}

			// 127.1.2.3:3306:3306
			if len(portPairSlice) == 3 {
				port.HostIP = portPairSlice[0]
				port.HostPort = portPairSlice[1]
				if !strings.Contains(portPairSlice[2], "/") {
					portPairSlice[2] = fmt.Sprintf("%s/tcp", portPairSlice[2])
				}
				ports[portPairSlice[2]] = append(ports[portPairSlice[2]], port)
			}
		}

		// 端口映射 map
		ep.Ports = ports
	} else {
		// 若是 start 操作 ，则继承已经解析好的 ports
		ep.Ports = containerInfo.NetWorks.Ports
	}

	containerInfo.NetWorks = ep

	// 调用网络驱动挂载和配置网络端点
	if err = NetworkDriverMap[strings.ToLower(nw.Driver)].Connect(nw, containerInfo.NetWorks); err != nil {
		return err
	}

	// 到容器的namespace配置容器网络设备IP地址
	if err = configEndpointIPAddressAndRoute(containerInfo); err != nil {
		return err
	}

	// 利用 IP tables 配置主机和容器的端口映射
	return configPortMapping(containerInfo)
}

// Disconnect 解除容器和已创建网络的连接
func Disconnect(networkID string, containerInfo *container.ContainerInfo) error {

	// 不为 bridge 网络 直接返回
	if strings.ToLower(containerInfo.NetWorks.Network.Driver) != "bridge" {
		return nil
	}

	// 调用网络驱动 删除连接
	if err := NetworkDriverMap[strings.ToLower(containerInfo.NetWorks.Network.Driver)].Disconnect(containerInfo.NetWorks); err != nil {
		return err
	}
	// 释放 ip
	if err := ipAllocator.Release(containerInfo.NetWorks.Network.IPRange, &containerInfo.NetWorks.IPAddress); err != nil {
		return fmt.Errorf("Remove Network %v ip %v error: %v", networkID, containerInfo.NetWorks.IPAddressStr, err)
	}

	log.Debugf("Release ip %v success", containerInfo.NetWorks.IPAddressStr)

	// 情况容器网络状态
	// 暂时不请客 容器 网络状态
	// containerInfo.NetWorks = &container.Endpoint{}

	return delPortMapping(containerInfo)
}

// enterContainerNetNs 进入容器 NET NS
func enterContainerNetNs(enLink *netlink.Link, containerInfo *container.ContainerInfo) func() {

	// 获取容器进程 PID NS 信息
	f, err := os.OpenFile(fmt.Sprintf("/proc/%d/ns/net", containerInfo.Status.Pid), os.O_RDONLY, 0)
	if err != nil {
		log.Errorf("error get container net namespace, %v", err)
	}

	// 获取 NET NS 的文件描述符
	containerNetnsFD := f.Fd()

	// 锁定当前的线程
	// 防止下次 协程调度时 当前 goroutine 调度到其他线程
	// 绑定目标 M  （GMP模型）
	runtime.LockOSThread()

	// 修改 veth peer 另外一端移到容器的 namespace 中
	if err = netlink.LinkSetNsFd(*enLink, int(containerNetnsFD)); err != nil {
		log.Errorf("Set veth peer Link to container %v net ns error : %v", containerInfo.Name, err)
	}

	// 获取 host 网络 namespace
	// 便于设置完毕后回到 host 环境
	hostNetNs, err := netns.Get()
	if err != nil {
		log.Errorf("Get host Net NS error : %v", err)
	}

	// 设置当前进程到新的网络namespace，并在函数执行完成之后再恢复到之前的namespace
	if err = netns.Set(netns.NsHandle(containerNetnsFD)); err != nil {
		log.Errorf("In to Container Net NS error : %v", err)
	}

	// 执行完后，回到原来的 Net NS
	// defer 也可以
	return func() {
		netns.Set(hostNetNs)
		hostNetNs.Close()
		// 解除绑定
		runtime.UnlockOSThread()
		f.Close()
	}
}

// configEndpointIPAddressAndRoute 配置容器网络 endpoint 的IP地址和路由
func configEndpointIPAddressAndRoute(containerInfo *container.ContainerInfo) error {

	// 获取容器网络端点
	// driver.create 中配置的
	vethPeerLink, err := netlink.LinkByName(containerInfo.NetWorks.Device.PeerName)
	if err != nil {
		return fmt.Errorf("fail config endpoint: %v", err)
	}

	// 进入容器 Net NS 中，将 peerlink 挂载
	// 执行完毕后恢复到 host Net NS
	defer enterContainerNetNs(&vethPeerLink, containerInfo)()

	// 获取容器网络 IP 地址网段
	interfaceIP := containerInfo.NetWorks.Network.IPRange
	// 获取容器网络 IP 地址
	interfaceIP.IP = containerInfo.NetWorks.IPAddress

	// 设置容器内 veth peer 的 IP
	if err = setInterfaceIP(containerInfo.NetWorks.Device.PeerName, interfaceIP.String()); err != nil {
		return fmt.Errorf("%v,%s", containerInfo.NetWorks.Network, err)
	}

	// 开启容器内部的 veth peer
	if err = setInterfaceUP(containerInfo.NetWorks.Device.PeerName); err != nil {
		return err
	}

	// 开启容器内部的 环回口地址 127.0.0.1
	if err = setInterfaceUP("lo"); err != nil {
		return err
	}

	// 设置容器的所有对外访问地址 都通过 gwIP
	// route add -net 0.0.0.0/0 gw [containerInfo.NetWorks.IP.IP]
	_, cidr, _ := net.ParseCIDR("0.0.0.0/0")
	defaultRoute := &netlink.Route{
		LinkIndex: vethPeerLink.Attrs().Index,
		Gw:        net.ParseIP(containerInfo.NetWorks.Network.GateWayIP),
		Dst:       cidr,
	}

	// route add 命令
	if err = netlink.RouteAdd(defaultRoute); err != nil {
		return err
	}

	return nil
}

// setInterfaceUP 启用网口
func setInterfaceUP(bridgeID string) error {

	// 获取网络信息
	link, err := netlink.LinkByName(bridgeID)
	if err != nil {
		return fmt.Errorf("Get link by named %s error: %v", link.Attrs().Name, err)
	}

	// ip xxx up 启动网口(接口)
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("Set up interface %s  error : %v", bridgeID, err)
	}

	return nil
}

// setInterfaceIP Set the IP addr of a netlink interface 设置 veth peer IP
func setInterfaceIP(bridgeID string, rawIP string) error {

	// 设置重启次数
	retries := 3

	// 设置 link 连接 和 err
	var link netlink.Link
	var err error

	// 多次尝试 获取 目标网络信息
	for i := 1; i < retries; i++ {
		// 获取网络信息
		link, err = netlink.LinkByName(bridgeID)
		if err == nil {
			break
		}
		log.Debugf("Get New Bridge link %s fail，retry time %v", bridgeID, i)
		// 重试等待时间
		time.Sleep(1 * time.Second)
	}

	if err != nil {
		return fmt.Errorf("Get New Bridge link %v error: %v", bridgeID, err)
	}

	ipNet, err := netlink.ParseIPNet(rawIP)
	if err != nil {
		return fmt.Errorf("Get ipNet form rawIP error: %v", err)
	}

	// 设置IP
	addr := &netlink.Addr{IPNet: ipNet, Peer: ipNet, Label: "", Flags: 0, Scope: 0, Broadcast: nil}

	// 在 host 主机上执行 ip add
	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("Set ip add error: %v", err)
	}

	return nil
}

// InitNetwork 初始化 已经创建的网络
func InitNetwork() {

	// 判断默认网络是否已经存在
	DefaultNetworkFilePath := path.Join(container.NetFileDir, strings.Join([]string{container.DefaultNetworkID, ".json"}, ""))

	// 若默认网络不存在则创建
	if exists, _ := container.PathExists(DefaultNetworkFilePath); !exists {
		// 若未创建默认网络, 则创建
		err := CreateNetwork(container.DefaultNetworkDriver, container.DefaultNetworkSubnet, container.DefaultNetworkID)
		if err != nil {
			log.Errorf("Create default network %v error: %v", container.DefaultNetworkID, err)
		}
	}

	// 全部 已创建 network 信息
	networks := []*container.Network{}

	// 获取网络配置数据
	filepath.Walk(
		container.NetFileDir, func(nwPath string, info os.FileInfo, err error) error {
			if !strings.HasSuffix(nwPath, ".json") {
				return nil
			}

			// nwID   nwID.json
			_, nwID := path.Split(nwPath)

			nwID = nwID[0:(len(nwID) - 5)]
			nw := &container.Network{
				ID: nwID,
			}

			if err := nw.Load(); err != nil {
				log.Errorf("Load network Config %v error : %v", nwID, err)
			}

			networks = append(networks, nw)
			return nil
		},
	)

	for _, nw := range networks {

		// 判断网络是否已正常
		_, err := net.InterfaceByName(nw.ID)

		// no such network interface 报错，才是表明未创建目标 bridge 网络
		if err == nil || !strings.Contains(err.Error(), "no such network interface") {
			continue
		}

		// 调用目标网络驱动的 create 方法恢复网络
		nw, err := NetworkDriverMap[strings.ToLower(nw.Driver)].Create(nw.IPRangeString, nw.ID)

		if err != nil {
			log.Error("Restore network %v error %v", nw.ID, err)
		}

		nw.Dump()
	}
}
