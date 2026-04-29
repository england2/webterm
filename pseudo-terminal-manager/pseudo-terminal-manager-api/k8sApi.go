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
	logManagerf(
		"k8s init namespace=%s statefulSet=%s servicePort=%d",
		namespace,
		pseudoTerminalStatefulSetName,
		pseudoTerminalServicePort,
	)

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

	oldState := pseudoTerminal.state
	pseudoTerminal.state = newState
	logManagerf("state update pod=%s old=%s new=%s terminal=%s", pseudoTerminal.pod.Name, oldState, newState, pseudoTerminalSummary(pseudoTerminal))

	if err := checkToScale(); err != nil {
		log.Printf("failed to scale pseudo-terminals after state update: %v", err)
	}
}

// checkToScale ensures there is always at least one spare pseudo-terminal in
// state `ready first`. This only scales up; it does not scale down in order to
// avoid terminating active sessions unexpectedly.
func checkToScale() error {
	logManagerf("checkToScale invoked currentStates=%s", pseudoTerminalStateCounts())
	return scale()
}

func scale() error {
	logManagerf("scale start currentStates=%s", pseudoTerminalStateCounts())
	updatePseudoTerminalsList()

	readyCount := countPseudoTerminalsInState("ready first")
	logManagerf("scale evaluated readyFirst=%d minReadyFirst=%d total=%d", readyCount, minReadyFirstPseudoTerminals, len(pseudoTerminalList))
	if readyCount >= minReadyFirstPseudoTerminals {
		logManagerf("scale skipped because enough ready terminals exist")
		return nil
	}

	logManagerf("scale requesting replicas=%d because no spare ready terminal exists", len(pseudoTerminalList)+1)
	return setPseudoTerminalReplicas(int32(len(pseudoTerminalList) + 1))
}

func setPseudoTerminalReplicas(replicaCount int32) error {
	logManagerf("setPseudoTerminalReplicas requested replicaCount=%d", replicaCount)
	statefulSetClient := clientset.AppsV1().StatefulSets(namespace)
	statefulSet, err := statefulSetClient.Get(context.Background(),
		pseudoTerminalStatefulSetName, metav1.GetOptions{})
	if err != nil {
		logManagerf("failed to fetch StatefulSet name=%s err=%v", pseudoTerminalStatefulSetName, err)
		return err
	}

	currentReplicas := int32(1)
	if statefulSet.Spec.Replicas != nil {
		currentReplicas = *statefulSet.Spec.Replicas
	}

	if currentReplicas == replicaCount {
		logManagerf("replica update skipped; StatefulSet already at %d replicas", currentReplicas)
		return nil
	}

	statefulSet.Spec.Replicas = &replicaCount
	_, err = statefulSetClient.Update(context.Background(), statefulSet, metav1.UpdateOptions{})
	if err != nil {
		logManagerf("failed updating StatefulSet replicas from %d to %d err=%v", currentReplicas, replicaCount, err)
		return err
	}

	logManagerf("scaled pseudo-terminal StatefulSet from %d to %d replicas", currentReplicas, replicaCount)
	return nil
}

func countPseudoTerminalsInState(state string) int {
	count := 0
	for _, pseudoTerminal := range pseudoTerminalList {
		if pseudoTerminal.state == state {
			count++
		}
	}
	logManagerf("countPseudoTerminalsInState state=%s count=%d", state, count)
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
	logManagerf("searching for available pseudo-terminal among %d terminals", len(pseudoTerminalList))
	for _, pseudoTerminal := range pseudoTerminalList {
		logManagerf("availability check terminal=%s isPodReady=%t", pseudoTerminalSummary(pseudoTerminal), isPodReady(pseudoTerminal.pod))
		if pseudoTerminal.state == "ready first" && isPodReady(pseudoTerminal.pod) {
			logManagerf("available terminal found terminal=%s", pseudoTerminalSummary(pseudoTerminal))
			return pseudoTerminal, nil
		}
	}

	logManagerf("no ready pseudo-terminal available after scanning %d terminals", len(pseudoTerminalList))
	return nil, fmt.Errorf("no ready pseudo-terminal available")
}

func getOrCreateAvailablePseudoTerminal() (*pseudoTerminal, error) {
	logManagerf("getOrCreateAvailablePseudoTerminal start")
	updatePseudoTerminalsList()

	if pseudoTerminal, err := getAvailablePseudoTerminal(); err == nil {
		logManagerf("getOrCreateAvailablePseudoTerminal returning existing terminal=%s", pseudoTerminalSummary(pseudoTerminal))
		return pseudoTerminal, nil
	}

	if err := checkToScale(); err != nil {
		logManagerf("getOrCreateAvailablePseudoTerminal scale failed err=%v", err)
		return nil, err
	}

	deadline := time.Now().Add(waitForAvailablePseudoTerminalTimeout)
	logManagerf("waiting for new terminal until deadline=%s", deadline.Format(time.RFC3339))
	for time.Now().Before(deadline) {
		time.Sleep(waitForAvailablePseudoTerminalPollInterval)
		updatePseudoTerminalsList()

		if pseudoTerminal, err := getAvailablePseudoTerminal(); err == nil {
			logManagerf("newly available terminal found terminal=%s", pseudoTerminalSummary(pseudoTerminal))
			return pseudoTerminal, nil
		}
	}

	logManagerf("timed out waiting for an available pseudo-terminal after %s", waitForAvailablePseudoTerminalTimeout)
	return nil, fmt.Errorf("timed out waiting for an available pseudo-terminal")
}

type pseudoTerminalFn func(pseudoTerminal) string

func getPseudoTerminalByAny(inFn pseudoTerminalFn, match string) (*pseudoTerminal, error) {
	var res string
	logManagerf("getPseudoTerminalByAny start match=%q total=%d", match, len(pseudoTerminalList))
	for _, pseudoTerminal := range pseudoTerminalList {
		res = inFn(*pseudoTerminal)
		logManagerf("getPseudoTerminalByAny compare candidate=%q terminal=%s", res, pseudoTerminalSummary(pseudoTerminal))
		if res == match {
			logManagerf("getPseudoTerminalByAny matched terminal=%s", pseudoTerminalSummary(pseudoTerminal))
			return pseudoTerminal, nil
		}
	}
	logManagerf("getPseudoTerminalByAny no match lastCandidate=%q target=%q", res, match)
	return nil, fmt.Errorf("no match with %v and %v\n", res, match)
}

func getPodByName(name string) (*v1.Pod, error) {
	logManagerf("getPodByName name=%s", name)

	v1Pod, err := clientset.CoreV1().Pods(namespace).Get(context.Background(),
		name, metav1.GetOptions{})
	if err != nil {
		logManagerf("getPodByName failed name=%s err=%v", name, err)
		return nil, err
	}

	logManagerf("getPodByName found pod={%s}", podSummary(*v1Pod))
	return v1Pod, nil

}

// ran once at startup.
// deletes then reassigns services to ensure revision hash selector is accurate
func recreateServices() {
	logManagerf("recreateServices start namespace=%s", namespace)
	servicesClient := clientset.CoreV1().Services(namespace)
	svcList, err := servicesClient.List(context.TODO(), metav1.ListOptions{})
	check(err)
	deletePolicy := metav1.DeletePropagationForeground

	for _, s := range svcList.Items {
		if isManagedPseudoTerminalServiceName(s.Name) {
			logManagerf("recreateServices deleting managed service name=%s nodePort=%d selector=%v", s.Name, serviceNodePort(&s), s.Spec.Selector)
			if err := servicesClient.Delete(context.TODO(), s.Name, metav1.DeleteOptions{
				PropagationPolicy: &deletePolicy,
			}); err != nil {
				panic(err)
			}
		}
	}
	logManagerf("recreateServices complete")

}

func updatePseudoTerminalsList() {
	logManagerf("updatePseudoTerminalsList start previousStates=%s", pseudoTerminalStateCounts())

	podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	check(err)

	logManagerf("updatePseudoTerminalsList listed pods total=%d", len(podList.Items))

	currentPseudoTerminals := make(map[string]*pseudoTerminal, len(pseudoTerminalList))
	for _, pseudoTerminal := range pseudoTerminalList {
		currentPseudoTerminals[pseudoTerminal.pod.Name] = pseudoTerminal
	}

	nextPseudoTerminalList := make([]*pseudoTerminal, 0, len(podList.Items))
	for _, v1pod := range podList.Items {
		if !isManagedPseudoTerminalName(v1pod.Name) {
			continue
		}
		logManagerf("updatePseudoTerminalsList processing managed pod={%s}", podSummary(v1pod))

		if pseudoTerminal, ok := currentPseudoTerminals[v1pod.Name]; ok {
			logManagerf("updatePseudoTerminalsList refreshing existing terminal pod=%s oldState=%s oldUserIP=%s", v1pod.Name, pseudoTerminal.state, pseudoTerminal.userIP)
			pseudoTerminal.pod = v1pod
			pseudoTerminal.svc = getAssociatedSvc(&v1pod)
			nextPseudoTerminalList = append(nextPseudoTerminalList, pseudoTerminal)
			continue
		}

		logManagerf("updatePseudoTerminalsList discovered new terminal pod=%s; defaulting state=ready first userIP=none", v1pod.Name)
		nextPseudoTerminalList = append(nextPseudoTerminalList, &pseudoTerminal{
			pod:    v1pod,
			svc:    getAssociatedSvc(&v1pod),
			state:  "ready first",
			userIP: "none",
		})
	}

	pseudoTerminalList = nextPseudoTerminalList
	logManagerf("updatePseudoTerminalsList complete newStates=%s", pseudoTerminalStateCounts())
}

// used to populate v1.Service in pseudoTerminalList
// POSSIBLE BUG] will recur forever if it needs to create a new service but encounters an error
func getAssociatedSvc(v1pod *v1.Pod) *v1.Service {

	svcName := fmt.Sprintf("%v-npsvc", v1pod.Name)
	logManagerf("getAssociatedSvc pod=%s service=%s", v1pod.Name, svcName)
	svcObj, err := clientset.CoreV1().Services(namespace).Get(context.Background(),
		svcName, metav1.GetOptions{})

	// will the above function err if there is not an existing pod?
	if err != nil {
		logManagerf("service lookup failed pod=%s service=%s err=%v; creating replacement", v1pod.Name, svcName, err)
		exposePod(v1pod)
		return getAssociatedSvc(v1pod)
	}

	logManagerf("getAssociatedSvc found service=%s nodePort=%d selector=%v", svcObj.Name, serviceNodePort(svcObj), svcObj.Spec.Selector)
	return svcObj

}

func (inPseudoTerminal *pseudoTerminal) print() {
	logManagerf("terminal snapshot %s", pseudoTerminalSummary(inPseudoTerminal))
}

func printList() {
	logManagerf("printList start %s", pseudoTerminalStateCounts())
	for _, p := range pseudoTerminalList {
		p.print()
	}
	logManagerf("printList end")
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
	logManagerf("exposePod creating NodePort service name=%s pod=%s selector=%v labels=%v port=%d", svcName, v1pod.Name, selectorLabels, serviceLabels, pseudoTerminalServicePort)

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

	createdService, err := clientset.CoreV1().Services(namespace).Create(context.TODO(), service,
		metav1.CreateOptions{})
	if err != nil {
		logManagerf("exposePod failed service=%s pod=%s err=%v", svcName, v1pod.Name, err)
		return
	}
	logManagerf("exposePod created service=%s nodePort=%d", createdService.Name, serviceNodePort(createdService))
}
