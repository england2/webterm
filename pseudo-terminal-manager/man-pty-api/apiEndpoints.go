package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type jsonStruct struct {
	IP      string `json:"ip"`
	PODNAME string `json:"podName"`
	STATUS  string `json:"status"`
}

func bindJsonHelper(c *gin.Context) (jsonStruct, error) {
	var incomingJson jsonStruct
	if err := c.BindJSON(&incomingJson); err != nil {
		rawJson, _ := c.GetRawData()
		fmt.Printf("error binding json. raw data below \n%v", string(rawJson))
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("error binding json: %v", err))
		return jsonStruct{}, err
	}

	return incomingJson, nil
}

// ENDPOINT getIP
func getIP(c *gin.Context) {

	incomingJson, err := bindJsonHelper(c)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("in getIP")                              //t
	fmt.Printf("incomingJson.IP: %v\n", incomingJson.IP) //t

	// reconnect
	ptyA, err := getPtyByAny(func(pty pty) string { return pty.userIP },
		incomingJson.IP)
	if err == nil {
		c.IndentedJSON(200, jsonStruct{
			IP:      ptyA.getAddress(),
			PODNAME: ptyA.pod.Name,
			STATUS:  "reconnecting",
		})
		return
	}

	// first connect
	ptyB, err := getPtyByAny(func(pty pty) string { return pty.state },
		"ready first")
	if err == nil {
		c.IndentedJSON(200, jsonStruct{
			IP:      ptyB.getAddress(),
			PODNAME: ptyB.pod.Name,
			STATUS:  "first connect",
		})
		ptyB.state = "in use"
		ptyB.userIP = incomingJson.IP
		return
	}

	// TODO
	// error
	c.IndentedJSON(500, jsonStruct{
		IP:      "NONE",
		PODNAME: "NONE",
		STATUS:  "DEFAULT CASE ERROR",
	})

	return
}

func (pty *pty) getAddress() string {

	nodeObj, err := clientset.CoreV1().Nodes().Get(context.Background(),
		pty.pod.Spec.NodeName, metav1.GetOptions{})
	check(err)

	var nodeIP string
	for _, addr := range nodeObj.Status.Addresses {
		if addr.Type == "ExternalIP" {
			nodeIP = addr.Address
		}
	}

	var nodePort string
	for _, port := range pty.svc.Spec.Ports {
		nodePort = strconv.Itoa(int(port.NodePort))
	}

	return fmt.Sprintf("%v:%v", nodeIP, nodePort)
}

// ENDPOINT killUserPod
func killUserPod(c *gin.Context) {
	fmt.Println("in killUserPod")

	incomingJson, err := bindJsonHelper(c)
	if err != nil {
		fmt.Println(err)
		return
	}

	pty, err := getPtyByAny(func(p pty) string {
		return p.pod.Name
	}, incomingJson.PODNAME)

	if err != nil {
		log.Printf("%v not found in ptyList", incomingJson.PODNAME)
	}

	pty.state = "recreating"

	// delete the pod
	err = clientset.CoreV1().Pods(namespace).Delete(context.TODO(), incomingJson.PODNAME, metav1.DeleteOptions{})
	if err != nil {
		log.Fatal(err)
	}

	go waitUpdatePty(incomingJson.PODNAME, pty)

}

var runningFilter *filter

func getFilter(firstParam *filterParam) (*filter, bool) {
	if runningFilter == nil {
		fmt.Println("starting filter")
		runningFilter = newFilter(getEventChan(), firstParam)
		return runningFilter, false
	}
	fmt.Println("returning running filter")
	return runningFilter, true
}

func waitUpdatePty(podNameToFilter string, pty *pty) {

	fp := filterParam{

		desc: podNameToFilter,
		pass: func(event watch.Event, filterDone chan any) bool {
			pod, castOk := event.Object.(*v1.Pod)
			if !castOk {
				log.Print("Fatal cast! destroying filter")
				runningFilter = nil
				close(filterDone)
				return false
			}
			if pod.Name == podNameToFilter {
				return true
			}
			return false
		},
		outChan: make(chan watch.Event),
	}

	filter, isRunning := getFilter(&fp)

	if !isRunning {
		go filter.runFilter()
	} else {
		fmt.Println("filter.paramStream <- fp")
		filter.paramStream <- fp
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go waitPatternPendingRunning(&fp, &wg)

	wg.Wait()

	fmt.Printf("setting %v state to [ready first]\n", podNameToFilter)
	pty.state = "ready first"

}

func waitPatternPendingRunning(fp *filterParam, wg *sync.WaitGroup) {

	var lastPhase string
	for {
		select {
		case event := <-fp.outChan:
			pod, _ := event.Object.(*v1.Pod)

			currentPhase := string(pod.Status.Phase)

			if lastPhase == "Pending" && currentPhase == "Running" {
				fmt.Println("pattern found") //t
				runningFilter.remIndexChan <- runningFilter.getFpIndex(fp)
				wg.Done()
				return
			}
			lastPhase = currentPhase
		}
	}
}

func (fil *filter) getFpIndex(inputFp *filterParam) int {

	for i, fp := range fil.params {
		if inputFp.outChan == fp.outChan {
			return i
		}
	}

	log.Fatalf("getFpIndex: filterParam %v not in filter.params", inputFp.desc)
	return -1
}
