package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
)

type filterParam struct {
	pass    func(watch.Event) bool
	outChan chan watch.Event
}

type filter struct {
	params      []*filterParam
	done        chan any
	inChan      <-chan watch.Event
	paramStream chan filterParam
}

// go func
func (fil *filter) runFilter() {
	for {
		select {
		case <-fil.done:
			return
		// BUG fps are not getting in
		case paramToAppend := <-fil.paramStream:
			fil.params = append(fil.params, &paramToAppend)
			fmt.Printf("params:. %v\n", fil.params)
		case event := <-fil.inChan:
			for _, fp := range fil.params {
				if fp.pass(event) {
					fp.outChan <- event
				}
			}
		default:
		}
	}
}

func newFilter(inChan <-chan watch.Event) *filter {
	var params []*filterParam
	done := make(chan any) // podRunningWatch done
	paramStream := make(chan filterParam)

	return &filter{
		params:      params,
		done:        done,
		inChan:      inChan,
		paramStream: paramStream,
	}
}

// // filterChan paste // ==========================
var config *rest.Config
var clientset *kubernetes.Clientset
var namespace string

func init() {

	namespace = "ptys"

	config, err := rest.InClusterConfig()
	check(err)

	clientset, err = kubernetes.NewForConfig(config)
	check(err)

}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

// TODO? cast event to pod only once, and pass pods through the filter
func makeStringFilter(filterString string) (filterParam, chan watch.Event) {
	outChan := make(chan watch.Event)
	fp := filterParam{
		pass: func(event watch.Event) bool {
			pod, _ := event.Object.(*v1.Pod)
			fmt.Printf("pod.Name: %v\n", pod.Name) //t
			if pod.Name == string(filterString) {
				return true
			}
			return false
		},
		outChan: outChan,
	}

	return fp, outChan
}

func printChanPhase(done chan any, ch chan watch.Event) { //t
	for {
		select {
		case <-done:
			return
		case event := <-ch:
			pod, _ := event.Object.(*v1.Pod)
			fmt.Printf("pod name, phase: %v:%v\n", pod.Name, pod.Status.Phase)
		default:
		}
	}
}

func main() {

	fp_set0 := filterParam{
		pass: func(event watch.Event) bool {
			pod, castOk := event.Object.(*v1.Pod)
			if !castOk {
				log.Fatal("Fatal cast!")
			}
			fmt.Printf("pod.Status.Phase: %v\n", pod.Status.Phase) //t is data getting into the pass filter check?
			fmt.Printf("pod.Name: %v\n", pod.Name)                 //t
			if pod.Name == "ptys-set-0" {
				return true
			}
			return false
		},
		outChan: make(chan watch.Event),
	}

	filter_podWatch := newFilter(getEventChan()) // inChan begins receiving immediately

	go filter_podWatch.runFilter()

	// manualParams := []*fp{&fp_set0}
	// fmt.Printf("manualParams: %v\n", manualParams) //t help

	// filter_podWatch.params = manualParams
	filter_podWatch.paramStream <- fp_set0

	// consumes fp_set0's filtered channel
	var wg sync.WaitGroup
	wg.Add(1)
	updateSet := func() {
		fmt.Println("updating pty-set-0 to ready first")
	}

	waitPatternPendingRunning(make(chan any), fp_set0.outChan, &wg, updateSet)

	// printChanPhase(make(chan any), fp_set0.outChan)

}

func getEventChan() <-chan watch.Event {
	var api = clientset.CoreV1().Pods(namespace)
	pods, err := api.List(context.TODO(), metav1.ListOptions{})
	check(err)
	resourceVersion := pods.ListMeta.ResourceVersion
	watcher, err := api.Watch(context.TODO(), metav1.ListOptions{ResourceVersion: resourceVersion})
	check(err)
	return watcher.ResultChan()
}

// waits until the Pending -> Running phase pattern is found.
// consumer of a filtered watch.Event stream.
// syncing with program
// internal: When the pattern is found, run a function inside here that affects the external state (ptysList)
// external: Using wg, block external operation until the pattern is found
func waitPatternPendingRunning(done chan any, eventInputStream chan watch.Event, wg *sync.WaitGroup, callBack func()) {

	var lastPhase string
	for {
		select {
		case <-done:
			fmt.Println("closing waitPatternPendingRunning")
			wg.Done()
			return
		case event := <-eventInputStream:
			fmt.Println(event)

			pod, castOk := event.Object.(*v1.Pod)
			if !castOk {
				log.Fatal("Fatal cast!")
			}

			currentPhase := string(pod.Status.Phase)

			//t do they get assigned; are they in the right order?
			fmt.Printf("currentPhase: %v\n", currentPhase)
			fmt.Printf("lastPhase: %v\n", lastPhase)
			//t

			if lastPhase == "Pending" && currentPhase == "Running" {
				callBack()
				// wg.Done()
				return
			}
			lastPhase = currentPhase
		}
	}
}
