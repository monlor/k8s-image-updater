package updater

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
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

// getRegistryClientForImage finds the right registry client (with auth) for a given image.
// It iterates through a list of image pull secrets to find credentials.
func (u *Updater) getRegistryClientForImage(ctx context.Context, image, namespace string, secretNames []string) (*registry.RegistryClient, error) {
	imageInfo, err := registry.ParseImage(image)
	if err != nil {
		// Fallback to anonymous client if parsing fails, as it might be a local image
		logrus.Warnf("Could not parse image name %s, using anonymous registry client: %v", image, err)
		return registry.NewRegistryClient("", ""), nil
	}
	imageRegistry := imageInfo.Registry

	// Define struct for docker config
	type DockerConfigEntry struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Auth     string `json:"auth"`
	}
	type DockerConfigJSON struct {
		Auths map[string]DockerConfigEntry `json:"auths"`
	}

	for _, secretName := range secretNames {
		secret, err := u.k8sClient.GetSecret(ctx, namespace, secretName)
		if err != nil {
			logrus.Warnf("Failed to get secret %s in namespace %s, skipping: %v", secretName, namespace, err)
			continue
		}

		if secret.Type != corev1.SecretTypeDockerConfigJson {
			logrus.Debugf("Secret %s is not of type %s, skipping", secretName, corev1.SecretTypeDockerConfigJson)
			continue
		}

		configData, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			logrus.Warnf("Secret %s of type %s does not contain %s key, skipping", secretName, corev1.SecretTypeDockerConfigJson, corev1.DockerConfigJsonKey)
			continue
		}

		var dockerConfig DockerConfigJSON
		if err := json.Unmarshal(configData, &dockerConfig); err != nil {
			logrus.Warnf("Failed to unmarshal docker config from secret %s, skipping: %v", secretName, err)
			continue
		}

		if authEntry, found := dockerConfig.Auths[imageRegistry]; found {
			username, password := authEntry.Username, authEntry.Password
			if authEntry.Auth != "" {
				decoded, err := base64.StdEncoding.DecodeString(authEntry.Auth)
				if err != nil {
					logrus.Warnf("Failed to decode auth from secret %s for registry %s, skipping: %v", secretName, imageRegistry, err)
					continue
				}
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					username = parts[0]
					password = parts[1]
				}
			}
			logrus.Debugf("Found credentials for registry %s in secret %s", imageRegistry, secretName)
			return registry.NewRegistryClient(username, password), nil
		}
	}

	logrus.Debugf("No credentials found for registry %s in provided secrets, using anonymous access.", imageRegistry)
	return registry.NewRegistryClient("", ""), nil
}

// filterTagsByRegex filters a list of tags based on a regex pattern.
func filterTagsByRegex(tags []string, regexStr string) ([]string, error) {
	if regexStr == "" {
		return tags, nil
	}
	re, err := regexp.Compile(regexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid regex for allow-tags: %v", err)
	}
	filteredTags := []string{}
	for _, tag := range tags {
		if re.MatchString(tag) {
			filteredTags = append(filteredTags, tag)
		}
	}
	logrus.Debugf("Filtered %d tags to %d with regex: %s", len(tags), len(filteredTags), regexStr)
	return filteredTags, nil
}

// Check if an image needs to be updated based on mode
func (u *Updater) checkReleaseMode(ctx context.Context, currentImage string, registryClient *registry.RegistryClient, allowTagsRegex string) (string, error) {
	imageInfo, err := registry.ParseImage(currentImage)
	if err != nil {
		return "", fmt.Errorf("failed to parse image %s: %v", currentImage, err)
	}

	tags, err := registryClient.ListTags(ctx, currentImage)
	if err != nil {
		return "", fmt.Errorf("failed to list tags for %s: %v", currentImage, err)
	}
	logrus.Debugf("Found %d tags for image %s", len(tags), currentImage)

	tags, err = filterTagsByRegex(tags, allowTagsRegex)
	if err != nil {
		return "", err
	}

	sortedTags := registry.SortVersionTags(tags)
	if len(sortedTags) > 0 && sortedTags[0] != imageInfo.Tag {
		logrus.Debugf("Current tag: %s, Latest tag: %s", imageInfo.Tag, sortedTags[0])
		return fmt.Sprintf("%s/%s:%s", imageInfo.Registry, imageInfo.Repository, sortedTags[0]), nil
	}
	return "", nil
}

func (u *Updater) checkAlphabeticalMode(ctx context.Context, currentImage string, registryClient *registry.RegistryClient, allowTagsRegex string) (string, error) {
	imageInfo, err := registry.ParseImage(currentImage)
	if err != nil {
		return "", fmt.Errorf("failed to parse image %s: %v", currentImage, err)
	}

	tags, err := registryClient.ListTags(ctx, currentImage)
	if err != nil {
		return "", fmt.Errorf("failed to list tags for %s: %v", currentImage, err)
	}
	logrus.Debugf("Found %d tags for image %s", len(tags), currentImage)

	tags, err = filterTagsByRegex(tags, allowTagsRegex)
	if err != nil {
		return "", err
	}

	sortedTags := registry.SortAlphabeticalTags(tags)
	if len(sortedTags) > 0 && sortedTags[0] != imageInfo.Tag {
		logrus.Debugf("Current tag: %s, Latest tag: %s", imageInfo.Tag, sortedTags[0])
		return fmt.Sprintf("%s/%s:%s", imageInfo.Registry, imageInfo.Repository, sortedTags[0]), nil
	}
	return "", nil
}

func (u *Updater) checkDigestMode(ctx context.Context, currentImage string, registryClient *registry.RegistryClient, tagToCheck string) (string, error) {
	imageInfo, err := registry.ParseImage(currentImage)
	if err != nil {
		return "", fmt.Errorf("failed to parse image %s: %v", currentImage, err)
	}

	imageToCheck := fmt.Sprintf("%s/%s:%s", imageInfo.Registry, imageInfo.Repository, tagToCheck)

	newDigest, err := registryClient.GetDigest(ctx, imageToCheck)
	if err != nil {
		return "", fmt.Errorf("failed to get digest for %s: %v", imageToCheck, err)
	}
	logrus.Debugf("Checking digest for %s. Current digest: %s, New digest from registry: %s", imageToCheck, imageInfo.Digest, newDigest)
	if newDigest != imageInfo.Digest {
		// We use the image base from the original image, and the new digest. The tag is not preserved.
		return fmt.Sprintf("%s/%s@%s", imageInfo.Registry, imageInfo.Repository, newDigest), nil
	}
	return "", nil
}

func (u *Updater) checkLatestMode(ctx context.Context, currentImage string, registryClient *registry.RegistryClient, annotations *map[string]string, podTemplate *corev1.PodTemplateSpec) (bool, error) {
	newDigest, err := registryClient.GetDigest(ctx, currentImage)
	if err != nil {
		return false, fmt.Errorf("failed to get digest for %s: %v", currentImage, err)
	}

	lastDigest := (*annotations)[config.AnnotationLastDigest]
	if lastDigest == "" {
		(*annotations)[config.AnnotationLastDigest] = newDigest
		// First time seeing this image, store the digest
		logrus.Debugf("First time seeing image %s, storing digest %s", currentImage, newDigest)
		return true, nil
	}

	// Compare digests
	if newDigest != lastDigest {
		(*annotations)[config.AnnotationLastDigest] = newDigest
		(*podTemplate).Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
		logrus.Infof("New digest detected for %s: %s -> %s", currentImage, lastDigest, newDigest)
		return true, nil
	}
	return false, nil
}

// Update container if needed
func (u *Updater) updateContainerIfNeeded(ctx context.Context, container *corev1.Container, annotations *map[string]string, namespace string, podTemplate *corev1.PodTemplateSpec) (bool, error) {
	// Ensure annotations map exists
	if *annotations == nil {
		*annotations = make(map[string]string)
	}

	if (*annotations)[config.AnnotationEnabled] != "true" {
		logrus.Debugf("Auto-update not enabled for container %s", container.Name)
		return false, nil
	}

	containerName := (*annotations)[config.AnnotationContainer]
	if containerName != "" && containerName != container.Name {
		logrus.Debugf("Container %s does not match target container %s", container.Name, containerName)
		return false, nil
	}

	mode := (*annotations)[config.AnnotationMode]
	if mode == "" {
		mode = "release" // Default to release mode
	}

	allowTagsAnnotation := (*annotations)[config.AnnotationAllowTags]
	var allowTagsRegex string
	if strings.HasPrefix(allowTagsAnnotation, "regexp:") {
		allowTagsRegex = strings.TrimPrefix(allowTagsAnnotation, "regexp:")
	}

	// Get all imagePullSecrets
	var secretNames []string
	for _, secret := range podTemplate.Spec.ImagePullSecrets {
		secretNames = append(secretNames, secret.Name)
	}

	registryClient, err := u.getRegistryClientForImage(ctx, container.Image, namespace, secretNames)
	if err != nil {
		return false, fmt.Errorf("failed to get registry client: %v", err)
	}

	logrus.Debugf("Using update mode %s for container %s", mode, container.Name)

	switch mode {
	case "latest":
		if container.ImagePullPolicy != corev1.PullAlways {
			logrus.Warnf("Container %s is in latest mode but imagePullPolicy is not Always, skipping update", container.Name)
			return false, nil
		}
		return u.checkLatestMode(ctx, container.Image, registryClient, annotations, podTemplate)

	case "digest":
		tagToCheck := "latest" // default
		if allowTagsAnnotation != "" && !strings.HasPrefix(allowTagsAnnotation, "regexp:") {
			tagToCheck = allowTagsAnnotation
		}
		newImage, err := u.checkDigestMode(ctx, container.Image, registryClient, tagToCheck)
		if err != nil {
			return false, err
		}
		if newImage != "" {
			logrus.Infof("Updating image for container %s from %s to %s", container.Name, container.Image, newImage)
			container.Image = newImage
			return true, nil
		}

	case "alphabetical", "name":
		newImage, err := u.checkAlphabeticalMode(ctx, container.Image, registryClient, allowTagsRegex)
		if err != nil {
			return false, err
		}
		if newImage != "" {
			logrus.Infof("Updating image for container %s from %s to %s", container.Name, container.Image, newImage)
			container.Image = newImage
			return true, nil
		}

	case "release":
		newImage, err := u.checkReleaseMode(ctx, container.Image, registryClient, allowTagsRegex)
		if err != nil {
			return false, err
		}
		if newImage != "" {
			logrus.Infof("Updating image for container %s from %s to %s", container.Name, container.Image, newImage)
			container.Image = newImage
			return true, nil
		}

	default:
		logrus.Warnf("Unknown update mode: %s", mode)
	}

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
		if deploy.Annotations[config.AnnotationEnabled] != "true" {
			logrus.Debugf("Deployment %s/%s is not enabled for auto-update", deploy.Namespace, deploy.Name)
			continue
		}

		updated := false
		for i := range deploy.Spec.Template.Spec.Containers {
			container := &deploy.Spec.Template.Spec.Containers[i]
			logrus.Debugf("Checking container %s in deployment %s/%s", container.Name, deploy.Namespace, deploy.Name)

			containerUpdated, err := u.updateContainerIfNeeded(ctx, container, &deploy.Annotations, deploy.Namespace, &deploy.Spec.Template)
			if err != nil {
				logrus.Errorf("Failed to update container %s in deployment %s/%s: %v", container.Name, deploy.Namespace, deploy.Name, err)
				continue
			}
			if containerUpdated {
				logrus.Infof("Container %s in deployment %s/%s needs update", container.Name, deploy.Namespace, deploy.Name)
				updated = true
			}
		}

		if updated {
			logrus.Infof("Updating deployment %s/%s", deploy.Namespace, deploy.Name)
			if err := u.k8sClient.UpdateDeployment(&deploy); err != nil {
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
		if sts.Annotations[config.AnnotationEnabled] != "true" {
			logrus.Debugf("StatefulSet %s/%s is not enabled for auto-update", sts.Namespace, sts.Name)
			continue
		}

		updated := false
		for i := range sts.Spec.Template.Spec.Containers {
			container := &sts.Spec.Template.Spec.Containers[i]
			logrus.Debugf("Checking container %s in statefulset %s/%s", container.Name, sts.Namespace, sts.Name)

			containerUpdated, err := u.updateContainerIfNeeded(ctx, container, &sts.Annotations, sts.Namespace, &sts.Spec.Template)
			if err != nil {
				logrus.Errorf("Failed to update container %s in statefulset %s/%s: %v", container.Name, sts.Namespace, sts.Name, err)
				continue
			}
			if containerUpdated {
				logrus.Infof("Container %s in statefulset %s/%s needs update", container.Name, sts.Namespace, sts.Name)
				updated = true
			}
		}

		if updated {
			logrus.Infof("Updating statefulset %s/%s", sts.Namespace, sts.Name)
			if err := u.k8sClient.UpdateStatefulSet(&sts); err != nil {
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
		if ds.Annotations[config.AnnotationEnabled] != "true" {
			logrus.Debugf("DaemonSet %s/%s is not enabled for auto-update", ds.Namespace, ds.Name)
			continue
		}

		updated := false
		for i := range ds.Spec.Template.Spec.Containers {
			container := &ds.Spec.Template.Spec.Containers[i]
			logrus.Debugf("Checking container %s in daemonset %s/%s", container.Name, ds.Namespace, ds.Name)

			containerUpdated, err := u.updateContainerIfNeeded(ctx, container, &ds.Annotations, ds.Namespace, &ds.Spec.Template)
			if err != nil {
				logrus.Errorf("Failed to update container %s in daemonset %s/%s: %v", container.Name, ds.Namespace, ds.Name, err)
				continue
			}
			if containerUpdated {
				logrus.Infof("Container %s in daemonset %s/%s needs update", container.Name, ds.Namespace, ds.Name)
				updated = true
			}
		}

		if updated {
			logrus.Infof("Updating daemonset %s/%s", ds.Namespace, ds.Name)
			if err := u.k8sClient.UpdateDaemonSet(&ds); err != nil {
				logrus.Errorf("Failed to update daemonset %s/%s: %v", ds.Namespace, ds.Name, err)
			}
		} else {
			logrus.Debugf("No updates needed for daemonset %s/%s", ds.Namespace, ds.Name)
		}
	}

	return nil
}
