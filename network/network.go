package network

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"qsrdocker/container"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

var (
	// NetworkDriverMap 网络驱动
	NetworkDriverMap = map[string]NetworkDriver{
		"none": nil,
	}
)

// NetworkDriver 网络 driver 接口  Host None Container Bridge
type NetworkDriver interface {
	// 驱动名称
	Name() string
	// 创建目标驱动的网络
	Create(subnet string, name string) (*container.Network, error)
	// 删除目标驱动的网络
	Delete(network *container.Network) error
	// 连接网络端点EndPoint到网络
	Connect(network *container.Network, endpoint *container.Endpoint) error
	// 断开网络端点EndPoint到网络
	Disconnect(network *container.Network, endpoint *container.Endpoint) error
}

// CreateNetwork 创建网络
func CreateNetwork(driver, subnet, networkName string) error {

	// 讲网段字符串转化为 net.IPNet 对象
	_, cidr, _ := net.ParseCIDR(subnet)

	// 从 IP manager 获取 网关IP
	// 目标网段的第一个 IP
	gwIP, err := ipAllocator.Allocate(cidr)
	if err != nil {
		return err
	}

	cidr.IP = gwIP

	// 调用目标网络驱动的 create 方法创建网络
	nw, err := NetworkDriverMap[strings.ToLower(driver)].Create(cidr.String(), networkName)
	if err != nil {
		return err
	}

	return nw.Dump()
}

// DeleteNetwork 删除网络
func DeleteNetwork(networkName string) error {
	nw := &container.Network{
		Name: networkName,
	}

	if err := nw.Load(); err != nil {
		return fmt.Errorf("Get NetWork %v Info err: %v", networkName, err)
	}

	// 回收 IP 地址
	if err := ipAllocator.Release(nw.IP, &nw.IP.IP); err != nil {
		return fmt.Errorf("Remove Network %v gateway ip %v error: %v", networkName, nw.IP.IP, err)
	}

	if err := NetworkDriverMap[strings.ToLower(nw.Driver)].Delete(nw); err != nil {
		return fmt.Errorf("Remove Network %v Driver error: %v", networkName, err)
	}

	// 删除配置文件
	return nw.Remove()
}

// Connect 连接容器和已创建网络
func Connect(networkName string, portSlice []string, containerInfo *container.ContainerInfo) error {

	nw := &container.Network{
		Name: networkName,
	}

	if err := nw.Load(); err != nil {
		return fmt.Errorf("Get NetWork %v Info err: %v", networkName, err)
	}

	// 分配容器IP地址
	ip, err := ipAllocator.Allocate(nw.IP)
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
		if len(portPairSlice) == 2 {
			port.HostIP = "0.0.0.0"
			port.HostPort = portPairSlice[0]
			if !strings.Contains(portPairSlice[1], "/") {
				portPairSlice[1] = fmt.Sprintf("%s/tcp", portPairSlice[1])
			}
			ports[portPairSlice[1]] = append(ports[portPairSlice[1]], port)
		}

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
		ID:        fmt.Sprintf("%s-%s", containerInfo.ID, networkName),
		IPAddress: ip,
		Network:   nw,
		Ports:     ports,
	}

	containerInfo.NetWorks = ep

	// 调用网络驱动挂载和配置网络端点
	if err = NetworkDriverMap[nw.Driver].Connect(nw, ep); err != nil {
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
func Disconnect(networkName string, containerInfo *container.ContainerInfo) error {
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
	interfaceIP := containerInfo.NetWorks.Network.IP
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
	for dnatInfo, portSlice := range containerInfo.NetWorks.Ports {
		// pm ===  {hostip: string , hostPort: string }
		// dnatInfo "443/tcp"

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
			iptablesCmd := fmt.Sprintf(
				"-t nat -A PREROUTING -d %s -p %s -m %s --dport %s -j DNAT --to-destination %s:%s",
				pm.HostIP, protocol, protocol, pm.HostPort, containerInfo.NetWorks.IPAddress.String(), containerPort)

			// 执行 iptables
			cmd := exec.Command("iptables", strings.Split(iptablesCmd, " ")...)
			//err := cmd.Run()
			output, err := cmd.Output()
			if err != nil {
				log.Errorf("iptables Output, %v", output)
				continue
			}
		}

	}
	return nil
}

// ListNetWork 显示现在存在的网络
func ListNetWork() {
	networks := []*container.Network{}

	// 获取网络配置数据
	filepath.Walk(container.NetFileDir, func(nwPath string, info os.FileInfo, err error) error {
		if strings.HasSuffix(nwPath, "/") {
			return nil
		}

		// nwName   nwName.json
		_, nwName := path.Split(nwPath)

		nwName = nwName[0:(len(nwName) - 5)]
		nw := &container.Network{
			Name: nwName,
		}

		if err := nw.Load(); err != nil {
			log.Errorf("Load network Config %v error : %v", nwName, err)
		}

		networks = append(networks, nw)
		return nil
	})

	// 表格打印
	w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
	fmt.Fprint(w, "NAME\tIpRange\tDriver\n")
	for _, nw := range networks {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			nw.Name,
			nw.IP.String(),
			nw.Driver,
		)
	}
	if err := w.Flush(); err != nil {
		log.Errorf("Flush error %v", err)
		return
	}

}

// setInterfaceUP 启用网口
func setInterfaceUP(interfaceName string) error {
	iface, err := netlink.LinkByName(interfaceName)
	if err != nil {
		return fmt.Errorf("Retrieving a link named %s error: %v", iface.Attrs().Name, err)
	}

	if err := netlink.LinkSetUp(iface); err != nil {
		return fmt.Errorf("Enabling interface for %s  error : %v", interfaceName, err)
	}
	return nil
}

// setInterfaceIP Set the IP addr of a netlink interface 设置 veth peer IP
func setInterfaceIP(peerName string, rawIP string) error {

	retries := 3
	var iface netlink.Link
	var err error
	for i := 1; i < retries; i++ {
		iface, err = netlink.LinkByName(peerName)
		if err == nil {
			break
		}
		log.Debugf("Retrieving new Bridge netlink link %s fail，retrying %v", peerName, i)
		// 重试等待时间
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		return fmt.Errorf("Abandoning retrieving the new bridge link from netlink, Run [ ip link ] to troubleshoot the error: %v", err)
	}

	ipNet, err := netlink.ParseIPNet(rawIP)
	if err != nil {
		return err
	}

	// 设置IP
	addr := &netlink.Addr{IPNet: ipNet, Peer: ipNet, Label: "", Flags: 0, Scope: 0, Broadcast: nil}
	return netlink.AddrAdd(iface, addr)
}

// setupIPTables 设置SNAT
func setupIPTables(bridgeName string, subnet *net.IPNet) error {
	iptablesCmd := fmt.Sprintf("-t nat -A POSTROUTING -s %s ! -o %s -j MASQUERADE", subnet.String(), bridgeName)
	cmd := exec.Command("iptables", strings.Split(iptablesCmd, " ")...)
	//err := cmd.Run()
	output, err := cmd.Output()
	if err != nil {
		log.Errorf("iptables Output, %v", output)
	}
	return err
}
