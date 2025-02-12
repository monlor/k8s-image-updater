package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/monlor/k8s-image-updater/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	appsv1 "k8s.io/api/apps/v1"
)

type Client struct {
	clientset *kubernetes.Clientset
}

func GetClient() (*Client, error) {
	var k8sConfig *rest.Config
	var err error

	// 1. First try to use KubeConfig from configuration file
	if config.GlobalConfig.KubeConfig != "" {
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", config.GlobalConfig.KubeConfig)
		if err == nil {
			goto CREATE_CLIENT
		}
	}

	// 2. Try to use local ~/.kube/config
	if home := os.Getenv("HOME"); home != "" {
		kubeconfig := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(kubeconfig); err == nil {
			k8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
			if err == nil {
				goto CREATE_CLIENT
			}
		}
	}

	// 3. Finally try to use InClusterConfig
	k8sConfig, err = rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config: no valid configuration found")
	}

CREATE_CLIENT:
	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %v", err)
	}

	return &Client{clientset: clientset}, nil
}

// Get image tag from image string
func getImageTag(image string) string {
	if parts := strings.Split(image, ":"); len(parts) > 1 {
		return parts[1]
	}
	return "latest" // default tag
}

// Check if restart is needed
func shouldRestart(currentImage string, newImage string, pullPolicy corev1.PullPolicy) bool {
	// Restart is needed if image is the same and pull policy is Always
	return currentImage == newImage && pullPolicy == corev1.PullAlways
}

// Restart Deployment
func (c *Client) restartDeployment(deploy *appsv1.Deployment) error {
	// Ensure annotations exist
	if deploy.Spec.Template.Annotations == nil {
		deploy.Spec.Template.Annotations = make(map[string]string)
	}

	// Add or update restart annotation
	deploy.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err := c.clientset.AppsV1().Deployments(deploy.Namespace).Update(context.Background(), deploy, metav1.UpdateOptions{})
	return err
}

// Restart StatefulSet
func (c *Client) restartStatefulSet(sts *appsv1.StatefulSet) error {
	// Ensure annotations exist
	if sts.Spec.Template.Annotations == nil {
		sts.Spec.Template.Annotations = make(map[string]string)
	}

	// Add or update restart annotation
	sts.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err := c.clientset.AppsV1().StatefulSets(sts.Namespace).Update(context.Background(), sts, metav1.UpdateOptions{})
	return err
}

// Restart DaemonSet
func (c *Client) restartDaemonSet(ds *appsv1.DaemonSet) error {
	// Ensure annotations exist
	if ds.Spec.Template.Annotations == nil {
		ds.Spec.Template.Annotations = make(map[string]string)
	}

	// Add or update restart annotation
	ds.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err := c.clientset.AppsV1().DaemonSets(ds.Namespace).Update(context.Background(), ds, metav1.UpdateOptions{})
	return err
}

func (c *Client) UpdateDeploymentImage(namespace, service, container, image string) (string, error) {
	deploy, err := c.clientset.AppsV1().Deployments(namespace).Get(context.Background(), service, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// If container is empty, use the first container
	if container == "" && len(deploy.Spec.Template.Spec.Containers) > 0 {
		container = deploy.Spec.Template.Spec.Containers[0].Name
	}

	containerFound := false
	for i := range deploy.Spec.Template.Spec.Containers {
		if deploy.Spec.Template.Spec.Containers[i].Name != container {
			continue
		}
		containerFound = true

		// Case 1: Image is the same and pull policy is Always, need to restart
		if deploy.Spec.Template.Spec.Containers[i].Image == image && deploy.Spec.Template.Spec.Containers[i].ImagePullPolicy == corev1.PullAlways {
			if err := c.restartDeployment(deploy); err != nil {
				return "", fmt.Errorf("failed to restart deployment: %v", err)
			}
			return fmt.Sprintf("Updated deployment %s/%s (container: %s) by restarting to fetch latest image %s", namespace, service, container, image), nil
		}

		// Case 2: Image is different, need to update image
		if deploy.Spec.Template.Spec.Containers[i].Image != image {
			deploy.Spec.Template.Spec.Containers[i].Image = image
			_, err = c.clientset.AppsV1().Deployments(namespace).Update(context.Background(), deploy, metav1.UpdateOptions{})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Updated deployment %s/%s (container: %s) with image %s", namespace, service, container, image), nil
		}
	}

	if !containerFound {
		return "", fmt.Errorf("container %s not found in deployment", container)
	}

	return fmt.Sprintf("Image %s is already up to date for deployment %s/%s (container: %s)", image, namespace, service, container), nil
}

func (c *Client) UpdateStatefulSetImage(namespace, service, container, image string) (string, error) {
	sts, err := c.clientset.AppsV1().StatefulSets(namespace).Get(context.Background(), service, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// If container is empty, use the first container
	if container == "" && len(sts.Spec.Template.Spec.Containers) > 0 {
		container = sts.Spec.Template.Spec.Containers[0].Name
	}

	containerFound := false
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name != container {
			continue
		}
		containerFound = true

		// Case 1: Image is the same and pull policy is Always, need to restart
		if sts.Spec.Template.Spec.Containers[i].Image == image && sts.Spec.Template.Spec.Containers[i].ImagePullPolicy == corev1.PullAlways {
			if err := c.restartStatefulSet(sts); err != nil {
				return "", fmt.Errorf("failed to restart statefulset: %v", err)
			}
			return fmt.Sprintf("Updated statefulset %s/%s (container: %s) by restarting to fetch latest image %s", namespace, service, container, image), nil
		}

		// Case 2: Image is different, need to update image
		if sts.Spec.Template.Spec.Containers[i].Image != image {
			sts.Spec.Template.Spec.Containers[i].Image = image
			_, err = c.clientset.AppsV1().StatefulSets(namespace).Update(context.Background(), sts, metav1.UpdateOptions{})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Updated statefulset %s/%s (container: %s) with image %s", namespace, service, container, image), nil
		}
	}

	if !containerFound {
		return "", fmt.Errorf("container %s not found in statefulset", container)
	}

	return fmt.Sprintf("Image %s is already up to date for statefulset %s/%s (container: %s)", image, namespace, service, container), nil
}

func (c *Client) UpdateDaemonSetImage(namespace, service, container, image string) (string, error) {
	ds, err := c.clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), service, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// If container is empty, use the first container
	if container == "" && len(ds.Spec.Template.Spec.Containers) > 0 {
		container = ds.Spec.Template.Spec.Containers[0].Name
	}

	containerFound := false
	for i := range ds.Spec.Template.Spec.Containers {
		if ds.Spec.Template.Spec.Containers[i].Name != container {
			continue
		}
		containerFound = true

		// Case 1: Image is the same and pull policy is Always, need to restart
		if ds.Spec.Template.Spec.Containers[i].Image == image && ds.Spec.Template.Spec.Containers[i].ImagePullPolicy == corev1.PullAlways {
			if err := c.restartDaemonSet(ds); err != nil {
				return "", fmt.Errorf("failed to restart daemonset: %v", err)
			}
			return fmt.Sprintf("Updated daemonset %s/%s (container: %s) by restarting to fetch latest image %s", namespace, service, container, image), nil
		}

		// Case 2: Image is different, need to update image
		if ds.Spec.Template.Spec.Containers[i].Image != image {
			ds.Spec.Template.Spec.Containers[i].Image = image
			_, err = c.clientset.AppsV1().DaemonSets(namespace).Update(context.Background(), ds, metav1.UpdateOptions{})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Updated daemonset %s/%s (container: %s) with image %s", namespace, service, container, image), nil
		}
	}

	if !containerFound {
		return "", fmt.Errorf("container %s not found in daemonset", container)
	}

	return fmt.Sprintf("Image %s is already up to date for daemonset %s/%s (container: %s)", image, namespace, service, container), nil
}

// List all deployments in the cluster
func (c *Client) ListDeployments(ctx context.Context, opts metav1.ListOptions) ([]appsv1.Deployment, error) {
	deployments, err := c.clientset.AppsV1().Deployments("").List(ctx, opts)
	if err != nil {
		return nil, err
	}
	return deployments.Items, nil
}

// List all statefulsets in the cluster
func (c *Client) ListStatefulSets(ctx context.Context, opts metav1.ListOptions) ([]appsv1.StatefulSet, error) {
	statefulsets, err := c.clientset.AppsV1().StatefulSets("").List(ctx, opts)
	if err != nil {
		return nil, err
	}
	return statefulsets.Items, nil
}

// List all daemonsets in the cluster
func (c *Client) ListDaemonSets(ctx context.Context, opts metav1.ListOptions) ([]appsv1.DaemonSet, error) {
	daemonsets, err := c.clientset.AppsV1().DaemonSets("").List(ctx, opts)
	if err != nil {
		return nil, err
	}
	return daemonsets.Items, nil
}

// Get secret from the cluster
func (c *Client) GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	return c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{}) 
}

// Update deployment in the cluster
func (c *Client) UpdateDeployment(deploy *appsv1.Deployment) error {
	_, err := c.clientset.AppsV1().Deployments(deploy.Namespace).Update(context.Background(), deploy, metav1.UpdateOptions{})
	return err
}

// Update statefulset in the cluster
func (c *Client) UpdateStatefulSet(sts *appsv1.StatefulSet) error {
	_, err := c.clientset.AppsV1().StatefulSets(sts.Namespace).Update(context.Background(), sts, metav1.UpdateOptions{})
	return err
}

// Update daemonset in the cluster
func (c *Client) UpdateDaemonSet(ds *appsv1.DaemonSet) error {
	_, err := c.clientset.AppsV1().DaemonSets(ds.Namespace).Update(context.Background(), ds, metav1.UpdateOptions{})
	return err
}