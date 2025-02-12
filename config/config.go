package config

type Config struct {
	// API 服务配置
	APIPort    int    `env:"API_PORT" envDefault:"8080"`
	APIKey     string `env:"API_KEY" envDefault:""`
	KubeConfig string `env:"KUBECONFIG" envDefault:""`
}

var GlobalConfig = &Config{
	APIPort: 8080,
} 