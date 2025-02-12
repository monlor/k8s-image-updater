package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/monlor/k8s-image-updater/config"
	"github.com/monlor/k8s-image-updater/pkg/k8s"
	"github.com/sirupsen/logrus"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey != config.GlobalConfig.APIKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func UpdateImage(c *gin.Context) {
	// Get values from query parameters
	namespace := c.Query("namespace")
	name := c.Query("name")
	kind := strings.ToLower(c.DefaultQuery("kind", "deployment")) // default value is deployment
	image := c.Query("image")

	// Validate required parameters
	if namespace == "" || name == "" || image == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "namespace, name, and image are required"})
		return
	}

	// Validate resource type
	if kind != "deployment" && kind != "statefulset" && kind != "daemonset" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kind must be one of: deployment, statefulset, daemonset"})
		return
	}

	client, err := k8s.GetClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var result string
	var updateErr error

	switch kind {
	case "deployment":
		result, updateErr = client.UpdateDeployment(namespace, name, image, false)
	case "statefulset":
		result, updateErr = client.UpdateStatefulSet(namespace, name, image, false)
	case "daemonset":
		result, updateErr = client.UpdateDaemonSet(namespace, name, image, false)
	}

	if updateErr != nil {
		logrus.Errorf("Failed to update %s %s/%s: %v", kind, namespace, name, updateErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": updateErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": result,
		"details": fmt.Sprintf("Updated %s %s/%s with image %s", kind, namespace, name, image),
	})
} 