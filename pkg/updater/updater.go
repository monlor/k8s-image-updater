package updater

import (
	"context"
	"fmt"
	"time"

	"github.com/monlor/k8s-image-updater/config"
	"github.com/monlor/k8s-image-updater/pkg/k8s"
	"github.com/monlor/k8s-image-updater/pkg/registry"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Updater struct {
	k8sClient *k8s.Client
	registry  *registry.RegistryClient
}

func NewUpdater() (*Updater, error) {
	// Create Kubernetes client
	k8sClient, err := k8s.GetClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %v", err)
	}

	return &Updater{
		k8sClient: k8sClient,
		registry:  registry.NewRegistryClient("", ""), // Default to anonymous access
	}, nil
}

// Start the auto-update process
func (u *Updater) Start(ctx context.Context) {
	ticker := time.NewTicker(config.GlobalConfig.ImageUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := u.CheckAndUpdate(ctx); err != nil {
				logrus.Errorf("Failed to check and update images: %v", err)
			}
		}
	}
}

// Check and update all resources with auto-update annotations
func (u *Updater) CheckAndUpdate(ctx context.Context) error {
	logrus.Debug("Starting periodic check for image updates")

	// Check deployments
	if err := u.updateDeployments(ctx); err != nil {
		logrus.Errorf("Failed to update deployments: %v", err)
	}

	// Check statefulsets
	if err := u.updateStatefulSets(ctx); err != nil {
		logrus.Errorf("Failed to update statefulsets: %v", err)
	}

	// Check daemonsets
	if err := u.updateDaemonSets(ctx); err != nil {
		logrus.Errorf("Failed to update daemonsets: %v", err)
	}

	logrus.Debug("Completed periodic check for image updates")
	return nil
}

// Get registry client with authentication if needed
func (u *Updater) getRegistryClientForSecret(ctx context.Context, namespace, secretName string) (*registry.RegistryClient, error) {
	if secretName == "" {
		return registry.NewRegistryClient("", ""), nil
	}

	secret, err := u.k8sClient.GetSecret(ctx, namespace, secretName)
	if err != nil {
		return nil, fmt.Errorf("failed to get registry secret: %v", err)
	}

	username := string(secret.Data["username"])
	password := string(secret.Data["password"])
	return registry.NewRegistryClient(username, password), nil
}

// Check if an image needs to be updated based on mode
func (u *Updater) checkImageUpdate(ctx context.Context, currentImage, mode string, registryClient *registry.RegistryClient) (string, error) {
	imageInfo, err := registry.ParseImage(currentImage)
	if err != nil {
		return "", fmt.Errorf("failed to parse image %s: %v", currentImage, err)
	}

	logrus.Debugf("Checking image %s in %s mode", currentImage, mode)
	switch mode {
	case "digest":
		newDigest, err := registryClient.GetDigest(ctx, currentImage)
		if err != nil {
			return "", fmt.Errorf("failed to get digest for %s: %v", currentImage, err)
		}
		logrus.Debugf("Current digest: %s, New digest: %s", imageInfo.Digest, newDigest)
		if newDigest != imageInfo.Digest {
			return fmt.Sprintf("%s/%s@%s", imageInfo.Registry, imageInfo.Repository, newDigest), nil
		}
	case "release":
		tags, err := registryClient.ListTags(ctx, currentImage)
		if err != nil {
			return "", fmt.Errorf("failed to list tags for %s: %v", currentImage, err)
		}
		logrus.Debugf("Found %d tags for image %s", len(tags), currentImage)
		sortedTags := registry.SortVersionTags(tags)
		logrus.Debugf("Sorted tags: %v", sortedTags)
		if len(sortedTags) > 0 && sortedTags[0] != imageInfo.Tag {
			logrus.Debugf("Current tag: %s, Latest tag: %s", imageInfo.Tag, sortedTags[0])
			return fmt.Sprintf("%s/%s:%s", imageInfo.Registry, imageInfo.Repository, sortedTags[0]), nil
		}
	default:
		logrus.Warnf("Unknown update mode: %s", mode)
	}

	return "", nil
}

// Update container if needed
func (u *Updater) updateContainerIfNeeded(ctx context.Context, container *corev1.Container, annotations map[string]string, namespace string) (bool, error) {
	if annotations[config.AnnotationEnable] != "true" {
		logrus.Debugf("Auto-update not enabled for container %s", container.Name)
		return false, nil
	}

	containerName := annotations[config.AnnotationContainer]
	if containerName != "" && containerName != container.Name {
		logrus.Debugf("Container %s does not match target container %s", container.Name, containerName)
		return false, nil
	}

	mode := annotations[config.AnnotationMode]
	if mode == "" {
		mode = "release" // Default to release mode
	}
	logrus.Debugf("Using update mode %s for container %s", mode, container.Name)

	registryClient, err := u.getRegistryClientForSecret(ctx, namespace, annotations[config.AnnotationSecret])
	if err != nil {
		return false, fmt.Errorf("failed to get registry client: %v", err)
	}

	logrus.Debugf("Checking for updates of image %s", container.Image)
	newImage, err := u.checkImageUpdate(ctx, container.Image, mode, registryClient)
	if err != nil {
		return false, fmt.Errorf("failed to check image update: %v", err)
	}

	if newImage != "" {
		logrus.Infof("New image found for %s: %s -> %s", container.Name, container.Image, newImage)
		container.Image = newImage
		return true, nil
	}

	logrus.Debugf("No new image found for container %s", container.Name)
	return false, nil
}

// Update deployments with auto-update annotations
func (u *Updater) updateDeployments(ctx context.Context) error {
	logrus.Debug("Checking deployments for updates")
	deployments, err := u.k8sClient.ListDeployments(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	logrus.Debugf("Found %d deployments in total", len(deployments))

	for _, deploy := range deployments {
		logrus.Debugf("Checking deployment %s/%s", deploy.Namespace, deploy.Name)
		if deploy.Annotations[config.AnnotationEnable] != "true" {
			logrus.Debugf("Deployment %s/%s is not enabled for auto-update", deploy.Namespace, deploy.Name)
			continue
		}

		updated := false
		for i := range deploy.Spec.Template.Spec.Containers {
			container := &deploy.Spec.Template.Spec.Containers[i]
			logrus.Debugf("Checking container %s in deployment %s/%s", container.Name, deploy.Namespace, deploy.Name)
			
			containerUpdated, err := u.updateContainerIfNeeded(ctx, container, deploy.Annotations, deploy.Namespace)
			if err != nil {
				logrus.Errorf("Failed to update container %s in deployment %s/%s: %v", container.Name, deploy.Namespace, deploy.Name, err)
				continue
			}
			if containerUpdated {
				logrus.Infof("Container %s in deployment %s/%s needs update", container.Name, deploy.Namespace, deploy.Name)
			}
			updated = updated || containerUpdated
		}

		if updated {
			logrus.Infof("Updating deployment %s/%s", deploy.Namespace, deploy.Name)
			if _, err := u.k8sClient.UpdateDeployment(deploy.Namespace, deploy.Name, deploy.Spec.Template.Spec.Containers[0].Image, false); err != nil {
				logrus.Errorf("Failed to update deployment %s/%s: %v", deploy.Namespace, deploy.Name, err)
			}
		} else {
			logrus.Debugf("No updates needed for deployment %s/%s", deploy.Namespace, deploy.Name)
		}
	}

	return nil
}

// Update StatefulSets with auto-update annotations
func (u *Updater) updateStatefulSets(ctx context.Context) error {
	logrus.Debug("Checking statefulsets for updates")
	statefulsets, err := u.k8sClient.ListStatefulSets(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	logrus.Debugf("Found %d statefulsets in total", len(statefulsets))

	for _, sts := range statefulsets {
		logrus.Debugf("Checking statefulset %s/%s", sts.Namespace, sts.Name)
		if sts.Annotations[config.AnnotationEnable] != "true" {
			logrus.Debugf("StatefulSet %s/%s is not enabled for auto-update", sts.Namespace, sts.Name)
			continue
		}

		updated := false
		for i := range sts.Spec.Template.Spec.Containers {
			container := &sts.Spec.Template.Spec.Containers[i]
			logrus.Debugf("Checking container %s in statefulset %s/%s", container.Name, sts.Namespace, sts.Name)
			
			containerUpdated, err := u.updateContainerIfNeeded(ctx, container, sts.Annotations, sts.Namespace)
			if err != nil {
				logrus.Errorf("Failed to update container %s in statefulset %s/%s: %v", container.Name, sts.Namespace, sts.Name, err)
				continue
			}
			if containerUpdated {
				logrus.Infof("Container %s in statefulset %s/%s needs update", container.Name, sts.Namespace, sts.Name)
			}
			updated = updated || containerUpdated
		}

		if updated {
			logrus.Infof("Updating statefulset %s/%s", sts.Namespace, sts.Name)
			if _, err := u.k8sClient.UpdateStatefulSet(sts.Namespace, sts.Name, sts.Spec.Template.Spec.Containers[0].Image, false); err != nil {
				logrus.Errorf("Failed to update statefulset %s/%s: %v", sts.Namespace, sts.Name, err)
			}
		} else {
			logrus.Debugf("No updates needed for statefulset %s/%s", sts.Namespace, sts.Name)
		}
	}

	return nil
}

// Update DaemonSets with auto-update annotations
func (u *Updater) updateDaemonSets(ctx context.Context) error {
	logrus.Debug("Checking daemonsets for updates")
	daemonsets, err := u.k8sClient.ListDaemonSets(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	logrus.Debugf("Found %d daemonsets in total", len(daemonsets))

	for _, ds := range daemonsets {
		logrus.Debugf("Checking daemonset %s/%s", ds.Namespace, ds.Name)
		if ds.Annotations[config.AnnotationEnable] != "true" {
			logrus.Debugf("DaemonSet %s/%s is not enabled for auto-update", ds.Namespace, ds.Name)
			continue
		}

		updated := false
		for i := range ds.Spec.Template.Spec.Containers {
			container := &ds.Spec.Template.Spec.Containers[i]
			logrus.Debugf("Checking container %s in daemonset %s/%s", container.Name, ds.Namespace, ds.Name)
			
			containerUpdated, err := u.updateContainerIfNeeded(ctx, container, ds.Annotations, ds.Namespace)
			if err != nil {
				logrus.Errorf("Failed to update container %s in daemonset %s/%s: %v", container.Name, ds.Namespace, ds.Name, err)
				continue
			}
			if containerUpdated {
				logrus.Infof("Container %s in daemonset %s/%s needs update", container.Name, ds.Namespace, ds.Name)
			}
			updated = updated || containerUpdated
		}

		if updated {
			logrus.Infof("Updating daemonset %s/%s", ds.Namespace, ds.Name)
			if _, err := u.k8sClient.UpdateDaemonSet(ds.Namespace, ds.Name, ds.Spec.Template.Spec.Containers[0].Image, false); err != nil {
				logrus.Errorf("Failed to update daemonset %s/%s: %v", ds.Namespace, ds.Name, err)
			}
		} else {
			logrus.Debugf("No updates needed for daemonset %s/%s", ds.Namespace, ds.Name)
		}
	}

	return nil
} 