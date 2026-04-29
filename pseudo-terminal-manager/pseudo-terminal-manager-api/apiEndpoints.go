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
	logManagerf("http request path=%s remote=%s contentLength=%d", c.Request.URL.Path, c.ClientIP(), c.Request.ContentLength)
	if err := c.BindJSON(&incomingJson); err != nil {
		rawJson, _ := c.GetRawData()
		logManagerf("json bind failed path=%s remote=%s raw=%q err=%v", c.Request.URL.Path, c.ClientIP(), string(rawJson), err)
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("error binding json: %v", err))
		return jsonStruct{}, err
	}

	logManagerf(
		"http request parsed path=%s remote=%s ip=%q podName=%q status=%q",
		c.Request.URL.Path,
		c.ClientIP(),
		incomingJson.IP,
		incomingJson.PODNAME,
		incomingJson.STATUS,
	)
	return incomingJson, nil
}

// ENDPOINT getPseudoTerminalAddress
func getPseudoTerminalAddress(c *gin.Context) {

	incomingJson, err := bindJsonHelper(c)
	if err != nil {
		logManagerf("getPseudoTerminalAddress aborting due to bind error: %v", err)
		return
	}

	logManagerf("getPseudoTerminalAddress start clientID=%q remote=%s", incomingJson.IP, c.ClientIP())

	// reconnect
	updatePseudoTerminalsList()
	pseudoTerminalA, err := getPseudoTerminalByAny(func(pseudoTerminal pseudoTerminal) string { return pseudoTerminal.userIP },
		incomingJson.IP)
	if err == nil {
		address := pseudoTerminalA.getAddress()
		logManagerf("reconnect matched clientID=%q terminal=%s address=%s", incomingJson.IP, pseudoTerminalSummary(pseudoTerminalA), address)
		c.IndentedJSON(200, jsonStruct{
			IP:      address,
			PODNAME: pseudoTerminalA.pod.Name,
			STATUS:  "reconnecting",
		})
		return
	}
	logManagerf("no reconnect match for clientID=%q: %v", incomingJson.IP, err)

	// first connect
	pseudoTerminalB, err := getOrCreateAvailablePseudoTerminal()
	if err == nil {
		logManagerf("allocating terminal clientID=%q terminalBefore=%s", incomingJson.IP, pseudoTerminalSummary(pseudoTerminalB))
		updateState(pseudoTerminalB, "in use")
		pseudoTerminalB.userIP = incomingJson.IP
		address := pseudoTerminalB.getAddress()
		logManagerf("allocation complete clientID=%q terminalAfter=%s address=%s", incomingJson.IP, pseudoTerminalSummary(pseudoTerminalB), address)

		c.IndentedJSON(200, jsonStruct{
			IP:      address,
			PODNAME: pseudoTerminalB.pod.Name,
			STATUS:  "first connect",
		})
		return
	}
	logManagerf("no pseudo-terminal available for clientID=%q err=%v", incomingJson.IP, err)

	c.IndentedJSON(503, jsonStruct{
		IP:      "NONE",
		PODNAME: "NONE",
		STATUS:  "no pseudo-terminal available",
	})

	return
}

func (pseudoTerminal *pseudoTerminal) getAddress() string {
	logManagerf("resolving address for terminal=%s", pseudoTerminalSummary(pseudoTerminal))

	nodeObj, err := clientset.CoreV1().Nodes().Get(context.Background(),
		pseudoTerminal.pod.Spec.NodeName, metav1.GetOptions{})
	check(err)

	var nodeIP string
	for _, addr := range nodeObj.Status.Addresses {
		if addr.Type == "ExternalIP" {
			nodeIP = addr.Address
			break
		}
	}

	if nodeIP == "" {
		for _, addr := range nodeObj.Status.Addresses {
			if addr.Type == "InternalIP" {
				nodeIP = addr.Address
				break
			}
		}
	}

	if nodeIP == "" {
		for _, addr := range nodeObj.Status.Addresses {
			if addr.Type == "Hostname" {
				nodeIP = addr.Address
				break
			}
		}
	}

	var nodePort string
	for _, port := range pseudoTerminal.svc.Spec.Ports {
		nodePort = strconv.Itoa(int(port.NodePort))
	}

	address := fmt.Sprintf("%v:%v", nodeIP, nodePort)
	logManagerf("resolved address pod=%s nodeIP=%s nodePort=%s address=%s", pseudoTerminal.pod.Name, nodeIP, nodePort, address)
	return address
}

// ENDPOINT killUserPod
func killUserPod(c *gin.Context) {
	incomingJson, err := bindJsonHelper(c)
	if err != nil {
		logManagerf("killUserPod aborting due to bind error: %v", err)
		return
	}
	logManagerf("killUserPod requested podName=%q remote=%s", incomingJson.PODNAME, c.ClientIP())

	pseudoTerminal, err := getPseudoTerminalByAny(func(p pseudoTerminal) string {
		return p.pod.Name
	}, incomingJson.PODNAME)

	if err != nil {
		logManagerf("killUserPod pod=%q not found in pseudoTerminalList err=%v", incomingJson.PODNAME, err)
		return
	}
	logManagerf("killUserPod matched terminal=%s", pseudoTerminalSummary(pseudoTerminal))

	updateState(pseudoTerminal, "recreating")

	// delete the pod
	logManagerf("deleting pod name=%s namespace=%s", incomingJson.PODNAME, namespace)
	err = clientset.CoreV1().Pods(namespace).Delete(context.TODO(), incomingJson.PODNAME, metav1.DeleteOptions{})
	if err != nil {
		log.Fatal(err)
	}
	logManagerf("delete submitted for pod name=%s; waiting for replacement lifecycle", incomingJson.PODNAME)

	go waitUpdatePseudoTerminal(incomingJson.PODNAME, pseudoTerminal)

}

var runningFilter *filter

func getFilter(firstParam *filterParam) (*filter, bool) {
	if runningFilter == nil {
		logManagerf("starting new watch filter desc=%s", firstParam.desc)
		runningFilter = newFilter(getEventChan(), firstParam)
		return runningFilter, false
	}
	logManagerf("reusing running filter for desc=%s", firstParam.desc)
	return runningFilter, true
}

func waitUpdatePseudoTerminal(podNameToFilter string, pseudoTerminal *pseudoTerminal) {
	logManagerf("waitUpdatePseudoTerminal start podName=%s terminal=%s", podNameToFilter, pseudoTerminalSummary(pseudoTerminal))

	fp := filterParam{

		desc: podNameToFilter,
		pass: func(event watch.Event, filterDone chan any) bool {
			pod, castOk := event.Object.(*v1.Pod)
			if !castOk {
				logManagerf("watch filter cast failure for desc=%s eventType=%s; destroying filter", podNameToFilter, event.Type)
				runningFilter = nil
				close(filterDone)
				return false
			}
			logManagerf("watch event desc=%s type=%s pod={%s}", podNameToFilter, event.Type, podSummary(*pod))
			if pod.Name == podNameToFilter {
				return true
			}
			return false
		},
		outChan: make(chan watch.Event),
	}

	filter, isRunning := getFilter(&fp)

	if !isRunning {
		logManagerf("launching watch filter loop for pod=%s", podNameToFilter)
		go filter.runFilter()
	} else {
		logManagerf("registering additional filter param for pod=%s", podNameToFilter)
		filter.paramStream <- fp
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go waitPatternPendingRunning(&fp, &wg)

	wg.Wait()

	logManagerf("replacement pod observed as ready again for podName=%s; setting state to ready first", podNameToFilter)
	updateState(pseudoTerminal, "ready first")

}

func waitPatternPendingRunning(fp *filterParam, wg *sync.WaitGroup) {
	logManagerf("waiting for Pending->Running pattern desc=%s", fp.desc)
	var lastPhase string
	for {
		select {
		case event := <-fp.outChan:
			pod, _ := event.Object.(*v1.Pod)

			currentPhase := string(pod.Status.Phase)
			logManagerf("phase transition candidate desc=%s eventType=%s lastPhase=%s currentPhase=%s pod={%s}", fp.desc, event.Type, lastPhase, currentPhase, podSummary(*pod))

			if lastPhase == "Pending" && currentPhase == "Running" {
				logManagerf("Pending->Running pattern found desc=%s pod=%s", fp.desc, pod.Name)
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
			logManagerf("getFpIndex matched desc=%s index=%d", inputFp.desc, i)
			return i
		}
	}

	log.Fatalf("getFpIndex: filterParam %v not in filter.params", inputFp.desc)
	return -1
}
