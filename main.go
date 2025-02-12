package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/monlor/k8s-image-updater/config"
	"github.com/monlor/k8s-image-updater/pkg/api"
	"github.com/sirupsen/logrus"
)

func main() {
	// Set log format
	logrus.SetFormatter(&logrus.JSONFormatter{})

	// Create Gin router
	r := gin.Default()

	// Add authentication middleware
	r.Use(api.AuthMiddleware())

	// Register routes
	r.GET("/api/v1/update", api.UpdateImage)

	// Start server
	addr := fmt.Sprintf(":%d", config.GlobalConfig.APIPort)
	logrus.Infof("Starting server on %s", addr)
	if err := r.Run(addr); err != nil {
		logrus.Fatalf("Failed to start server: %v", err)
	}
} 