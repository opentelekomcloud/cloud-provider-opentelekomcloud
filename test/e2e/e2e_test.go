//go:build e2e

// Package e2e runs against a live cluster with the CCM deployed; see README.
package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/elb"
)

const (
	appLabel     = "e2e-nginx"
	pollInterval = 5 * time.Second
)

func testTimeout() time.Duration {
	if v := os.Getenv("E2E_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 10 * time.Minute
}

func newKubeClient(t *testing.T) kubernetes.Interface {
	t.Helper()
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
	if err != nil {
		t.Fatalf("cannot load kubeconfig (set KUBECONFIG or provide ~/.kube/config): %v", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("cannot create Kubernetes client: %v", err)
	}
	return client
}

func setupNamespace(ctx context.Context, t *testing.T, client kubernetes.Interface) string {
	t.Helper()
	name := fmt.Sprintf("ccm-e2e-%05d", rand.Intn(100000))
	_, err := client.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create namespace %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().Namespaces().Delete(context.Background(), name, metav1.DeleteOptions{})
	})
	return name
}

func deployNginx(ctx context.Context, t *testing.T, client kubernetes.Interface, namespace string) {
	t.Helper()
	replicas := int32(2)
	_, err := client.AppsV1().Deployments(namespace).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: appLabel},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": appLabel}},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": appLabel}},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name:  "nginx",
						Image: "nginx:1.27",
						Ports: []v1.ContainerPort{{ContainerPort: 80}},
					}},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create deployment: %v", err)
	}
}

// serviceAnnotations builds LB annotations from E2E_* env overrides.
func serviceAnnotations() map[string]string {
	ann := map[string]string{}
	if v := os.Getenv("E2E_SUBNET_ID"); v != "" {
		ann[elb.AnnotationSubnetID] = v
	}
	if v := os.Getenv("E2E_AVAILABILITY_ZONES"); v != "" {
		ann[elb.AnnotationAvailabilityZones] = v
	}
	if v := os.Getenv("E2E_EIP_BANDWIDTH"); v != "" {
		ann[elb.AnnotationEipBandwidth] = v
	}
	return ann
}

func createLBService(ctx context.Context, t *testing.T, client kubernetes.Interface, namespace, name string) {
	t.Helper()
	_, err := client.CoreV1().Services(namespace).Create(ctx, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: serviceAnnotations(),
		},
		Spec: v1.ServiceSpec{
			Type:     v1.ServiceTypeLoadBalancer,
			Selector: map[string]string{"app": appLabel},
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   v1.ProtocolTCP,
				Port:       80,
				TargetPort: intstr.FromInt32(80),
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
}

func waitForIngress(ctx context.Context, t *testing.T, client kubernetes.Interface, namespace, name string) string {
	t.Helper()
	var ingress string
	err := wait.PollUntilContextTimeout(ctx, pollInterval, testTimeout(), true, func(ctx context.Context) (bool, error) {
		svc, err := client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if len(svc.Status.LoadBalancer.Ingress) > 0 && svc.Status.LoadBalancer.Ingress[0].IP != "" {
			ingress = svc.Status.LoadBalancer.Ingress[0].IP
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("service %s/%s did not get an ingress IP: %v", namespace, name, err)
	}
	return ingress
}

func checkHTTP(t *testing.T, url string) {
	t.Helper()
	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, // #nosec G402 -- test traffic to a fresh LB
	}
	deadline := time.Now().Add(testTimeout())
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := httpClient.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("HTTP check of %s failed: %v", url, lastErr)
}

func deleteServiceAndWait(ctx context.Context, t *testing.T, client kubernetes.Interface, namespace, name string) {
	t.Helper()
	if err := client.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("failed to delete service: %v", err)
	}
	// The Service stays until the cloud LB is gone and the finalizer dropped.
	err := wait.PollUntilContextTimeout(ctx, pollInterval, testTimeout(), true, func(ctx context.Context) (bool, error) {
		_, err := client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("service %s/%s was not cleaned up: %v", namespace, name, err)
	}
}

// TestLoadBalancerService: provision, optional HTTP check, update, delete.
func TestLoadBalancerService(t *testing.T) {
	ctx := context.Background()
	client := newKubeClient(t)
	namespace := setupNamespace(ctx, t, client)
	deployNginx(ctx, t, client, namespace)

	const svcName = "e2e-lb"
	createLBService(ctx, t, client, namespace, svcName)

	ingress := waitForIngress(ctx, t, client, namespace, svcName)
	t.Logf("service got ingress IP %s", ingress)

	// The VIP is VPC-private; check HTTP only when enabled or an EIP exists.
	if os.Getenv("E2E_CHECK_HTTP") == "true" || os.Getenv("E2E_EIP_BANDWIDTH") != "" {
		checkHTTP(t, "http://"+ingress)
		t.Log("HTTP check passed")
	}

	// Reconfigure: add a second port and verify the service stays healthy.
	svc, err := client.CoreV1().Services(namespace).Get(ctx, svcName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get service: %v", err)
	}
	svc.Spec.Ports = append(svc.Spec.Ports, v1.ServicePort{
		Name:       "http-alt",
		Protocol:   v1.ProtocolTCP,
		Port:       8080,
		TargetPort: intstr.FromInt32(80),
	})
	if _, err := client.CoreV1().Services(namespace).Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("failed to update service: %v", err)
	}
	waitForIngress(ctx, t, client, namespace, svcName)
	t.Log("service updated with second port")

	deleteServiceAndWait(ctx, t, client, namespace, svcName)
	t.Log("service and load balancer deleted")
}

// TestLoadBalancerSessionAffinity: ClientIP affinity LB provisions fine.
func TestLoadBalancerSessionAffinity(t *testing.T) {
	ctx := context.Background()
	client := newKubeClient(t)
	namespace := setupNamespace(ctx, t, client)
	deployNginx(ctx, t, client, namespace)

	timeout := int32(600)
	_, err := client.CoreV1().Services(namespace).Create(ctx, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "e2e-lb-affinity",
			Annotations: serviceAnnotations(),
		},
		Spec: v1.ServiceSpec{
			Type:            v1.ServiceTypeLoadBalancer,
			Selector:        map[string]string{"app": appLabel},
			SessionAffinity: v1.ServiceAffinityClientIP,
			SessionAffinityConfig: &v1.SessionAffinityConfig{
				ClientIP: &v1.ClientIPConfig{TimeoutSeconds: &timeout},
			},
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   v1.ProtocolTCP,
				Port:       80,
				TargetPort: intstr.FromInt32(80),
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	waitForIngress(ctx, t, client, namespace, "e2e-lb-affinity")
	deleteServiceAndWait(ctx, t, client, namespace, "e2e-lb-affinity")
}
