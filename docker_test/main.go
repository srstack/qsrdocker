package main

import (
	"fmt"
	"github.com/srstack/numaer"
)

func main(){
	if numaer.IsNUMA() {
		fmt.Println("os is NUMA")
	}

	Nodes, err := numaer.Nodes()

	if err != nil {
		fmt.Errorf("ERR: %v" , err)
	}

	for _, v := range Nodes {
		fmt.Printf("node : %v \n",v.Name )
	}

	if numNode, err := numaer.NumNode(); err == nil {
		fmt.Printf("NUMA Node Num: %v \n",numNode)
	}

}