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
	logManagerf("startup args parsed ginAddr=%s refreshSpeed=%d", ginAddr, refreshSpeed)
}

func debugPrint() {
	logManagerf("debug printer started refreshSpeed=%d", refreshSpeed)
	for refreshSpeed != 0 {
		updatePseudoTerminalsList()
		logManagerf("debug snapshot %s", pseudoTerminalStateCounts())
		printList()
		time.Sleep(time.Second * time.Duration(refreshSpeed))
		fmt.Println("----------------------------------------------")
	}
}

func main() {
	logManagerf(
		"manager boot namespace=%s statefulSet=%s servicePort=%d",
		namespace,
		pseudoTerminalStatefulSetName,
		pseudoTerminalServicePort,
	)
	recreateServices()
	updatePseudoTerminalsList()

	go debugPrint()

	router := gin.Default()

	router.POST("/getPseudoTerminalAddress", getPseudoTerminalAddress)
	router.POST("/killUserPod", killUserPod)

	logManagerf("starting gin server addr=%s", ginAddr)
	if err := router.Run(ginAddr); err != nil {
		logManagerf("gin server exited with error: %v", err)
	}
}
