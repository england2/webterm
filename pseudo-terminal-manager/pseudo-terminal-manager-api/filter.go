package main

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type filterParam struct {
	desc    string
	pass    func(watch.Event, chan any) bool
	outChan chan watch.Event
}

type filter struct {
	params       []*filterParam
	done         chan any
	remIndexChan chan int
	inChan       <-chan watch.Event
	paramStream  chan filterParam
}

func (fil *filter) runFilter() {
	for {
		select {
		case paramToAppend := <-fil.paramStream:
			fil.params = append(fil.params, &paramToAppend)
			fmt.Printf("params: %v\n", fil.params)
		case indexToRemove := <-fil.remIndexChan:
			fmt.Printf("removing filterParam %v\n", fil.params[indexToRemove].desc) //t
			fmt.Println(fil.params)                                                 //t
			fil.params = remove(fil.params, indexToRemove)
			fmt.Println(fil.params) //t
		case event := <-fil.inChan:
			for _, fp := range fil.params {
				if fp.pass(event, fil.done) {
					fp.outChan <- event
				}
			}
		case <-fil.done:
			return
		default:
			if len(fil.params) == 0 {
				fmt.Println("len(fil.params) == 0. closing filter") //t
				close(fil.done)
				runningFilter = nil
			}
		}
	}
}

func newFilter(inChan <-chan watch.Event, firstParam *filterParam) *filter {
	return &filter{
		params:       []*filterParam{firstParam},
		done:         make(chan any),
		remIndexChan: make(chan int),
		inChan:       inChan,
		paramStream:  make(chan filterParam),
	}
}
func remove(s []*filterParam, i int) []*filterParam {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
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
