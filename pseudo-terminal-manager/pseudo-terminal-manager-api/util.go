package main

import (
	"fmt"
	"log"
	"strings"

	v1 "k8s.io/api/core/v1"
)

func check(e error) {
	if e != nil {
		log.Printf("error: %v", e)
	}
}

func logManagerf(format string, args ...any) {
	log.Printf("[manager] "+format, args...)
}

func podReadyCondition(v1pod v1.Pod) string {
	for _, condition := range v1pod.Status.Conditions {
		if condition.Type == v1.PodReady {
			return string(condition.Status)
		}
	}

	return "Unknown"
}

func serviceNodePort(svc *v1.Service) int32 {
	if svc == nil {
		return 0
	}

	for _, port := range svc.Spec.Ports {
		if port.NodePort != 0 {
			return port.NodePort
		}
	}

	return 0
}

func podSummary(v1pod v1.Pod) string {
	return fmt.Sprintf(
		"name=%s phase=%s ready=%s node=%s rv=%s deletionTimestamp=%t",
		v1pod.Name,
		v1pod.Status.Phase,
		podReadyCondition(v1pod),
		v1pod.Spec.NodeName,
		v1pod.ResourceVersion,
		v1pod.DeletionTimestamp != nil,
	)
}

func pseudoTerminalSummary(pt *pseudoTerminal) string {
	if pt == nil {
		return "<nil>"
	}

	serviceName := "<nil>"
	if pt.svc != nil {
		serviceName = pt.svc.Name
	}

	return fmt.Sprintf(
		"pod={%s} service=%s nodePort=%d state=%s userIP=%s",
		podSummary(pt.pod),
		serviceName,
		serviceNodePort(pt.svc),
		pt.state,
		pt.userIP,
	)
}

func pseudoTerminalStateCounts() string {
	if len(pseudoTerminalList) == 0 {
		return "total=0"
	}

	counts := make(map[string]int)
	for _, pt := range pseudoTerminalList {
		counts[pt.state]++
	}

	parts := []string{fmt.Sprintf("total=%d", len(pseudoTerminalList))}
	for _, state := range []string{"ready first", "in use", "recreating"} {
		parts = append(parts, fmt.Sprintf("%s=%d", state, counts[state]))
	}

	return strings.Join(parts, " ")
}
