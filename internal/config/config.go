package config

import (
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env            string         `yaml:"env" env-default:"local"`
	UpdateInterval time.Duration  `yaml:"update_interval" env-default:"1s"`
	Mqtt           MQTT           `yaml:"mqtt"`
	Zabbix         Zabbix         `yaml:"zabbix"`
	Topics         TopicsPublic   `yaml:"topics"`
	Severity       map[int]string `yaml:"severity,omitempty"`
}

type Zabbix struct {
	Address string `yaml:"address" env-default:"http://localhost:8080/api_jsonrpc.php"`
	//Login    string `yaml:"login" env-default:"Admin"`
	//Password string `yaml:"password" env-default:"zabbix"`
	Token string `yaml:"token" env-default:""`
	Group string `yaml:"group" env-default:""`
}

type TopicsPublic struct {
	TotalTriggers string            `yaml:"total_triggers,omitempty"`
	Uptime        string            `yaml:"uptime,omitempty"`
	Servers       map[string]string `yaml:"servers" env-default:""`
}

type MQTT struct {
	Address  string `yaml:"address" env-default:"tcp://localhost:1883"`
	ClientID string `yaml:"client_id" env-default:"smith-zabbix-mqtt"`
	Auth     bool   `yaml:"authorization" env-default:"false"`
	Login    string `yaml:"login" env-default:""`
	Password string `yaml:"password" env-default:""`
}

// Загружаем конфигурацию из файла
// В приоритете загрузка файла конфигурации, указанного в переменной окружении
func MustLoad(configPath string) *Config {
	var (
		err error
		cfg Config
	)

	path := configPath

	if configPathENV := os.Getenv("CONFIG_FILE_SZM"); configPathENV != "" {
		path = configPathENV
		log.Printf("ENV: %s", path)
	}

	// Если файла конфигурации не существует, то выходим
	if _, err = os.Stat(path); os.IsNotExist(err) {
		log.Fatalf("configuration file does not exist: '%s'", path)
	}

	// Читаем конфигурацию
	err = cleanenv.ReadConfig(path, &cfg)
	if err != nil {
		log.Fatalf("error reading config file: '%s'", err)
	}

	return &cfg
}
