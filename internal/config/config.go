package config

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env                 string            `yaml:"env" env-default:"local"`
	UpdateInterval      time.Duration     `yaml:"update_interval" env-default:"1s"`
	Mqtt                MQTT              `yaml:"mqtt"`
	Zabbix              Zabbix            `yaml:"zabbix"`
	VirtualDevice       VirtualDevice     `yaml:"mqtt_virtual_device"`
	Severity            map[string]string `yaml:"severity,omitempty"`
	DescriptionSeverity map[string]Lang   `yaml:"description_severity,omitempty"`
	Hosts               []string          `yaml:"hosts,omitempty"`
}

type Zabbix struct {
	Address string `yaml:"address" env-default:"http://localhost:8080/api_jsonrpc.php"`
	//Login    string `yaml:"login" env-default:"Admin"`
	//Password string `yaml:"password" env-default:"zabbix"`
	Token string `yaml:"token" env-default:""`
	Group string `yaml:"group" env-default:""`
}

type MQTT struct {
	Address  string `yaml:"address" env-default:"tcp://localhost:1883"`
	ClientID string `yaml:"client_id" env-default:"smith-zabbix-mqtt"`
	Login    string `yaml:"login" env-default:""`
	Password string `yaml:"password" env-default:""`
}

type VirtualDevice struct {
	Name          string `yaml:"name" env-default:"statusServers"`
	TotalTriggers bool   `yaml:"total_triggers"`
	Uptime        bool   `yaml:"uptime"`
}

type Lang struct {
	Rus string `yaml:"ru" json:"ru"`
	Eng string `yaml:"en" json:"en"`
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

	cfg.toDefine()

	// Читаем конфигурацию
	err = cleanenv.ReadConfig(path, &cfg)
	if err != nil {
		log.Fatalf("error reading config file: '%s'", err)
	}

	if err = cfg.validate(); err != nil {
		log.Fatalf("error validate config: '%s'", err)
	}

	return &cfg
}

// Валидация конфигурации
func (cfg *Config) validate() error {
	const op = "Config.validate" // Имя текущей функции для логов и ошибок

	if cfg.Zabbix.Address == "" {
		return errors.New(op + ":zabbix address is required")
	}
	if cfg.Zabbix.Token == "" {
		return errors.New(op + ":zabbix token is required")
	}
	if cfg.Mqtt.Address == "" {
		return errors.New(op + ":mqtt address is required")
	}
	if cfg.UpdateInterval <= 0 {
		return errors.New(op + ":update interval must be positive")
	}
	if cfg.VirtualDevice.Name == "" {
		return errors.New(op + ":virtual device name is required")
	}
	if len(cfg.Hosts) == 0 {
		return errors.New(op + ":no hosts configured for monitoring")
	}
	return nil
}

// Функция задает значения по умолчанию. Вызывать перед ReadConfig
func (cfg *Config) toDefine() {
	// Значение по умолчанию для описания приоритетов
	cfg.DescriptionSeverity = map[string]Lang{
		"2": {Rus: "Норма", Eng: "Normal"},
		"3": {Rus: "Внимание", Eng: "Warning"},
		"4": {Rus: "Авария", Eng: "Alarm"},
	}

	//Значение по умолчанию для конвертации приоритетов
	cfg.Severity = map[string]string{
		"-1": "0", "0": "0", "1": "1", "2": "2", "3": "3", "4": "4", "5": "5",
	}
}
