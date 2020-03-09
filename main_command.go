package main

import (
	"fmt"
	"os"
	"qsrdocker/cgroups/subsystems"
	"qsrdocker/container"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// run 命令定义函数的Flge，可使用 -- 指定参数
var runCmd = cli.Command{
	Name:      "run",
	Usage:     `Create a container with namespace and cgroup`,
	ArgsUsage: "imageName [command]",

	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "it,ti", // 指定 t 参数即当前的输入输出导入到标准输入输出
			Usage: `Enable tty and Keep STDIN open even if not attached`,
		},
		cli.BoolFlag{
			Name:  "d", // 后台去启动 默认模式
			Usage: "Detach container",
		},
		cli.StringFlag{
			Name:  "m", // 设置 内存使用
			Usage: "Set Memory limit",
		},
		cli.StringFlag{
			Name:  "cpushare", // 限制 Cpu 使用
			Usage: "Set cpushare limit",
		},
		cli.StringFlag{
			Name:  "cpuset", // 限制 Cpu 使用核数
			Usage: "Set cpuset limit",
		},
		cli.StringFlag{
			Name:  "cpumem", // 在 NUMA模式下 限制 Cpu 使用 内存节点
			Usage: "Set cpumem node limit in NUMA mode，Usually no restrictions",
		},
		cli.StringFlag{
			Name:  "name", // 容器名称
			Usage: "Container name",
		},
		cli.StringFlag{
			Name:  "oom_kill_disable", // 容器名称
			Usage: "oom_kill_disable, 1: disable 0:able (default 0)",
		},
		// 存在多个 -v 操作
		cli.StringSliceFlag{
			Name:  "v", // 数据卷
			Usage: "Set volume mount",
		},
		cli.StringSliceFlag{
			Name:  "e",
			Usage: "Set environment",
		},
		cli.StringFlag{
			Name:  "n", // 指定网络
			Usage: "Set container network id",
			Value: container.DefaultNetworkID,
		},
		cli.StringFlag{
			Name:  "netdriver", // 指定网络
			Usage: "Set container network driver, like bridge, host, none, container",
			Value: container.DefaultNetworkDriver,
		},
		cli.StringFlag{
			Name:  "container", // 指定网络
			Usage: "Set container ID/Name with container driver network",
			Value: container.DefaultNetworkID,
		},
		cli.StringSliceFlag{
			Name:  "p",
			Usage: "Set port mapping",
		},
	},

	/*
		1. 是否包含 cmd
		2. 获取用户指定 cmd
		3. 调用 run 函数
	*/
	Action: func(context *cli.Context) error {

		// 打印当前输入的命令
		log.Debugf("Qsrdocker run cmd : %v", context.Args())

		if len(context.Args()) < 1 {
			return fmt.Errorf("Missing run container command, please qsrdocker run -h")
		}

		var cmdList []string
		for _, arg := range context.Args() {
			cmdList = append(cmdList, arg)
		}

		imageName := cmdList[0]
		cmdList = cmdList[1:]

		tty := context.Bool("it")
		// -ti 或者 -it 都可以
		detach := context.Bool("d")

		// 容器名称
		containerName := context.String("name")

		// 数据卷
		volumes := context.StringSlice("v")

		// 数据卷
		envSlice := context.StringSlice("e")

		// 容器网络ID
		networkID := context.String("n")

		// 容器网络driver
		networkDriver := strings.ToLower(context.String("netdriver"))

		if networkDriver == "container" {
			return fmt.Errorf("This mode is not currently supported")
		}
		
		// container 模式网络 目标 container 信息
		containerNetwork := context.String("container")

		// 端口映射
		portmapping := context.StringSlice("p")

		if tty && detach {
			return fmt.Errorf("ti and detach parameter can not both provided")
		}

		log.Debugf("Enable tty %v", tty)

		log.Debugf("Enable detach %v", detach)

		oomKillAble := context.String("oom_kill_disable")
		if oomKillAble != "0" {
			// 可能存在其他数字(用户乱写....)
			oomKillAble = "1"
		}

		resConfig := &subsystems.ResourceConfig{
			MemoryLimit:    context.String("m"),
			CPUSet:         context.String("cpuset"),
			CPUShare:       context.String("cpushare"),
			CPUMem:         context.String("cpumem"),
			OOMKillDisable: oomKillAble,
		}

		// 选用 container 网络模式 时，必须采用
		if networkDriver == "container" && containerNetwork == "" {
			return fmt.Errorf("Please set container ID/Name with container driver network")
		}

		// 若是以下三种网络模型 则不需要 networkID 的存在
		if networkDriver == "none" || networkDriver == "container" || networkDriver == "host" {
			networkID = ""
		}

		QsrdockerRun(tty, cmdList, volumes, envSlice, portmapping, resConfig, imageName, containerName, networkID, networkDriver, containerNetwork)
		return nil
	},
}

/*
init 初始化函数, 该函数/操作为 runCmd 默认会调用的内部方法，禁止外部调用
*/
var initCmd = cli.Command{
	Name: "init",
	Usage: `Init container process run user's process in container, Do not call it outside.
		Warring: you can not use init in bash/sh !`,
	HideHelp: true, // 隐藏 init命令
	Hidden:   true,

	/*
		1. 获取传递过来的 参数
		2. 执行容器初始化
	*/

	Action: func(context *cli.Context) error {
		log.Debugf("init qsrdocker")
		err := container.RunContainerInitProcess()
		return err
	},
}

// 导出当前容器生成镜像
// 分层镜像特性实现
var commitCmd = cli.Command{
	Name:      "commit",
	ArgsUsage: "containerName imageName",
	Usage:     "commit a container into image",
	Action: func(context *cli.Context) error {

		// 判断输入是否正确
		if len(context.Args()) < 2 {
			return fmt.Errorf("Missing container name and image name")
		}
		containerName := context.Args().Get(0)
		imageName := context.Args().Get(1)
		CommitContainer(containerName, imageName)
		return nil
	},
}

// listCmd: qsrdocker ps [-a] []
var listCmd = cli.Command{
	Name:      "ps",
	Usage:     "List all the container",
	ArgsUsage: "[]",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "a", // 指定 t 参数即当前的输入输出导入到标准输入输出
			Usage: `Show all containers (default shows just running)`,
		},
	},

	Action: func(context *cli.Context) error {
		// show all container
		all := context.Bool("a")
		listContainers(all)

		return nil
	},
}

// logCommand qsrdocker logs -f/-t
var logCmd = cli.Command{
	Name:      "logs",
	Usage:     "Print logs of a container",
	ArgsUsage: "containerName",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "f,follow", // tail -f 追踪
			Usage: `Follow log output`,
		},
		cli.IntFlag{
			Name:  "t,tail", // tail 现在末尾几行
			Usage: `Show from the end of the logs (default "all")`,
		},
	},
	Action: func(context *cli.Context) error {
		if len(context.Args()) < 1 {
			return fmt.Errorf("Please input container Name")
		}

		// tail -f
		follow := context.Bool("f")

		// 打印末尾几行
		tail := context.Int("t")

		if tail < 0 {
			return fmt.Errorf("Please input --t/--tail positive number")
		}

		containerName := context.Args().Get(0)

		// 打印 log
		logContainer(containerName, tail, follow)
		return nil
	},
}

var execCmd = cli.Command{
	Name:      "exec",
	Usage:     "Exec a command into container",
	ArgsUsage: "containerName [command]",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "it,ti", // 指定 t 参数即当前的输入输出导入到标准输入输出
			Usage: `Enable tty and Keep STDIN open even if not attached`,
		},
	},
	Action: func(context *cli.Context) error {

		tty := context.Bool("it")
		// -ti 或者 -it 都可以

		// 获取环境变量
		// 第一次调用的时候会是否
		if strings.Replace(os.Getenv(ENVEXECPID), " ", "", -1) != "" {
			log.Debugf("Exec callback Pid %v , container Pid %s", os.Getgid(), os.Getenv(ENVEXECPID))
			return nil
		}

		if len(context.Args()) < 2 {
			return fmt.Errorf("Missing container name or command")
		}

		// 获取
		containerName := context.Args().Get(0)

		var cmdList []string

		// 返回除去 containerName
		// Tail 除去第一个
		for _, arg := range context.Args().Tail() {
			cmdList = append(cmdList, arg)
		}

		// 处理
		ExecContainer(tty, containerName, cmdList)
		return nil
	},
}

// inspectCmd  qsrdocker inspect [containerName/ID]  获取容器信息
var inspectCmd = cli.Command{
	Name:      "inspect",
	Usage:     "Print info of a container",
	ArgsUsage: "containerName",
	Action: func(context *cli.Context) error {
		if len(context.Args()) < 1 {
			return fmt.Errorf("Please input container Name")
		}

		containerName := context.Args().Get(0)

		// 打印 log
		inspectContainer(containerName)
		return nil
	},
}

// stopCmd 暂停 运行中的容器
var stopCmd = cli.Command{
	Name:      "stop",
	Usage:     "Stop a container",
	ArgsUsage: "containerName",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "t", // 指定 t
			Value: 0,
			Usage: `Seconds to wait for stop before killing it`,
		},
	},
	Action: func(context *cli.Context) error {
		if len(context.Args()) < 1 {
			return fmt.Errorf("Missing container name")
		}

		sleepTime := context.Int("t")

		containerName := context.Args().Get(0)
		stopContainer(containerName, sleepTime)
		return nil
	},
}

// removeCmd 删除 Dead / Stop 的容器  -f 强制停止
var removeCmd = cli.Command{
	Name:      "rm",
	Usage:     "Remove unused one or more containers",
	ArgsUsage: "containerName...",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "f", // 强制删除容器
			Usage: `Force the removal of a running container (uses SIGKILL)`,
		},
		cli.BoolFlag{
			Name:  "v", // 强制删除容器
			Usage: `Remove the volumes associated with the container`,
		},
	},
	Action: func(context *cli.Context) error {
		if len(context.Args()) < 1 {
			return fmt.Errorf("Missing container name")
		}

		// 获取参数
		Force := context.Bool("f")
		volume := context.Bool("v")

		for _, containerName := range context.Args() {
			// 多个容器
			removeContainer(containerName, Force, volume)
		}
		return nil
	},
}

// startCmd 启动
var startCmd = cli.Command{
	Name:      "start",
	Usage:     "Start one or more stopped containers",
	ArgsUsage: "containerName...",
	Action: func(context *cli.Context) error {
		if len(context.Args()) < 1 {
			return fmt.Errorf("Missing container name")
		}

		for _, containerName := range context.Args() {
			// 多个容器
			startContainer(containerName)
		}
		return nil
	},
}
