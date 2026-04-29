package main

import (
	"context"

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
	logManagerf("filter loop started initialParams=%d", len(fil.params))
	for {
		select {
		case paramToAppend := <-fil.paramStream:
			fil.params = append(fil.params, &paramToAppend)
			logManagerf("filter param added desc=%s totalParams=%d", paramToAppend.desc, len(fil.params))
		case indexToRemove := <-fil.remIndexChan:
			logManagerf("filter param removing desc=%s index=%d totalBefore=%d", fil.params[indexToRemove].desc, indexToRemove, len(fil.params))
			fil.params = remove(fil.params, indexToRemove)
			logManagerf("filter param removed totalAfter=%d", len(fil.params))
		case event := <-fil.inChan:
			logManagerf("filter received watch event type=%s activeParams=%d", event.Type, len(fil.params))
			for _, fp := range fil.params {
				if fp.pass(event, fil.done) {
					logManagerf("filter event matched desc=%s forwarding eventType=%s", fp.desc, event.Type)
					fp.outChan <- event
				}
			}
		case <-fil.done:
			logManagerf("filter loop stopping due to done signal")
			return
		default:
			if len(fil.params) == 0 {
				logManagerf("filter loop closing because no params remain")
				close(fil.done)
				runningFilter = nil
			}
		}
	}
}

func newFilter(inChan <-chan watch.Event, firstParam *filterParam) *filter {
	logManagerf("newFilter created firstDesc=%s", firstParam.desc)
	return &filter{
		params:       []*filterParam{firstParam},
		done:         make(chan any),
		remIndexChan: make(chan int),
		inChan:       inChan,
		paramStream:  make(chan filterParam),
	}
}
func remove(s []*filterParam, i int) []*filterParam {
	logManagerf("remove helper swapping index=%d with lastIndex=%d", i, len(s)-1)
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

func getEventChan() <-chan watch.Event {
	var api = clientset.CoreV1().Pods(namespace)
	pods, err := api.List(context.TODO(), metav1.ListOptions{})
	check(err)
	resourceVersion := pods.ListMeta.ResourceVersion
	logManagerf("creating pod watch namespace=%s resourceVersion=%s podCount=%d", namespace, resourceVersion, len(pods.Items))
	watcher, err := api.Watch(context.TODO(), metav1.ListOptions{ResourceVersion: resourceVersion})
	check(err)
	logManagerf("pod watch established namespace=%s", namespace)
	return watcher.ResultChan()
}
