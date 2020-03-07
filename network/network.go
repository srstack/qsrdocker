package network

import (
	"fmt"
	"net"
	"os"
	"os/exec"
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
		"none":   nil,
		"bridge": &BridgeNetworkDriver{},
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

	// 讲网段字符串转化为 net.IPNet 对象
	_, cidr, _ := net.ParseCIDR(subnet)

	// 创建目标网段
	if err := ipAllocator.Create(cidr); err != nil {
		return fmt.Errorf("Create Network error %v", err)
	}

	// 从 IP manager 获取 网关IP
	// 目标网段的第一个 IP
	gwIP, err := ipAllocator.Allocate(cidr)
	if err != nil {
		return err
	}

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

	// 删除配置文件
	return nw.Remove()
}

// Connect 连接容器和已创建网络
func Connect(networkID string, portSlice []string, containerInfo *container.ContainerInfo) error {

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

	// 解析 Ports
	// hostPort:containerPort、ip:hostPort:containerPort
	// [80:80, 127.1.2.3:3306:3306]
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

	// 创建网络端点
	ep := &container.Endpoint{
		ID:        fmt.Sprintf("%s-%s", containerInfo.ID, networkID),
		IPAddress: ip,
		Network:   nw,
		Ports:     ports,
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

	// 调用网络驱动 删除连接
	if err := NetworkDriverMap[strings.ToLower(containerInfo.NetWorks.Network.Driver)].Disconnect(containerInfo.NetWorks); err != nil {
		return err
	}

	// 情况容器网络状态
	containerInfo.NetWorks = &container.Endpoint{}

	return nil
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
		Gw:        containerInfo.NetWorks.IPAddress,
		Dst:       cidr,
	}

	// route add 命令
	if err = netlink.RouteAdd(defaultRoute); err != nil {
		return err
	}

	return nil
}

// configPortMapping 使用 iptables 完成 dnat 端口映射
func configPortMapping(containerInfo *container.ContainerInfo) error {

	// 获取 container IP 和 容器网络link信息
	containerIP := containerInfo.NetWorks.IPAddress.String()
	containerIPWithMask := fmt.Sprintf("%v/32", containerIP)
	linkID := containerInfo.NetWorks.Network.ID // bridgeID

	// 获取 portmap 信息
	for dnatInfo, portSlice := range containerInfo.NetWorks.Ports {
		// pm ===  {hostip: string , hostPort: string }
		// dnatInfo "443/tcp"

		// container port
		//dnatSlice ["443","tcp"]
		dnatSlice := strings.Split(dnatInfo, "/")

		if len(dnatSlice) != 2 {
			log.Errorf("Get port nat error, %v", dnatSlice)
			continue
		}

		containerPort := dnatSlice[0]
		protocol := strings.ToLower(dnatSlice[1])

		// 存在 1:n 端口映射
		for _, pm := range portSlice {
			// iptables dnat
			// -A QSRDOCKER -d [172.17.0.3/32] ! -i [qsrdocker0] -o [qsrdocker0] -p [tcp] -m [tcp] --dport [3306] -j ACCEPT
			PortMappingAcceptCmd := fmt.Sprintf(
				"-A QSRDOCKER -d %s ! -i %s -o %s -p %s -m %s --dport %s -j ACCEPT",
				containerIPWithMask, linkID, linkID, protocol, protocol, containerPort)
			// 执行 iptables
			_, err := exec.Command("iptables", strings.Split(PortMappingAcceptCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables set error %v", err)
				continue
			}

			// -A POSTROUTING -s 172.17.0.2/32 -d 172.17.0.2/32 -p tcp -m tcp --dport 3306 -j MASQUERADE
			PortMappingPostRoutingCmd := fmt.Sprintf(
				"-t nat -A POSTROUTING -s %v -d %v -p %v -m %v --dport %v -j MASQUERADE",
				containerIPWithMask, containerIPWithMask, protocol, protocol, containerPort)
			// 执行 iptables
			_, err = exec.Command("iptables", strings.Split(PortMappingPostRoutingCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables set error %v", err)
				continue
			}

			// -A DOCKER ! -i qsrdocker0 -p tcp -m tcp --dport 33060 -j DNAT --to-destination 172.17.0.2:3306
			PortMappingDNatCmd := fmt.Sprintf(
				"-t nat -A QSRDOCKER ! -i %v -p %v -m %v --dport %v -j DNAT --to-destination %v:%v",
				linkID, protocol, protocol, pm.HostPort, containerIP, containerPort)
			// 执行 iptables
			_, err = exec.Command("iptables", strings.Split(PortMappingDNatCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables set error %v", err)
				continue
			}

		}

	}
	return nil
}

// delPortMapping 删除 iptables 完成 dnat 端口映射
func delPortMapping(containerInfo *container.ContainerInfo) error {

	// 获取 container IP 和 容器网络link信息
	containerIP := containerInfo.NetWorks.IPAddress.String()
	containerIPWithMask := fmt.Sprintf("%v/32", containerIP)
	linkID := containerInfo.NetWorks.Network.ID // bridgeID

	// 获取 portmap 信息
	for dnatInfo, portSlice := range containerInfo.NetWorks.Ports {
		// pm ===  {hostip: string , hostPort: string }
		// dnatInfo "443/tcp"

		// container port
		//dnatSlice ["443","tcp"]
		dnatSlice := strings.Split(dnatInfo, "/")

		if len(dnatSlice) != 2 {
			log.Errorf("Get port nat error, %v", dnatSlice)
			continue
		}

		containerPort := dnatSlice[0]
		protocol := strings.ToLower(dnatSlice[1])

		// 存在 1:n 端口映射
		for _, pm := range portSlice {
			// iptables dnat
			// -D QSRDOCKER -d [172.17.0.3/32] ! -i [qsrdocker0] -o [qsrdocker0] -p [tcp] -m [tcp] --dport [3306] -j ACCEPT
			PortMappingAcceptCmd := fmt.Sprintf(
				"-D QSRDOCKER -d %s ! -i %s -o %s -p %s -m %s --dport %s -j ACCEPT",
				containerIPWithMask, linkID, linkID, protocol, protocol, containerPort)
			// 执行 iptables
			_, err := exec.Command("iptables", strings.Split(PortMappingAcceptCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables del error %v", err)
				continue
			}

			// -D POSTROUTING -s 172.17.0.2/32 -d 172.17.0.2/32 -p tcp -m tcp --dport 3306 -j MASQUERADE
			PortMappingPostRoutingCmd := fmt.Sprintf(
				"-t nat -D POSTROUTING -s %v -d %v -p %v -m %v --dport %v -j MASQUERADE",
				containerIPWithMask, containerIPWithMask, protocol, protocol, containerPort)
			// 执行 iptables
			_, err = exec.Command("iptables", strings.Split(PortMappingPostRoutingCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables del error %v", err)
				continue
			}

			// -D DOCKER ! -i qsrdocker0 -p tcp -m tcp --dport 33060 -j DNAT --to-destination 172.17.0.2:3306
			PortMappingDNatCmd := fmt.Sprintf(
				"-t nat -D QSRDOCKER ! -i %v -p %v -m %v --dport %v -j DNAT --to-destination %v:%v",
				linkID, protocol, protocol, pm.HostPort, containerIP, containerPort)
			// 执行 iptables
			_, err = exec.Command("iptables", strings.Split(PortMappingDNatCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables del error %v", err)
				continue
			}

		}

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
		return err
	}

	// 设置IP
	addr := &netlink.Addr{IPNet: ipNet, Peer: ipNet, Label: "", Flags: 0, Scope: 0, Broadcast: nil}

	// 在 host 主机上执行 ip add
	return netlink.AddrAdd(link, addr)
}

// setupIPTables 设置SNAT
// -A FORWARD -o qsrdocker0 -j QSRDOCKER
// -A FORWARD -o qsrdocker0 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
// -A FORWARD -i qsrdocker0 ! -o qsrdocker0 -j ACCEPT
// -A FORWARD -i qsrdocker0 -o qsrdocker0 -j ACCEPT
// -t nat -A POSTROUTING -s 172.17.0.0/16 ! -o docker0 -j MASQUERADE  (SNAT)
func setIPTables(bridgeID string, subnet *net.IPNet) error {

	// 设置转发链 QSRDOCKER Chain  cmd
	setChainCmd := fmt.Sprintf("-A FORWARD -o %v -j QSRDOCKER", bridgeID)
	// 直接运行 cmd 命令
	_, err := exec.Command("iptables", strings.Split(setChainCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	// 设置转发规则 conntrack
	setConntrackCmd := fmt.Sprintf("-A FORWARD -o %v -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT", bridgeID)
	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setConntrackCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	// 设置 非目标网络到目标网络 的 流量转发
	setForwardNotLocalCmd := fmt.Sprintf("-A FORWARD -i %v ! -o %v -j ACCEPT", bridgeID, bridgeID)
	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setForwardNotLocalCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	// 设置 非目标网络到目标网络 的 流量转发
	setForwardLocalCmd := fmt.Sprintf("-A FORWARD -i %v -o %v -j ACCEPT", bridgeID, bridgeID)
	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setForwardLocalCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	// 设置 SNAT
	setSnatCmd := fmt.Sprintf("-t nat -A POSTROUTING -s %v ! -o %v -j MASQUERADE", subnet.String(), bridgeID)
	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setSnatCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	// -t nat -A DOCKER -i docker0 -j RETURN
	setNatCmd := fmt.Sprintf("-t nat -A QSRDOCKER -i %v-j RETURN", bridgeID)
	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setNatCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	return nil
}

// delIPTables 删除网络 iptables 设置
func delIPTables(bridgeID string, subnet *net.IPNet) error {

	// 取消转发链 QSRDOCKER Chain  cmd
	setChainCmd := fmt.Sprintf("-D FORWARD -o %v -j QSRDOCKER", bridgeID)
	// 直接运行 cmd 命令
	_, err := exec.Command("iptables", strings.Split(setChainCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	// 取消转发规则 conntrack
	setConntrackCmd := fmt.Sprintf("-D FORWARD -o %v -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT", bridgeID)
	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setConntrackCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	// 取消非目标网络到目标网络 的 流量转发
	setForwardNotLocalCmd := fmt.Sprintf("-D FORWARD -i %v ! -o %v -j ACCEPT", bridgeID, bridgeID)
	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setForwardNotLocalCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	// 取消 非目标网络到目标网络 的 流量转发
	setForwardLocalCmd := fmt.Sprintf("-D FORWARD -i %v -o %v -j ACCEPT", bridgeID, bridgeID)
	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setForwardLocalCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	// 取消 SNAT
	setSnatCmd := fmt.Sprintf("-t nat -D POSTROUTING -s %v ! -o %v -j MASQUERADE", subnet.String(), bridgeID)
	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setSnatCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	// -t nat -A DOCKER -i docker0 -j RETURN
	setNatCmd := fmt.Sprintf("-t nat -D QSRDOCKER -i %v-j RETURN", bridgeID)
	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setNatCmd, " ")...).CombinedOutput()

	if err != nil {
		return err
	}

	return nil
}

// IPtablesInit 初始化容器iptables
// -nat -A PREROUTING -m addrtype --dst-type LOCAL -j DOCKER
// -nat -A OUTPUT ! -d 127.0.0.0/8 -m addrtype --dst-type LOCAL -j DOCKER
func IPtablesInit() error {

	// 创建新链
	newChainCmd := "-N QSRDOCKER"

	// 直接运行 cmd 命令
	_, err := exec.Command("iptables", strings.Split(newChainCmd, " ")...).CombinedOutput()

	// 报错不为空
	if err != nil {
		// 报错信息为  Chain already exists 表示已经创建过 直接返回
		if strings.Contains(err.Error(), "Chain already exists") {
			return nil
		}
		return err
	}

	// 创建新链
	newNatChainCmd := "-t nat -N QSRDOCKER"

	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(newNatChainCmd, " ")...).CombinedOutput()

	// 报错不为空
	if err != nil {
		// 报错信息为  Chain already exists 表示已经创建过 直接返回
		if strings.Contains(err.Error(), "Chain already exists") {
			return nil
		}
		return err
	}

	// 设置 Prerouting 规则
	setPreroutingCmd := "-t nat -A PREROUTING -m addrtype --dst-type LOCAL -j QSRDOCKER"

	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setPreroutingCmd, " ")...).CombinedOutput()

	// 报错不为空
	if err != nil {
		return err
	}

	// 设置output规则
	setOutputCmd := "-t nat -A OUTPUT ! -d 127.0.0.0/8 -m addrtype --dst-type LOCAL -j QSRDOCKER"

	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setOutputCmd, " ")...).CombinedOutput()

	// 报错不为空
	if err != nil {
		return err
	}

	log.Debug("Create New iptables Chain QSRDOCKER success")

	return nil
}
