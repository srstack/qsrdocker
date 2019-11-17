package numa

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	//"math"
	"strconv"
)

// Node ： NUMA Node 节点信息
type Node struct {
	Name string
	Zone []Zone
	CPU  []CPU
}

// Zone NUMA  zone 区域信息
type Zone struct {
	Type string
	Node Node
}

// CPU 相关信息
type CPU struct {
	ID int
	Node Node
}

// IsNUMA  判断是否为 NUMA 架构
func IsNUMA() bool {
	if _, err := os.Stat("/proc/zoneinfo"); !os.IsNotExist(err) {
		return true
	} 
	return false
}

// Nodes 获取当前系统 内存节点 node 信息
func Nodes() ([]Node, error) {
	if !IsNUMA() {
		return nil, fmt.Errorf("OS is not NUMA")
	}

	f, err := os.Open("/proc/buddyinfo")
	// buddy 文件包含了node 相关信息
	// 如：
	/*
	Node 0, zone      DMA     29      9      9      6      2      2      1      1      2      2      0 
	Node 0, zone    DMA32    941   2242   1419    398    142     60     16      1      1      0      0 
	*/

	if err != nil {
		return nil, fmt.Errorf("err : %v", err)
	}
	defer f.Close()

	var NUMANodeSlice []string

	// NewScanner创建并返回一个从f读取数据的Scanner，默认的分割函数是ScanLines
	scanner := bufio.NewScanner(f)
	// Scan方法获取当前位置的token（该token可以通过Bytes或Text方法获得），并让Scanner的扫描位置移动到下一个token。
	// 当扫描因为抵达输入流结尾或者遇到错误而停止时，本方法会返回false
	for scanner.Scan() {
		txt := scanner.Text()
		fields := strings.Split(txt, ",") // 以,切片
		NUMANodeSlice = append(NUMANodeSlice, fields[0])
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("err : %v", err)
	}

	// 去重
	NUMANodeSlice = RemoveReplicaSliceString(NUMANodeSlice)

	var Nodes []Node
	
	for _, NodeName := range NUMANodeSlice {
		Nodes = append(Nodes, Node{
			Name: NodeName,
		})
	}

	return Nodes, nil
} 

// NumNode ：获取当前系统 NUMA 数量
func NumNode() (int, error) {
	
	if !IsNUMA() {
		return 0, fmt.Errorf("OS is not NUMA")
	}

	// 获取当前系统参数
	if NodeSlice, err := Nodes(); err == nil {
		return len(NodeSlice), nil  //返回 []Node 数量
	} else {
		return 0, err
	}
}


// ZoneInfo 获取内存节点 node 的区域信息
func ZoneInfo(n *Node) ([]Zone, error) {

	return nil,nil
}

// CPUInfo 获取内存节点 Node 绑定的CPU信息
func CPUInfo(n *Node) ([]CPU, error) {
	return nil,nil
}

// BuddyInfo ：伙伴系统当前状态
func BuddyInfo(z *Zone) (map[int]int64, error) {// [11中内存碎片大小]剩余碎片数

	if !IsNUMA() {
		return nil, fmt.Errorf("OS is not NUMA")
	}

	NodeName := z.Node.Name
	ZoneType := z.Type

	f, err := os.Open("/proc/buddyinfo")
	// buddy 文件包含了node 相关信息
	// 如：
	/*
	Node 0, zone      DMA     29      9      9      6      2      2      1      1      2      2      0 
	Node 0, zone    DMA32    941   2242   1419    398    142     60     16      1      1      0      0 
	*/

	if err != nil {
		return nil, fmt.Errorf("err : %v", err)
	}

	defer f.Close()

	buddyMap := make(map[int]int64)

	// NewScanner创建并返回一个从f读取数据的Scanner，默认的分割函数是ScanLines
	scanner := bufio.NewScanner(f)
	// Scan方法获取当前位置的token（该token可以通过Bytes或Text方法获得），并让Scanner的扫描位置移动到下一个token。
	// 当扫描因为抵达输入流结尾或者遇到错误而停止时，本方法会返回false
	// 简单理解就是一行一行读取
	for scanner.Scan() {
		txt := scanner.Text()
		buddySlice := RemoveNullSliceString(strings.Split(txt, " "))
		// 判断相关信息
		if (buddySlice[0] + buddySlice[1]) == (NodeName + ",") && buddySlice[2] == "zone" && buddySlice[3] == ZoneType {
			for index, v := range buddySlice[4:] {
				vv, _ := strconv.ParseInt(v, 10, 64)
				k := 0
				if index == 0 {
					buddyMap[k] = vv
				} else {
					// 开方  math库的要f64 太难了转化了
					for i := 0; i < index; i++ {
						k = k * 2
					}
					buddyMap[k] = vv
				}
			}
		}  
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("err : %v", err)
	}	
	return buddyMap, nil
}


// RemoveReplicaSliceString 切片去重
func RemoveReplicaSliceString(srcSlice []string) []string {
 
	resultSlice := make([]string, 0)
	// 利用map key 值唯一去重
    tempMap := make(map[string]bool, len(srcSlice))
    for _, v := range srcSlice{
        if tempMap[v] == false{
            tempMap[v] = true
            resultSlice = append(resultSlice, v)
        }
    }
    return resultSlice
}


// RemoveNullSliceString : 删除空白字符的元素
func RemoveNullSliceString(srcSlice []string) []string {
 
	resultSlice := make([]string, 0)

	// 循环判断
    for _, v := range srcSlice{
        if v != "" && v != " " {
            resultSlice = append(resultSlice, v)
        }
    }
    return resultSlice
}