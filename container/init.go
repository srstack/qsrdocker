package container

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	log "github.com/sirupsen/logrus"
)


// RunCotainerInitProcess 创建真正的容器进程
func RunCotainerInitProcess() error {
	
	// 获取用户输入
	cmdList := readUserCmd()

	log.Debugf("Get cmdList %v, len : %v from user", cmdList, len(cmdList))

	if len(cmdList) == 1 && cmdList[0] == "" {
		return fmt.Errorf("Run container get user command error, command is nil")
	}

	// 设置挂载点
	setUpMount()

	// 调用 exec.LookPath 在系统的 PATH 中寻找命令的绝对路径
	absPath, err := exec.LookPath(cmdList[0])

	if err != nil {
		log.Errorf("Exec Loop Path error : %v", err)
	}

	log.Debugf("Find command absPATH : %s", absPath)

	// exec 创建真正的容器进程
	 if err := syscall.Exec(absPath, cmdList[0:], os.Environ()); err != nil {
		log.Errorf(err.Error())
	}

	return nil

}

// readUserCmd 获取用户参数
func readUserCmd() []string {

	// readPipe是下标为 3 的文件描述符
	readPipe := os.NewFile(uintptr(3), "pipe")
	defer readPipe.Close()
	cmdByte, err := ioutil.ReadAll(readPipe)

	if err != nil {
		log.Errorf("get user's cmd error : %v", err)
		return nil
	}

	// 传过来的是字节
	cmdString := string(cmdByte)

	cmdList := strings.Split(cmdString, " ")

	return cmdList
}

// pivot_root 系统调用，改变当前的root文件系统
// 与 chroot的区别
// chroot  是针对某个进程，系统的其他部分仍处于 原root下
// prvot_root 是将整个系统移植到新的 new_root 下，移除系统对 old_root 的依赖
// pivotRoot 修改当前 root 系统
func pivotRoot(root string) error {
	// 为了保证当前root的 new_root 和 old_root 不在同一文件系统中
	// 需要将 root 重新 mount 一次
	if err := syscall.Mount(root, root, "bind", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("Mount roots to itself fail : %v", err )
	}

	// 创建 rootfs/.pivot_root 存储 old_root
	pivotDir := filepath.Join(root, ".pivot_root")
	if err := os.Mkdir(pivotDir, 0777); err != nil {
		return fmt.Errorf("mkdir rootfs/.pivot_root fail: %v", err)
	}

	// 将 pivot 挂载到新的 rootfs
	// 将 old_root 挂载到 rootfs/.pivot_root
	if err := syscall.PivotRoot(root, pivotDir); err != nil {
		return fmt.Errorf("syscall.PivotRoot err : %v", err )
	}

	// 修改当前工作目录到根目录
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("change work dir err : %v", err )
	}
	
	// new_root/.pivot_root
	pivotDir = filepath.Join("/", ".pivot_root")

	// 解挂载
	// MNT_DETACH 函数执行带有此参数，不会立即执行umount操作，而会等挂载点退出忙碌状态时才会去卸载它
	if err := syscall.Unmount(pivotDir, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount old_root err : %v", err )
	}

	// 删除已经解除挂载的 old_root 临时文件夹 new_root/.pivot_root
	if err := os.Remove(pivotDir); err != nil {
		return fmt.Errorf("remove old_root err : %v", err )
	}

	return nil
}

// init 挂载点
// setUpMount 在 RunCotainerInitProcess 中执行
func setUpMount() {

	// 获取当前路径
	pwd, err := os.Getwd()
	if err != nil {
		log.Warnf("get pwd err : %v", err)
	}

	log.Debugf("Current dir is : %v", pwd)

	pivotRoot(pwd) // 修改当前目录为 根目录

	/*
		MS_ONEXC 本文件系统允许允许其他程序
		MS_NOSUID 本文件系统运行时，不允许 set_uid 和 set_gid
		MS_NODEV linux 2.4 之后有的 mount 默认参数
	*/
	defaultMountFlages := syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV

	// mount -t proc proc /proc
	syscall.Mount("proc", "/proc", "proc", uintptr(defaultMountFlages), "")
	
	// 挂载内存文件系统 tmpfs 
	syscall.Mount("tmpfs", "/dev", "tempfs", syscall.MS_NOSUID|syscall.MS_STRICTATIME, "mode=755")
}