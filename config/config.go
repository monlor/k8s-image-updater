package config

import (
	"time"

	"github.com/caarlos0/env/v10"
	"github.com/sirupsen/logrus"
)

type Config struct {
	// API service configuration
	APIPort     int    `env:"API_PORT" envDefault:"8080"`
	APIKey      string `env:"API_KEY" envDefault:""`
	KubeConfig  string `env:"KUBECONFIG" envDefault:""`
	LogLevel    string `env:"LOG_LEVEL" envDefault:""`
	LogTimezone string `env:"LOG_TIMEZONE" envDefault:"UTC"`

	// Image update configuration
	UpdaterEnabled      bool          `env:"UPDATER_ENABLED" envDefault:"true"`     // Enable/disable auto updater
	ImageUpdateInterval time.Duration `env:"IMAGE_UPDATE_INTERVAL" envDefault:"5m"` // Default check interval is 5 minutes

	// Allowed namespaces configuration
	AllowedNamespaces string `env:"ALLOWED_NAMESPACES" envDefault:""` // Comma-separated list of allowed namespaces
}

// Annotation keys for image update configuration
const (
	// Enable auto update for the resource
	LabelEnabled = "image-updater.k8s.io/enabled"
	// Image update mode: digest, release or latest
	AnnotationMode = "image-updater.k8s.io/mode"
	// Container name to update, if not set, update all containers
	AnnotationContainer = "image-updater.k8s.io/container"
	// Restart annotation for latest mode
	AnnotationRestart = "kubectl.kubernetes.io/restartedAt"
	// Last known digest for latest mode
	AnnotationLastDigest = "image-updater.k8s.io/last-digest"
	// Allow tags regex
	AnnotationAllowTags = "image-updater.k8s.io/allow-tags"
)

var GlobalConfig = &Config{}

func init() {
	if err := env.Parse(GlobalConfig); err != nil {
		logrus.Fatalf("Failed to parse environment variables: %v", err)
	}
}
