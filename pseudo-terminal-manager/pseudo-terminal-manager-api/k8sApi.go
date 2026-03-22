package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

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
	for _, s := range []string{"ready first", "recreatng", "in use"} {
		if newState == s {
			isValid = true
		}
	}
	if !isValid {
		log.Fatalf("%v IS NOT A VALID STATE\n", newState)
	}

	checkToScale()

}

// checkToScale ensures the following is true:
// - there is least one pod in state `ready first`. (ex: if there are 4 pods in
// state `in use`, the StatefulSet will scale up to 5.)
// - there are not more than 4 pods in state `ready first` (ex: if there are more
// than 5 pods in state `ready first`, the StatefulSet will scale down to 3.)
func checkToScale() {

	scale()
}

func scale() {

	// https://stackoverflow.com/questions/61653702/scale-deployment-replicas-with-kubernetes-go-client

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

	for _, v1pod := range podList.Items {
		if isManagedPseudoTerminalName(v1pod.Name) {
			// errors if pod is not in pseudoTerminalList
			_, err := getPseudoTerminalByAny(func(p pseudoTerminal) string {
				return p.pod.Name
			}, v1pod.Name)

			if err != nil {
				pseudoTerminalList = append(pseudoTerminalList, &pseudoTerminal{
					pod:    v1pod,
					svc:    getAssociatedSvc(&v1pod),
					state:  "ready first",
					userIP: "none",
				})

			}
		}
	}
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
	appLabel := v1pod.Labels["app"]
	labelSelectorMap := map[string]string{"app": appLabel,
		"statefulset.kubernetes.io/pod-name": v1pod.Name}

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: namespace,
			Labels:    labelSelectorMap,
		},
		Spec: v1.ServiceSpec{
			Type:     v1.ServiceTypeNodePort,
			Selector: labelSelectorMap,
			Ports: []v1.ServicePort{{
				Port:       int32(pseudoTerminalServicePort),
				TargetPort: intstr.FromInt(pseudoTerminalServicePort),
			}},
		},
	}

	clientset.CoreV1().Services(namespace).Create(context.TODO(), service,
		metav1.CreateOptions{})
}
