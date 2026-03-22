package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

var refreshSpeed int
var ginAddr string

func init() {
	if len(os.Args) < 2 {
		refreshSpeed = 0
		ginAddr = "0.0.0.0:6262"
	} else {
		ginAddr = os.Args[1]
		refreshSpeed, _ = strconv.Atoi(os.Args[2])

	}
	fmt.Printf("ginAddr: %v, refreshSpeed: %v\n", ginAddr, refreshSpeed)
}

func debugPrint() {
	for refreshSpeed != 0 {
		updatePseudoTerminalsList()
		printList()
		time.Sleep(time.Second * time.Duration(refreshSpeed))
		fmt.Println("----------------------------------------------")
	}
}

func main() {

	recreateServices()
	updatePseudoTerminalsList()

	go debugPrint()

	router := gin.Default()

	router.POST("/getPseudoTerminalAddress", getPseudoTerminalAddress)
	router.POST("/killUserPod", killUserPod)

	router.Run(ginAddr)
}
