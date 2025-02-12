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
func (c *Client) restartDeployment(namespace, name string) error {
	deploy, err := c.clientset.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Ensure annotations exist
	if deploy.Spec.Template.Annotations == nil {
		deploy.Spec.Template.Annotations = make(map[string]string)
	}

	// Add or update restart annotation
	deploy.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = c.clientset.AppsV1().Deployments(namespace).Update(context.Background(), deploy, metav1.UpdateOptions{})
	return err
}

// Restart StatefulSet
func (c *Client) restartStatefulSet(namespace, name string) error {
	sts, err := c.clientset.AppsV1().StatefulSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Ensure annotations exist
	if sts.Spec.Template.Annotations == nil {
		sts.Spec.Template.Annotations = make(map[string]string)
	}

	// Add or update restart annotation
	sts.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = c.clientset.AppsV1().StatefulSets(namespace).Update(context.Background(), sts, metav1.UpdateOptions{})
	return err
}

// Restart DaemonSet
func (c *Client) restartDaemonSet(namespace, name string) error {
	ds, err := c.clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Ensure annotations exist
	if ds.Spec.Template.Annotations == nil {
		ds.Spec.Template.Annotations = make(map[string]string)
	}

	// Add or update restart annotation
	ds.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = c.clientset.AppsV1().DaemonSets(namespace).Update(context.Background(), ds, metav1.UpdateOptions{})
	return err
}

func (c *Client) UpdateDeployment(namespace, name, image string, forceRestart bool) (string, error) {
	deploy, err := c.clientset.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	for i := range deploy.Spec.Template.Spec.Containers {
		// Case 1: Image is the same and pull policy is Always, need to restart
		if deploy.Spec.Template.Spec.Containers[i].Image == image && deploy.Spec.Template.Spec.Containers[i].ImagePullPolicy == corev1.PullAlways {
			if err := c.restartDeployment(namespace, name); err != nil {
				return "", fmt.Errorf("failed to restart deployment: %v", err)
			}
			return "Deployment restarted to fetch latest image", nil
		}

		// Case 2: Image is different, need to update image
		if deploy.Spec.Template.Spec.Containers[i].Image != image {
			deploy.Spec.Template.Spec.Containers[i].Image = image
			_, err = c.clientset.AppsV1().Deployments(namespace).Update(context.Background(), deploy, metav1.UpdateOptions{})
			if err != nil {
				return "", err
			}
			return "Image updated", nil
		}
	}

	return "Image is already up to date", nil
}

func (c *Client) UpdateStatefulSet(namespace, name, image string, forceRestart bool) (string, error) {
	sts, err := c.clientset.AppsV1().StatefulSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	for i := range sts.Spec.Template.Spec.Containers {
		// Case 1: Image is the same and pull policy is Always, need to restart
		if sts.Spec.Template.Spec.Containers[i].Image == image && sts.Spec.Template.Spec.Containers[i].ImagePullPolicy == corev1.PullAlways {
			if err := c.restartStatefulSet(namespace, name); err != nil {
				return "", fmt.Errorf("failed to restart statefulset: %v", err)
			}
			return "StatefulSet restarted to fetch latest image", nil
		}

		// Case 2: Image is different, need to update image
		if sts.Spec.Template.Spec.Containers[i].Image != image {
			sts.Spec.Template.Spec.Containers[i].Image = image
			_, err = c.clientset.AppsV1().StatefulSets(namespace).Update(context.Background(), sts, metav1.UpdateOptions{})
			if err != nil {
				return "", err
			}
			return "Image updated", nil
		}
	}

	return "Image is already up to date", nil
}

func (c *Client) UpdateDaemonSet(namespace, name, image string, forceRestart bool) (string, error) {
	ds, err := c.clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	for i := range ds.Spec.Template.Spec.Containers {
		// Case 1: Image is the same and pull policy is Always, need to restart
		if ds.Spec.Template.Spec.Containers[i].Image == image && ds.Spec.Template.Spec.Containers[i].ImagePullPolicy == corev1.PullAlways {
			if err := c.restartDaemonSet(namespace, name); err != nil {
				return "", fmt.Errorf("failed to restart daemonset: %v", err)
			}
			return "DaemonSet restarted to fetch latest image", nil
		}

		// Case 2: Image is different, need to update image
		if ds.Spec.Template.Spec.Containers[i].Image != image {
			ds.Spec.Template.Spec.Containers[i].Image = image
			_, err = c.clientset.AppsV1().DaemonSets(namespace).Update(context.Background(), ds, metav1.UpdateOptions{})
			if err != nil {
				return "", err
			}
			return "Image updated", nil
		}
	}

	return "Image is already up to date", nil
} 