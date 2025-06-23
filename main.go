package main

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/monlor/k8s-image-updater/config"
	"github.com/monlor/k8s-image-updater/pkg/api"
	"github.com/monlor/k8s-image-updater/pkg/updater"
	"github.com/sirupsen/logrus"
)

// CustomTextFormatter is a custom logrus formatter that includes a specific timezone.
type CustomTextFormatter struct {
	logrus.TextFormatter
	Location *time.Location
}

// Format formats the log entry.
func (f *CustomTextFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	entry.Time = entry.Time.In(f.Location)
	return f.TextFormatter.Format(entry)
}

// CustomJSONFormatter is a custom logrus formatter that includes a specific timezone.
type CustomJSONFormatter struct {
	logrus.JSONFormatter
	Location *time.Location
}

// Format formats the log entry.
func (f *CustomJSONFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	entry.Time = entry.Time.In(f.Location)
	return f.JSONFormatter.Format(entry)
}

func main() {
	// Load timezone
	loc, err := time.LoadLocation(config.GlobalConfig.LogTimezone)
	if err != nil {
		logrus.Warnf("Invalid timezone %q, using UTC: %v", config.GlobalConfig.LogTimezone, err)
		loc, _ = time.LoadLocation("UTC")
	}

	// Set log format
	if gin.Mode() == gin.ReleaseMode {
		logrus.SetFormatter(&CustomJSONFormatter{
			JSONFormatter: logrus.JSONFormatter{},
			Location:      loc,
		})
	} else {
		logrus.SetFormatter(&CustomTextFormatter{
			TextFormatter: logrus.TextFormatter{
				FullTimestamp: true,
			},
			Location: loc,
		})
	}

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
