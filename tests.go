package kommons

import (
	"context"
	goerrors "errors"
	"fmt"
	"strings"

	"github.com/flanksource/commons/console"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

func TestDeploy(client kubernetes.Interface, ns string, deploymentName string, t *console.TestResults) {
	events := client.CoreV1().Events(ns)
	deployment, err := client.AppsV1().Deployments(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		t.Skipf(deploymentName, "deployment not found")
		return
	}
	labelMap, _ := metav1.LabelSelectorAsMap(deployment.Spec.Selector)

	pods, _ := client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelMap).String(),
	})

	if len(pods.Items) == 0 {
		t.Failf(deploymentName, "No pods found for %s", deploymentName)
	}
	fails := make([]error, 0)
	for _, pod := range pods.Items {
		err = TestPod(deploymentName, client, events, pod)
		if err != nil {
			fails = append(fails, err)
		}
	}
	if len(fails) > 0 {
		message := fmt.Sprintf("%d of %d pods failed: ", len(fails), len(pods.Items))
		for _, err := range fails {
			message += err.Error() + ". "
		}
		message = strings.TrimSuffix(message, " ")
		t.Failf(deploymentName, message)
	}
	t.Passf(deploymentName, "%d of %d pods passed", len(pods.Items), len(pods.Items))
}

func TestPod(testName string, client kubernetes.Interface, events typedv1.EventInterface, pod v1.Pod) error {
	conditions := true
	// for _, condition := range pod.Status.Conditions {
	// 	if condition.Status == v1.ConditionFalse {
	// 		t.Failf(ns, "%s => %s: %s", pod.Name, condition.Type, condition.Message)
	// 		conditions = false
	// 	}
	// }
	if conditions && pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodSucceeded {
		return nil
	} else {
		events, err := events.List(context.TODO(), metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + pod.Name,
		})
		if err != nil {
			return goerrors.New(fmt.Sprintf("%s => %s, failed to get events %+v",
				pod.Name, pod.Status.Phase, err))
		}
		msg := ""
		for _, event := range events.Items {
			if event.Type == "Normal" {
				continue
			}
			msg += fmt.Sprintf("%s: %s ", event.Reason, event.Message)
		}
		return goerrors.New(fmt.Sprintf("%s/%s=%s %s ", pod.Namespace, pod.Name, pod.Status.Phase, msg))
	}

	// check all pods running or completed with < 3 restarts
	// check unbound pvcs
	// check all pod liveness / readiness
}

func TestNamespace(client kubernetes.Interface, ns string, t *console.TestResults) {
	pods := client.CoreV1().Pods(ns)
	events := client.CoreV1().Events(ns)
	list, err := pods.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Failf(ns, "Failed to get pods for %s: %v", ns, err)
		return
	}

	if len(list.Items) == 0 {
		_, err := client.CoreV1().Namespaces().Get(context.TODO(), ns, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			t.Skipf(ns, "[%s] namespace not found, skipping", ns)
		} else {
			t.Failf(ns, "[%s] Expected pods but none running - did you deploy?", ns)
		}
		return
	}
	fails := make([]error, 0)
	for _, pod := range list.Items {
		err = TestPod(ns, client, events, pod)
		if err != nil {
			fails = append(fails, err)
		}
	}
	if len(fails) > 0 {
		message := fmt.Sprintf("%d of %d pods failed: ", len(fails), len(list.Items))
		for _, err := range fails {
			message += err.Error() + ". "
		}
		message = strings.TrimSuffix(message, " ")
		t.Failf(ns, message)
	}
	t.Passf(ns, "%d of %d pods passed", len(list.Items), len(list.Items))
}
