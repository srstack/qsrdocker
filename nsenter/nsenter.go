package nsenter

// 仿写 nsenter 工具
// 原理为 系统调用 setns()  可以根据PID，将进程加入到指定的NS中


/*
#define _GNU_SOURCE
#include <unistd.h>
#include <errno.h>
#include <sched.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <fcntl.h>

// __attribute__ 代表这个包被引用则自动执行该函数，类似go中的 init()
// 或者 类似 构造函数
// 导入时执行因为没有环境变量，执行无效
// 在 exec 中显式调用生效

__attribute__((constructor)) void enter_namespace(void) {

	// 定义字符指针，用于存储 container 进程 PID
	// 字符指针 = 字符数组 = 字符串
	char *qsrdocker_pid;

	// 从环境变量中获取 PID
	qsrdocker_pid = getenv("qsrdocker_pid");

	// 
	if (qsrdocker_pid) {
		// debug log
		// fprintf(stdout, "got qsrdocker_pid=%s\n", qsrdocker_pid);
	} else {
		// debug log
		// fprintf(stdout, "missing qsrdocker_pid env skip nsenter");

		// 没获取到 pid 直接退出
		return;
	}

	// 存取 执行的命令
	char *qsrdocker_cmd;

	// 从环境变量中获取执行的命令
	qsrdocker_cmd = getenv("qsrdocker_cmd");
	if (qsrdocker_cmd) {
		// debug log
		// fprintf(stdout, "got qsrdocker_cmd=%s\n", qsrdocker_cmd);
	} else {
		// debug log
		// fprintf(stdout, "missing qsrdocker_cmd env skip nsenter");

		// 没获取到直接退出
		return;
	}

	// NS 计数器
	int i;
	char nspath[1024];
	
	// 设置 6 种环境变量
	char *namespaces[] = { "ipc", "uts", "net", "pid", "mnt", "user" };

	// qsrdocker 使用 user namespace , 当使用 qsrdocker exec 进入 container 时，确保当前用户与 创建 container 用户一致 

	for (i=0; i<6; i++) {
		// 拼接进程ns目录 
		sprintf(nspath, "/proc/%s/ns/%s", qsrdocker_pid, namespaces[i]);

		// 获取 ns 描述信息
		int fd = open(nspath, O_RDONLY);

		// 真正的系统调用 setns
		// 返回值 -1 则调用失败
		if (setns(fd, 0) == -1) {
			// debug log
			// fprintf(stderr, "setns on %s namespace failed: %s\n", namespaces[i], strerror(errno));
		} else {
			// debug log
			// fprintf(stdout, "setns on %s namespace succeeded\n", namespaces[i]);
		}
		// 关闭 类似于 file.Close()
		close(fd);
	}

	// 进入新的 namespace 执行命令
	int res = system(qsrdocker_cmd);
	exit(0);
	return;
}
*/
import "C"