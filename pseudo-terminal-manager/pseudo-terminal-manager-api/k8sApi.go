package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
)

var pseudoTerminalList []*pseudoTerminal
var namespace string
var pseudoTerminalStatefulSetName string
var pseudoTerminalServicePort int
var config *rest.Config
var clientset *kubernetes.Clientset

const minReadyFirstPseudoTerminals = 1
const waitForAvailablePseudoTerminalTimeout = 2 * time.Minute
const waitForAvailablePseudoTerminalPollInterval = time.Second

func init() {

	namespace = getEnv("POD_NAMESPACE", "pseudo-terminals")
	pseudoTerminalStatefulSetName = getEnv("PSEUDO_TERMINAL_STATEFULSET_NAME",
		getEnv("PTY_STATEFULSET_NAME", "pseudo-terminals-set"))
	pseudoTerminalServicePort = getEnvAsInt("PSEUDO_TERMINAL_SERVICE_PORT",
		getEnvAsInt("PTY_SERVICE_PORT", 7070))

	config, err := rest.InClusterConfig()
	check(err)

	clientset, err = kubernetes.NewForConfig(config)
	check(err)

}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvAsInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("invalid %s value %q, using %d", key, value, fallback)
		return fallback
	}

	return intValue
}

func isManagedPseudoTerminalName(name string) bool {
	return strings.HasPrefix(name, pseudoTerminalStatefulSetName+"-")
}

func isManagedPseudoTerminalServiceName(name string) bool {
	return isManagedPseudoTerminalName(name) && strings.HasSuffix(name, "-npsvc")
}

type pseudoTerminal struct {
	pod    v1.Pod
	svc    *v1.Service
	state  string
	userIP string
}

func updateState(pseudoTerminal *pseudoTerminal, newState string) {
	var isValid bool
	for _, s := range []string{"ready first", "recreating", "in use"} {
		if newState == s {
			isValid = true
		}
	}
	if !isValid {
		log.Fatalf("%v IS NOT A VALID STATE\n", newState)
	}

	pseudoTerminal.state = newState

	if err := checkToScale(); err != nil {
		log.Printf("failed to scale pseudo-terminals after state update: %v", err)
	}
}

// checkToScale ensures there is always at least one spare pseudo-terminal in
// state `ready first`. This only scales up; it does not scale down in order to
// avoid terminating active sessions unexpectedly.
func checkToScale() error {
	return scale()
}

func scale() error {
	updatePseudoTerminalsList()

	if countPseudoTerminalsInState("ready first") >= minReadyFirstPseudoTerminals {
		return nil
	}

	return setPseudoTerminalReplicas(int32(len(pseudoTerminalList) + 1))
}

func setPseudoTerminalReplicas(replicaCount int32) error {
	statefulSetClient := clientset.AppsV1().StatefulSets(namespace)
	statefulSet, err := statefulSetClient.Get(context.Background(),
		pseudoTerminalStatefulSetName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	currentReplicas := int32(1)
	if statefulSet.Spec.Replicas != nil {
		currentReplicas = *statefulSet.Spec.Replicas
	}

	if currentReplicas == replicaCount {
		return nil
	}

	statefulSet.Spec.Replicas = &replicaCount
	_, err = statefulSetClient.Update(context.Background(), statefulSet, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	log.Printf("scaled pseudo-terminal StatefulSet from %d to %d replicas", currentReplicas, replicaCount)
	return nil
}

func countPseudoTerminalsInState(state string) int {
	count := 0
	for _, pseudoTerminal := range pseudoTerminalList {
		if pseudoTerminal.state == state {
			count++
		}
	}
	return count
}

func isPodReady(v1pod v1.Pod) bool {
	if v1pod.Status.Phase != v1.PodRunning {
		return false
	}

	for _, condition := range v1pod.Status.Conditions {
		if condition.Type == v1.PodReady {
			return condition.Status == v1.ConditionTrue
		}
	}

	return false
}

func getAvailablePseudoTerminal() (*pseudoTerminal, error) {
	for _, pseudoTerminal := range pseudoTerminalList {
		if pseudoTerminal.state == "ready first" && isPodReady(pseudoTerminal.pod) {
			return pseudoTerminal, nil
		}
	}

	return nil, fmt.Errorf("no ready pseudo-terminal available")
}

func getOrCreateAvailablePseudoTerminal() (*pseudoTerminal, error) {
	updatePseudoTerminalsList()

	if pseudoTerminal, err := getAvailablePseudoTerminal(); err == nil {
		return pseudoTerminal, nil
	}

	if err := checkToScale(); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(waitForAvailablePseudoTerminalTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(waitForAvailablePseudoTerminalPollInterval)
		updatePseudoTerminalsList()

		if pseudoTerminal, err := getAvailablePseudoTerminal(); err == nil {
			return pseudoTerminal, nil
		}
	}

	return nil, fmt.Errorf("timed out waiting for an available pseudo-terminal")
}

type pseudoTerminalFn func(pseudoTerminal) string

func getPseudoTerminalByAny(inFn pseudoTerminalFn, match string) (*pseudoTerminal, error) {
	var res string
	for _, pseudoTerminal := range pseudoTerminalList {
		res = inFn(*pseudoTerminal)
		if res == match {
			return pseudoTerminal, nil
		}
	}
	return nil, fmt.Errorf("no match with %v and %v\n", res, match)
}

func getPodByName(name string) (*v1.Pod, error) {

	v1Pod, err := clientset.CoreV1().Pods(namespace).Get(context.Background(),
		name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return v1Pod, nil

}

// ran once at startup.
// deletes then reassigns services to ensure revision hash selector is accurate
func recreateServices() {
	servicesClient := clientset.CoreV1().Services(namespace)
	svcList, err := servicesClient.List(context.TODO(), metav1.ListOptions{})
	check(err)
	deletePolicy := metav1.DeletePropagationForeground

	for _, s := range svcList.Items {
		if isManagedPseudoTerminalServiceName(s.Name) {
			if err := servicesClient.Delete(context.TODO(), s.Name, metav1.DeleteOptions{
				PropagationPolicy: &deletePolicy,
			}); err != nil {
				panic(err)
			}
		}
	}

}

func updatePseudoTerminalsList() {

	podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	check(err)

	fmt.Printf("len(podList.Items): %v\n", len(podList.Items)) //t

	currentPseudoTerminals := make(map[string]*pseudoTerminal, len(pseudoTerminalList))
	for _, pseudoTerminal := range pseudoTerminalList {
		currentPseudoTerminals[pseudoTerminal.pod.Name] = pseudoTerminal
	}

	nextPseudoTerminalList := make([]*pseudoTerminal, 0, len(podList.Items))
	for _, v1pod := range podList.Items {
		if !isManagedPseudoTerminalName(v1pod.Name) {
			continue
		}

		if pseudoTerminal, ok := currentPseudoTerminals[v1pod.Name]; ok {
			pseudoTerminal.pod = v1pod
			pseudoTerminal.svc = getAssociatedSvc(&v1pod)
			nextPseudoTerminalList = append(nextPseudoTerminalList, pseudoTerminal)
			continue
		}

		nextPseudoTerminalList = append(nextPseudoTerminalList, &pseudoTerminal{
			pod:    v1pod,
			svc:    getAssociatedSvc(&v1pod),
			state:  "ready first",
			userIP: "none",
		})
	}

	pseudoTerminalList = nextPseudoTerminalList
}

// used to populate v1.Service in pseudoTerminalList
// POSSIBLE BUG] will recur forever if it needs to create a new service but encounters an error
func getAssociatedSvc(v1pod *v1.Pod) *v1.Service {

	svcName := fmt.Sprintf("%v-npsvc", v1pod.Name)
	svcObj, err := clientset.CoreV1().Services(namespace).Get(context.Background(),
		svcName, metav1.GetOptions{})

	// will the above function err if there is not an existing pod?
	if err != nil {
		fmt.Printf("creating service for %v\n", v1pod.Name)
		exposePod(v1pod)
		return getAssociatedSvc(v1pod)
	}

	return svcObj

}

func (inPseudoTerminal *pseudoTerminal) print() {

	var nodePort string
	for _, port := range inPseudoTerminal.svc.Spec.Ports {
		// fmt.Printf("port: %v\n", port) //t
		nodePort = strconv.Itoa(int(port.NodePort))
	}

	fmt.Printf("%v, %v, %v, %v, %v\n", inPseudoTerminal.pod.Name, inPseudoTerminal.svc.Name,
		inPseudoTerminal.state, nodePort, inPseudoTerminal.userIP)
}

func printList() {
	for _, p := range pseudoTerminalList {
		p.print()
	}
}

func exposePod(v1pod *v1.Pod) {

	svcName := v1pod.Name + "-npsvc"
	selectorLabels := map[string]string{
		"statefulset.kubernetes.io/pod-name": v1pod.Name,
	}
	serviceLabels := map[string]string{
		"statefulset.kubernetes.io/pod-name": v1pod.Name,
	}

	for _, key := range []string{
		"app.kubernetes.io/name",
		"app.kubernetes.io/instance",
		"app.kubernetes.io/component",
	} {
		if value := v1pod.Labels[key]; value != "" {
			serviceLabels[key] = value
		}
	}

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: namespace,
			Labels:    serviceLabels,
		},
		Spec: v1.ServiceSpec{
			Type:     v1.ServiceTypeNodePort,
			Selector: selectorLabels,
			Ports: []v1.ServicePort{{
				Port:       int32(pseudoTerminalServicePort),
				TargetPort: intstr.FromInt(pseudoTerminalServicePort),
			}},
		},
	}

	clientset.CoreV1().Services(namespace).Create(context.TODO(), service,
		metav1.CreateOptions{})
}
