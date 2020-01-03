package main

import (
	"fmt"
	"qsrdocker/container"
	"qsrdocker/cgroups/subsystems"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// run 命令定义函数的Flge，可使用 -- 指定参数
var runCmd = cli.Command{
	Name: "run",
	Usage: `Create a container with namespace and cgroup`,
	ArgsUsage: "[imageName] [command]",

	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:    "it,ti", // 指定 t 参数即当前的输入输出导入到标准输入输出
			Usage:   `Enable tty and Keep STDIN open even if not attached`,
		},
		cli.BoolFlag{
			Name:  "d",  // 后台去启动 默认模式
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
			Name:  "name",  // 容器名称
			Usage: "Container name",
		},
		// 存在多个 -v 操作
		cli.StringSliceFlag{
			Name:  "v", // 数据卷
			Usage: "Volume",
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

		if tty && detach {
			return fmt.Errorf("ti and detach parameter can not both provided")
		}

		log.Debugf("Enable tty %v", tty)

		log.Debugf("Enable detach %v", detach)

		resConfig := &subsystems.ResourceConfig{
			MemoryLimit: context.String("m"),
			CPUSet:      context.String("cpuset"),
			CPUShare:    context.String("cpushare"),
			CPUMem:    	 context.String("cpumem"),
		}

		QsrdockerRun(tty, cmdList, volumes, resConfig, imageName, containerName)
		return nil
	},
}

/*
init 初始化函数, 该函数/操作为 runCmd 默认会调用的内部方法，禁止外部调用
*/
var initCmd = cli.Command{
	Name: "init",
	Usage: `init container process run user's process in container, Do not call it outside.
		Warring: you can not use init in bash/sh !`,
	HideHelp: true, // 隐藏 init命令
	Hidden: true,

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
var commitCmd = cli.Command {
	Name: "commit",
	ArgsUsage: "[containerName] [imageName]",
	Usage: "commit a container into image",
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
	Name: "ps",
	Usage: "list all the container",
	ArgsUsage: "[]",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:    "a", // 指定 t 参数即当前的输入输出导入到标准输入输出
			Usage:   `Show all containers (default shows just running)`,
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
	Name:  "logs",
	Usage: "Print logs of a container",
	ArgsUsage: "[containerName]",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:    "f,follow", // 指定 t 参数即当前的输入输出导入到标准输入输出
			Usage:   `Follow log output`,
		},
		cli.IntFlag{
			Name:    "t,tail", // 指定 t 参数即当前的输入输出导入到标准输入输出
			Usage:   `Show from the end of the logs (default "all")`,
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
		
		if  tail < 0 {
			return fmt.Errorf("Please input --t/--tail positive number")
		}
		
		containerName := context.Args().Get(0)

		// 打印 log
		logContainer(containerName, tail, follow)
		return nil
	},
}