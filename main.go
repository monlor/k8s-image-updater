package main

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/monlor/k8s-image-updater/config"
	"github.com/monlor/k8s-image-updater/pkg/api"
	"github.com/monlor/k8s-image-updater/pkg/updater"
	"github.com/sirupsen/logrus"
)

func main() {
	// Set log format
	logrus.SetFormatter(&logrus.JSONFormatter{})
	// Set log level based on GIN_MODE
	if config.GlobalConfig.LogLevel != "" {
		level, err := logrus.ParseLevel(config.GlobalConfig.LogLevel)
		if err != nil {
			logrus.Warnf("Invalid log level %s, using default level", config.GlobalConfig.LogLevel)
		} else {
			logrus.SetLevel(level)
		}
	} else {
		if gin.Mode() == gin.ReleaseMode {
			logrus.SetLevel(logrus.InfoLevel)
		} else {
			logrus.SetLevel(logrus.DebugLevel)
		}
	}

	// Create and start the auto-updater if enabled
	ctx := context.Background()
	if config.GlobalConfig.UpdaterEnabled {
		logrus.Info("Auto-updater is enabled")
		imageUpdater, err := updater.NewUpdater()
		if err != nil {
			logrus.Fatalf("Failed to create image updater: %v", err)
		}
		go imageUpdater.Start(ctx)
	} else {
		logrus.Info("Auto-updater is disabled, only API service will be available")
	}

	// Create Gin router
	r := gin.Default()

	// Create API route group with authentication
	apiV1 := r.Group("/api/v1")
	apiV1.Use(api.AuthMiddleware()) 
	{
		// Register routes under the authenticated group
		apiV1.GET("/update", api.UpdateImage)
	}

	// Start server
	addr := fmt.Sprintf(":%d", config.GlobalConfig.APIPort)
	logrus.Infof("Starting server on %s", addr)
	if err := r.Run(addr); err != nil {
		logrus.Fatalf("Failed to start server: %v", err)
	}
} 