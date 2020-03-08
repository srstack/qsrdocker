package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"qsrdocker/container"
	"qsrdocker/network"
	"strings"
	"text/tabwriter"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// networkCmd 关于 网络的相关操作
var networkCmd = cli.Command{
	Name:  "network",
	Usage: "qsrdocker network COMMAND",
	Subcommands: []cli.Command{
		networkLsCmd,
		networkCreateCmd,
		networkRemoveCmd,
	},
}

// networkCreateCmd 创建网络
var networkCreateCmd = cli.Command{
	Name:  "create",
	Usage: "create a container network",
	ArgsUsage: "[NetWork Name]",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "driver",
			Usage: "Network Driver",
			Value: container.DefaultNetworkDriver, // 默认采用 bridge 网络
		},
		cli.StringFlag{
			Name:  "subnet",
			Usage: "Subnet CIDR",
		},
	},
	Action: func(context *cli.Context) error {

		// 判断是否输入 network Name （Network ID）
		if len(context.Args()) < 1 {
			return fmt.Errorf("Missing network name")
		}

		// 获取相关参数
		networkID := context.Args()[0]
		networkDriver := context.String("driver")
		subnetCIDR := context.String("subnet")

		// 若未输入 CIDR
		if strings.Replace(subnetCIDR, " ", "", -1) == "" {
			return fmt.Errorf("Missing network CIDR")
		}

		// 创建目标网络
		err := network.CreateNetwork(networkDriver, context.String("subnet"), networkID)
		if err != nil {
			return fmt.Errorf("Create network %v in driver %v error: %+v", networkID, networkDriver, err)
		}
		return nil
	},
}

// networkRemoveCmd 删除已创建网络
var networkRemoveCmd = cli.Command{
	Name: "remove",
	Usage: "Remove Network",
	ArgsUsage: "[NetWork Name]",
	Action: func(context *cli.Context) error {
		
		// 判断是否输入 NetWork Name 
		if len(context.Args()) < 1 {
			return fmt.Errorf("Missing network name")
		}

		// 删除网络
		err := network.DeleteNetwork(context.Args()[0])
		if err != nil {
			return fmt.Errorf("Remove network %v error: %v", context.Args()[0], err)
		}
		return nil
	},
}

// networkLsCmd 打印所有的镜像
var networkLsCmd = cli.Command{
	Name:      "ls",
	Usage:     "List networks",
	ArgsUsage: "[]",
	Action: func(context *cli.Context) error {
		// 打印出所有 网络
		listNetwork()
		return nil
	},
}

// listNetWork 显示现在存在的网络
func listNetwork() {
	networks := []*container.Network{}

	// 获取网络配置数据
	filepath.Walk(
		container.NetFileDir, func(nwPath string, info os.FileInfo, err error) error {
			if !strings.HasSuffix(nwPath, ".json") {
				return nil
			}

			// nwID   nwID.json
			_, nwID := path.Split(nwPath)

			nwID = nwID[0:(len(nwID) - 5)]
			nw := &container.Network{
				ID: nwID,
			}

			if err := nw.Load(); err != nil {
				log.Errorf("Load network Config %v error : %v", nwID, err)
			}

			networks = append(networks, nw)
			return nil
		},
	)

	// 表格打印
	w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
	fmt.Fprint(w, "NETWORK ID\tGateWay IP\tIP Range\tDriver\n")
	for _, nw := range networks {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			nw.ID,
			nw.GateWayIP,
			nw.IPRangeString,
			nw.Driver,
		)
	}

	if err := w.Flush(); err != nil {
		log.Errorf("Flush error %v", err)
	}
}
