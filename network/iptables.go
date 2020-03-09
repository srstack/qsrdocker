package network

import (
	"fmt"
	"net"
	"os/exec"
	"qsrdocker/container"
	"strings"

	log "github.com/sirupsen/logrus"
)

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
	setNatCmd := fmt.Sprintf("-t nat -A QSRDOCKER -i %v -j RETURN", bridgeID)
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
	errinfo, err := exec.Command("iptables", strings.Split(setChainCmd, " ")...).CombinedOutput()

	if err != nil {
		return fmt.Errorf(string(errinfo))
	}

	// 取消转发规则 conntrack
	setConntrackCmd := fmt.Sprintf("-D FORWARD -o %v -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT", bridgeID)
	// 直接运行 cmd 命令
	errinfo, err = exec.Command("iptables", strings.Split(setConntrackCmd, " ")...).CombinedOutput()

	if err != nil {
		return fmt.Errorf(string(errinfo))
	}

	// 取消非目标网络到目标网络 的 流量转发
	setForwardNotLocalCmd := fmt.Sprintf("-D FORWARD -i %v ! -o %v -j ACCEPT", bridgeID, bridgeID)
	// 直接运行 cmd 命令
	errinfo, err = exec.Command("iptables", strings.Split(setForwardNotLocalCmd, " ")...).CombinedOutput()

	if err != nil {
		return fmt.Errorf(string(errinfo))
	}

	// 取消 非目标网络到目标网络 的 流量转发
	setForwardLocalCmd := fmt.Sprintf("-D FORWARD -i %v -o %v -j ACCEPT", bridgeID, bridgeID)
	// 直接运行 cmd 命令
	errinfo, err = exec.Command("iptables", strings.Split(setForwardLocalCmd, " ")...).CombinedOutput()

	if err != nil {
		return fmt.Errorf(string(errinfo))
	}

	// 取消 SNAT
	setSnatCmd := fmt.Sprintf("-t nat -D POSTROUTING -s %v ! -o %v -j MASQUERADE", subnet.String(), bridgeID)
	// 直接运行 cmd 命令
	errinfo, err = exec.Command("iptables", strings.Split(setSnatCmd, " ")...).CombinedOutput()

	if err != nil {
		return fmt.Errorf(string(errinfo))
	}

	// -t nat -A DOCKER -i docker0 -j RETURN
	setNatCmd := fmt.Sprintf("-t nat -D QSRDOCKER -i %v -j RETURN", bridgeID)
	// 直接运行 cmd 命令
	errinfo, err = exec.Command("iptables", strings.Split(setNatCmd, " ")...).CombinedOutput()

	if err != nil {
		return fmt.Errorf(string(errinfo))
	}

	return nil
}

// IPtablesInit 初始化容器iptables
// -nat -A PREROUTING -m addrtype --dst-type LOCAL -j QSRDOCKER
// -nat -A OUTPUT ! -d 127.0.0.0/8 -m addrtype --dst-type LOCAL -j QSRDOCKER
func IPtablesInit() error {

	// 创建新链
	newChainCmd := "-N QSRDOCKER"

	// 直接运行 cmd 命令
	errinfo, err := exec.Command("iptables", strings.Split(newChainCmd, " ")...).CombinedOutput()

	// 报错不为空
	if err != nil {
		// 报错信息为  Chain already exists 表示已经创建过 直接返回
		if strings.Contains(string(errinfo), "Chain already exists") {
			return nil
		}
		return err
	}

	log.Debugf("Set QSRDOCKER Chain success")

	// 创建新链
	newNatChainCmd := "-t nat -N QSRDOCKER"

	// 直接运行 cmd 命令
	errinfo, err = exec.Command("iptables", strings.Split(newNatChainCmd, " ")...).CombinedOutput()

	// 报错不为空
	if err != nil {
		// 报错信息为  Chain already exists 表示已经创建过 直接返回
		if strings.Contains(string(errinfo), "Chain already exists") {
			return nil
		}
		return err
	}
	log.Debugf("Set nat QSRDOCKER Chain success")

	// 设置 Prerouting 规则
	setPreroutingCmd := "-t nat -A PREROUTING -m addrtype --dst-type LOCAL -j QSRDOCKER"

	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setPreroutingCmd, " ")...).CombinedOutput()

	// 报错不为空
	if err != nil {
		return err
	}

	log.Debugf("Set PREROUTING in QSRDOCKER Chain success")

	// 设置output规则
	setOutputCmd := "-t nat -A OUTPUT ! -d 127.0.0.0/8 -m addrtype --dst-type LOCAL -j QSRDOCKER"

	// 直接运行 cmd 命令
	_, err = exec.Command("iptables", strings.Split(setOutputCmd, " ")...).CombinedOutput()

	// 报错不为空
	if err != nil {
		return err
	}

	log.Debugf("Set OUTPUT in QSRDOCKER Chain success")

	log.Debug("Create New iptables Chain QSRDOCKER success")

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
			errinfo, err := exec.Command("iptables", strings.Split(PortMappingAcceptCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables set error %v", fmt.Errorf(string(errinfo)))
				continue
			}

			// -A POSTROUTING -s 172.17.0.2/32 -d 172.17.0.2/32 -p tcp -m tcp --dport 3306 -j MASQUERADE
			PortMappingPostRoutingCmd := fmt.Sprintf(
				"-t nat -A POSTROUTING -s %v -d %v -p %v -m %v --dport %v -j MASQUERADE",
				containerIPWithMask, containerIPWithMask, protocol, protocol, containerPort)
			// 执行 iptables
			errinfo, err = exec.Command("iptables", strings.Split(PortMappingPostRoutingCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables set error %v", fmt.Errorf(string(errinfo)))
				continue
			}

			// -A DOCKER ! -i qsrdocker0 -p tcp -m tcp --dport 33060 -j DNAT --to-destination 172.17.0.2:3306
			PortMappingDNatCmd := fmt.Sprintf(
				"-t nat -A QSRDOCKER ! -i %v -p %v -m %v --dport %v -j DNAT --to-destination %v:%v",
				linkID, protocol, protocol, pm.HostPort, containerIP, containerPort)
			// 执行 iptables
			errinfo, err = exec.Command("iptables", strings.Split(PortMappingDNatCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables set error %v", fmt.Errorf(string(errinfo)))
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
			errinfo, err := exec.Command("iptables", strings.Split(PortMappingAcceptCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables del error %v", fmt.Errorf(string(errinfo)))
				continue
			}

			// -D POSTROUTING -s 172.17.0.2/32 -d 172.17.0.2/32 -p tcp -m tcp --dport 3306 -j MASQUERADE
			PortMappingPostRoutingCmd := fmt.Sprintf(
				"-t nat -D POSTROUTING -s %v -d %v -p %v -m %v --dport %v -j MASQUERADE",
				containerIPWithMask, containerIPWithMask, protocol, protocol, containerPort)
			// 执行 iptables
			errinfo, err = exec.Command("iptables", strings.Split(PortMappingPostRoutingCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables del error %v", fmt.Errorf(string(errinfo)))
				continue
			}

			// -D DOCKER ! -i qsrdocker0 -p tcp -m tcp --dport 33060 -j DNAT --to-destination 172.17.0.2:3306
			PortMappingDNatCmd := fmt.Sprintf(
				"-t nat -D QSRDOCKER ! -i %v -p %v -m %v --dport %v -j DNAT --to-destination %v:%v",
				linkID, protocol, protocol, pm.HostPort, containerIP, containerPort)
			// 执行 iptables
			errinfo, err = exec.Command("iptables", strings.Split(PortMappingDNatCmd, " ")...).CombinedOutput()

			if err != nil {
				log.Errorf("iptables del error %v", fmt.Errorf(string(errinfo)))
				continue
			}

		}

	}
	return nil
}
