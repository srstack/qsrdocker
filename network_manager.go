package main

import (
	"path"
	"os"
	"fmt"
	"strings"
	"text/tabwriter"
	"path/filepath"
	"qsrdocker/container"

	"github.com/urfave/cli"
	log "github.com/sirupsen/logrus"
)


// networkCmd 关于 网络的相关操作
var networkCmd = cli.Command {
	Name: "network",
	Usage: "qsrdocker network COMMAND",
	Subcommands: []cli.Command {
		networkLsCmd,
},
}

// networkLsCmd 打印所有的镜像
var networkLsCmd = cli.Command { 
	Name: "ls",
	Usage: "List networks",
	ArgsUsage: "[]",
	Action: func(context *cli.Context) error {
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

		nwID= nwID[0:(len(nwID) - 5)]
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
	fmt.Fprint(w, "NETWORK ID\tNAME\tIP Range\tDriver\n")
	for _, nw := range networks {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			nw.ID,
			nw.Driver,
			nw.IP.String(),
			nw.Driver,
		)
	}
	
	if err := w.Flush(); err != nil {
		log.Errorf("Flush error %v", err)
	}
}
	