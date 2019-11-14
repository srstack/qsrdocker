package subsystems

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"
	log "github.com/Sirupsen/logrus"
)

// FindCgroupMointpoint 在 /proc/self/mountinfo 中找到关于 cgroup 挂载信息，获得挂载点根目录
func FindCgroupMointpoint(subsystem string) string {
	f, err := os.Open("/proc/self/mountinfo")
	// mountinfo 文件包含了目标进程的相关挂载信息
	// 如：
	// 34 24 0:30 / /sys/fs/cgroup/memory rw,nosuid,nodev,noexec,relatime shared:17 - cgroup cgroup rw,memory
	if err != nil {
		return ""
	}
	defer f.Close()
	// NewScanner创建并返回一个从f读取数据的Scanner，默认的分割函数是ScanLines
	scanner := bufio.NewScanner(f)
	// Scan方法获取当前位置的token（该token可以通过Bytes或Text方法获得），并让Scanner的扫描位置移动到下一个token。
	// 当扫描因为抵达输入流结尾或者遇到错误而停止时，本方法会返回false
	// 简单理解就是一行一行读取
	for scanner.Scan() {
		txt := scanner.Text()
		fields := strings.Split(txt, " ") // 以空格切片
		// Go 数组没有 -1 index，所以只能循环遍历判断
		for _, opt := range strings.Split(fields[len(fields)-1], ",") { // ["rw", "memory"]
			if opt == subsystem {
				log.Debugf("find cgroupRoot: %v", fields[4])
				return fields[4]
				// /sys/fs/cgroup/memory
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ""
	}
	return ""
	// 出现错误，统一返回 空字符串
}

// GetCgroupPath 得到 cgroup 在虚拟文件系统中的绝对路径
func GetCgroupPath(subsystem string, cgroupPath string, autoCreate bool) (string, error) {
	// 获得cgroup根目录路径
	cgroupRoot := FindCgroupMointpoint(subsystem)
	// 若cgroupRoot为空，则以 CgroupPath 为 subsystem 路径

	// 判断subsystem路径绝对路径是否存在 或者 文件/目录不存在且开启自动创建
	if _, err := os.Stat(path.Join(cgroupRoot, cgroupPath)); err == nil || (autoCreate && os.IsNotExist(err)) {
		// 创建目标目录
		if os.IsNotExist(err) {
			if err := os.Mkdir(path.Join(cgroupRoot, cgroupPath), 0755); err == nil {
			} else {
				// 无法创建目标目录
				return "", fmt.Errorf("error create cgroup %v", err)
			}
		}
		// 返回目标目录
		absCgroupPath := path.Join(cgroupRoot, cgroupPath) // 目标目录绝对路径

		log.Debugf("subsystem path : %v", absCgroupPath)

		return absCgroupPath, nil
	} else {
		// 无法获取目标目录
		return "", fmt.Errorf("cgroup path error %v", err)
	}
}
