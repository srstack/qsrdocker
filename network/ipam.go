package network

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"qsrdocker/container"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

)

// 使用 bitmap 位图算法来标记地址分配状态 0:未分配  1:已分配

// IPAM 存放 ip 地址分配信息
type IPAM struct {
	// 分配文件
	SubnetAllocatorPath string
	// 锁文件
	SubnetLockPath string
	// 网段和位图算法的数组map  key是网段  value是分配的位图数组
	Subnets *map[string]string
}

// 初始化 IPAM 使用 /var/qsrdocker/network/ipam/subnet.json
var ipAllocator = &IPAM{
	SubnetAllocatorPath: path.Join(container.NetIPadminDir, container.IPamConfigFile),
	SubnetLockPath:      path.Join(container.NetIPadminDir, container.IPamLockFile),
}

// load  反序列化 subnet.json
func (ipam *IPAM) load() error {

	// 判断 subnet 配置文件是否粗存在
	if exists, _ := container.PathExists(ipam.SubnetAllocatorPath); !exists {

		// 不存在直接返回 空
		// 目前无 subnet 信息
		return nil
	}

	// 打开文件并反序列化
	subnetJSONBytes, err := ioutil.ReadFile(ipam.SubnetAllocatorPath)

	// 打开文件失败 直接返回错误
	if err != nil {
		os.Remove(ipam.SubnetLockPath)
		return err
	}
	log.Debugf("Get config file %s", ipam.SubnetAllocatorPath)

	// 反序列化到 ipam.subnets
	err = json.Unmarshal(subnetJSONBytes, ipam.Subnets)
	if err != nil {
		log.Errorf("Unmarshal Subnet info error %v", err)
		os.Remove(ipam.SubnetLockPath)
		return err
	}

	log.Debugf("Load Subnet info success")

	return nil
}

// lock 添加文件锁
func (ipam *IPAM) lock() error {
	// 判断文件锁是否存在

	// 多次尝试
	for i := 1; i < 3; i++ {

		if exists, _ := container.PathExists(ipam.SubnetLockPath); exists {
			log.Debugf("Get subnet lock fail, retry %v", i)
		} else {

			// 判断 IPAM 目录是否存在 不存在则创建
			if exists, _ = container.PathExists(container.NetIPadminDir); !exists {
				os.MkdirAll(container.NetIPadminDir, 0644)
			}

			// 创建 文件锁
			lock, err := os.Create(ipam.SubnetLockPath)
			if err != nil {
				// 创建失败则返回
				return fmt.Errorf("Create subnet lock file fail error %v", err)
			}

			// 关闭锁文件
			lock.Close()
			// 跳出循环
			break
		}

		// 重试等待时间
		time.Sleep(1 * time.Second)
	}

	return nil

}

// unlock 删除文件锁
func (ipam *IPAM) unlock() error {

	if exists, _ := container.PathExists(ipam.SubnetLockPath); !exists {
		return fmt.Errorf("Get file lock error, %v is not exists", ipam.SubnetLockPath)
	}

	if err := os.Remove(ipam.SubnetLockPath); err != nil {
		log.Errorf("Remove file lock fail error %v", err)
	}

	return nil
}

// dump 序列化 subnet.json
func (ipam *IPAM) dump() error {

	// 判断 NetIPadminDir 是否存在
	// 不存在则创建
	if exists, _ := container.PathExists(container.NetIPadminDir); !exists {
		os.MkdirAll(container.NetIPadminDir, 0644)
		log.Debugf("Create ipam dir %s", container.NetIPadminDir)
	}

	// 打开 subnet.json 文件 ，不存在则创建  标志位 os.O_CREATE
	subnetConfigFile, err := os.OpenFile(ipam.SubnetAllocatorPath, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
	defer subnetConfigFile.Close()
	if err != nil {
		return err
	}

	log.Debugf("Get config file %s", ipam.SubnetAllocatorPath)

	// 序列化 bitmap
	ipamConfigJSONByte, err := json.MarshalIndent(ipam.Subnets, " ", "    ")
	if err != nil {
		return err
	}

	ipamConfigJSONStr := string(ipamConfigJSONByte)
	ipamConfigJSONStr = strings.Join([]string{ipamConfigJSONStr, "\n"}, "")

	// 写入文件
	_, err = subnetConfigFile.WriteString(ipamConfigJSONStr)
	if err != nil {
		return err
	}

	log.Debug("Dump Subnet info success")

	return nil
}

// Create 创建新的网段
func (ipam *IPAM) Create(subnet *net.IPNet) (err error) {

	// 存放网段中地址分配信息的 字符串切片
	ipam.Subnets = &map[string]string{}

	// 从文件中加载已经分配的网段信息
	// 存在配置文件才 load

	// 增加文件锁
	if err = ipam.lock(); err != nil{
		return
	}

	// 删除文件锁
	defer ipam.unlock()

	if exists, _ := container.PathExists(ipam.SubnetAllocatorPath); exists {
		err = ipam.load()
		if err != nil {
			log.Errorf("Dump SubNet info error %v", err)
			// 有名返回
			return
		}
	}

	// 如果之前分配过该网段, 则返回错误
	if _, exist := (*ipam.Subnets)[subnet.String()]; exist {
		err = fmt.Errorf("Subnet %v is exist, please Create another Subnet", subnet.String())
		return
	}

	// 判断 网段是否冲突
	// subnetCreatedString : 192.168.1.0/24
	for subnetCreatedString := range *ipam.Subnets {
		// 得到 已创建网络的 网络位地址 192.168.1.0 和 网段 192.168.1.0/24
		ipCreated, subCreated, _ := net.ParseCIDR(subnetCreatedString)

		// 1. 新创建网络包含已创建网络 网络位地址
		// 2. 已创建网络包含 新建网络 网络位地址
		// 满足以上仁义一种情况则说明 网段冲突
		if subnet.Contains(ipCreated) || subCreated.Contains(subnet.IP) {
			err = fmt.Errorf("Network Subnet %v fail error conflict with %v", subnet.String(), subCreated.String())
			return
		}
	}

	// 返回目标网段 网络位 和 主机位
	// 127.0.0.0/8  netsize:8  size:32
	netsize, size := subnet.Mask.Size()

	if netsize < 24 {
		err = fmt.Errorf("Network Subnet Mask must > 24")
		return
	}

	// 用 0 填满该网段配置
	// hostsize-netone 代表可用位数
	// 左移运算符"<<"是双目运算符。左移n位就是乘以2的n次方。 其功能把"<<"左边的运算数的各二进位全部左移若干位
	// 2^(size-netsize) == 1<<uint8(size-netsize)
	(*ipam.Subnets)[subnet.String()] = strings.Repeat("0", 1<<uint8(size-netsize))

	log.Debugf("Create SubNet %v success ", subnet.String())

	// 持久化配置
	defer ipam.dump()

	return
}

// Allocate 在网段中分配一个可用的 IP 地址
func (ipam *IPAM) Allocate(subnet *net.IPNet) (ip net.IP, err error) {

	// 存放网段中地址分配信息的 字符串切片
	ipam.Subnets = &map[string]string{}

	// 增加文件锁
	if err = ipam.lock(); err != nil{
		return
	}

	// 删除文件锁
	defer ipam.unlock()

	// 从文件中加载已经分配的网段信息
	// 存在配置文件才 load
	if exists, _ := container.PathExists(ipam.SubnetAllocatorPath); exists {
		err = ipam.load()
		if err != nil {
			log.Errorf("Dump SubNet info error %v", err)
			// 有名返回
			return
		}
	}

	// 将字符串转化为 网段信息
	_, subnet, _ = net.ParseCIDR(subnet.String())

	// 如果之前没有分配过该网段, 则返回错误
	if _, exist := (*ipam.Subnets)[subnet.String()]; !exist {
		err = fmt.Errorf("Subnet %v is not exist, please Create Network first", subnet.String())
		return
	}

	// 遍历位图map中的字符串
	for offset := range (*ipam.Subnets)[subnet.String()] {
		// 找到第一个 value 为 0 的项，即为可分配的 IP 地址
		if (*ipam.Subnets)[subnet.String()][offset] == '0' {
			// 设置该项的 value 为 1
			// 字符串切片不能直接赋值 "1"
			ipAllocs := []byte((*ipam.Subnets)[subnet.String()])
			ipAllocs[offset] = '1'

			// 赋值回原 value
			(*ipam.Subnets)[subnet.String()] = string(ipAllocs)

			// 获取初始IP ，即主机位全为 0
			// 将 IP 转化为 4 字节表达形式
			ip = subnet.IP.To4()

			// 根据偏移量 offset  得到目标 IP
			// 四次循环分别得到 1.2.3.4  1的偏移量 2的偏移量 3的偏移量 4的偏移量
			for t := uint(4); t > 0; t-- {
				// >> 右移n位就是除以2的n次方
				// 忽略小数
				[]byte(ip)[4-t] += uint8(offset >> ((t - 1) * 8))
			}

			// 由于是从 主机位 1 开始分配，需要 +1
			ip[3]++
			break
		}
	}

	log.Debugf("Allocate IP  %v success in %v", ip.String(), subnet.String())

	// 持久化
	ipam.dump()

	// 有名返回
	return
}

// Release 使用图位法释放IP地址
func (ipam *IPAM) Release(subnet *net.IPNet, ip *net.IP) error {


	// 初始化反序列化结构
	ipam.Subnets = &map[string]string{}

	// 增加文件锁
	if err := ipam.lock(); err != nil{
		return err
	}

	// 删除文件锁
	defer ipam.unlock()

	// 获取 ipam 数据
	err := ipam.load()
	if err != nil {
		log.Errorf("Dump Subnet info error %v", err)
	}

	// 从ip地址得到网段地址
	_, subnet, _ = net.ParseCIDR(subnet.String())

	// 初始化偏移量
	offset := 0

	// 将ip地址设置为4字节表达形式
	releaseIP := ip.To4()

	// 除去 网关 1 地址
	releaseIP[3]--

	// 计算偏移量
	// 分配的反向计算
	for t := uint(4); t > 0; t-- {
		offset += int(releaseIP[t-1]-subnet.IP[t-1]) << ((4 - t) * 8)
	}

	if offset == 0 {
		// 释放网关地址则删除该网段
		delete((*ipam.Subnets), subnet.String())

	} else {
		// 释放单个地址
		// 获取 位图map 偏移量
		ipAllocs := []byte((*ipam.Subnets)[subnet.String()])

		// 释放地址
		ipAllocs[offset] = '0'
		(*ipam.Subnets)[subnet.String()] = string(ipAllocs)
	}

	// 持久化
	ipam.dump()

	// 恢复IP
	releaseIP[3]++

	log.Debugf("Release IP %v success", releaseIP.String())

	return nil
}
