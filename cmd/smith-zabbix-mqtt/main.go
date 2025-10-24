package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/config"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/fabiang/go-zabbix"
)

const (
	envLocal = "local"
	envDev   = "dev"
	version  = "0.0.5"
	driver   = "smith-zabbix-mqtt"
	rus      = "Zabbix2MQTT"
	eng      = "Zabbix2MQTT"

	DEFAULT_CONFIG_PATH = "/etc/smith-zabbix-mqtt/config.yaml"
	MOSQUITTO_SOCK_FILE = "/var/run/mosquitto/mosquitto.sock"
	DEFAULT_BROKER_URL  = "tcp://localhost:1883"
)

var log *slog.Logger

func init() {
	log = setupLogger(envLocal)
}

type ConnectionManager struct {
	tm *TriggerManager
}

func (cm *ConnectionManager) GetOnConnectHandler() mqtt.OnConnectHandler {
	return func(client mqtt.Client) {
		log.Info("MQTT connection established")

		// Вызываем все нужные обработчики
		cm.tm.OnConnect(client)

	}
}

// Добавляем метод валидации конфигурации
func validateConfig(cfg *config.Config) error {
	if cfg.Zabbix.Address == "" {
		return errors.New("адрес Zabbix обязателен")
	}
	if cfg.Zabbix.Token == "" {
		return errors.New("токен Zabbix обязателен")
	}
	if cfg.Mqtt.Address == "" {
		return errors.New("адрес MQTT обязателен")
	}
	if cfg.UpdateInterval <= 0 {
		return errors.New("интервал обновления должен быть положительным")
	}
	if cfg.VirtualDevice.Name == "" {
		return errors.New("имя виртуального устройства обязательно")
	}
	if len(cfg.Hosts) == 0 {
		return errors.New("не найдено ни одного хоста для слежения")
	}
	return nil
}

func main() {

	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		os.Exit(0)
	}

	debug := flag.Bool("debug", false, "Enable debugging")
	configPath := flag.String("configFile", DEFAULT_CONFIG_PATH, "Config path")

	flag.Parse()

	// Читаем конфигурацию
	cfg := config.MustLoad(*configPath)

	//Проверка на валидность
	if err := validateConfig(cfg); err != nil {
		log.Error(
			"Ошибка конфигурации",
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}

	// Создаем и заполняем переменную для работы с триггерами
	triggerManager := NewTriggerManager(cfg)

	connectionManager := &ConnectionManager{}
	connectionManager.tm = triggerManager

	log.Info(
		"Starting smith-zabbix-mqtt",
		slog.String("version", version),
		slog.String("Zabbix", cfg.Zabbix.Address),
		slog.String("MQTT", cfg.Mqtt.Address),
	)

	// Подключение к Zabbix
	nSession := zabbix.Session{
		URL:   cfg.Zabbix.Address,
		Token: cfg.Zabbix.Token,
	}

	version, err := nSession.GetVersion()
	if err != nil {
		log.Error(
			"Zabbix - ошибка подключения",
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	} else {
		log.Info(
			"Zabbix - успешное подключение",
			slog.String("APIVersion", version.String()),
		)
	}

	// Подключение к MQTT
	if cfg.Mqtt.Address == DEFAULT_BROKER_URL && isSocket(MOSQUITTO_SOCK_FILE) {
		log.Info("broker URL is default and mosquitto socket detected, trying to connect via it")
		cfg.Mqtt.Address = "unix://" + MOSQUITTO_SOCK_FILE
	}

	opts := setupMQTT(cfg)
	opts.SetOnConnectHandler(connectionManager.GetOnConnectHandler())

	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Error(
			"MQTT - ошибка подключения",
			slog.String("error", token.Error().Error()),
		)
		os.Exit(1)
	}

	defer client.Disconnect(250)

	triggerManager.SetClient(client)
	publicMetaData(client, cfg)

	// Создаем основной тикер для чтения триггеров
	ticker := time.NewTicker(cfg.UpdateInterval)
	defer ticker.Stop()

	// Тикер uptime, если указали true для публикации
	if cfg.VirtualDevice.Uptime {
		tickerUptime := time.NewTicker(time.Second)
		defer tickerUptime.Stop()
		startTime := time.Now()
		go func() {
			for range tickerUptime.C {
				if client.IsConnectionOpen() {
					pub(client, topicUptime, uptime(startTime))
				}
			}
		}()
	}

	// Формируем запрос для триггеров
	trigParam := makeTriggerParam(cfg)

	go func() {
		for range ticker.C {
			triggers, err := nSession.GetTriggers(*trigParam)
			if err != nil && !errors.Is(err, zabbix.ErrNotFound) {
				log.Error(
					"Zabbix - ошибка получения триггеров",
					slog.String("getTriggers", err.Error()),
				)
				continue
			}
			// Тут все хорошо, триггеры получены

			triggerManager.ActiveOFF()

			// Перебираем все активные триггеры
			if len(triggers) != 0 {
				for _, vTrig := range triggers {
					// Перебираем все хосты в триггере
					for _, vHost := range vTrig.Hosts {
						triggerManager.AppendSeverity(vHost.Hostname, vTrig.Severity)
					}
				}
			}

			// ВЫЧИСЛЯЕМ максимумы после сбора ВСЕХ триггеров
			triggerManager.CalculateMaxSeverities()

			if client.IsConnectionOpen() {
				triggerManager.PublicAllSeverity()
				if cfg.VirtualDevice.TotalTriggers {
					pub(client, topicTotalTriggers, fmt.Sprint(len(triggers)))
				}
			}

			if *debug {
				var s string
				s += fmt.Sprintf("Активных триггеров: %d\n", len(triggers))
				if cfg.Zabbix.Group != "" {
					s += "selectGroup: " + cfg.Zabbix.Group + "\n"
				}
				for _, vTrig := range triggers {

					s += fmt.Sprintf("ID:%s Prioritet:%d", vTrig.TriggerID, vTrig.Severity)
					for _, vHost := range vTrig.Hosts {
						s += "[" + vHost.Hostname + "]"
					}
					s += "\n"
				}
				log.Debug(s)
			}
		}
	}()

	// Создаем канал, который ждет получения данных о завершении работы
	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-exitCh
}
