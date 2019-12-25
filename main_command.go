package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/srstack/qsrdocker/container"
	"github.com/urfave/cli"
	"github.com/srstack/qsrdocker/cgroups/subsystems"
)

// run 命令定义函数的Flges，可使用 -- 指定参数
var runCmd = cli.Command{
	Name: "run",
	Usage: `Create a container with namespace and cgroup, docker run -ti [-m] [...] [image] [command]`,

	Flags: []cli.Flag{

		cli.BoolFlag{
			Name:    "it,ti", // 指定 t 参数即当前的输入输出导入到标准输入输出
			Usage:   `enable tty and Keep STDIN open even if not attached`,
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
			Usage: "container name",
		},
		// 存在多个 -v 操作
		cli.StringSliceFlag{
			Name:  "v", // 数据卷
			Usage: "volume",
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
			return fmt.Errorf("ti and d paramter can not both provided")
		}

		log.Debugf("Enable tty %v", tty)

		resConfig := &subsystems.ResourceConfig{
			MemoryLimit: context.String("m"),
			CPUSet:      context.String("cpuset"),
			CPUShare:    context.String("cpushare"),
			CPUMem:    	 context.String("cpumem"),
		}

		log.Debugf("Create cgroup config: %+v", resConfig)

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
		err := container.RunCotainerInitProcess()
		return err
	},
}
