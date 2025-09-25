package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"maps"
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
	version  = "0.0.1"

	DEFAULT_CONFIG_PATH = "/etc/smith-zabbix-mqtt/config.yaml"
	MOSQUITTO_SOCK_FILE = "/var/run/mosquitto/mosquitto.sock"
	DEFAULT_BROKER_URL  = "tcp://localhost:1883"
)

var cfg *config.Config
var log *slog.Logger
var startTime time.Time
var severity = map[int]string{0: "2", 1: "2", 2: "3", 3: "3", 4: "4", 5: "4"}

func init() {
	startTime = time.Now()
	log = setupLogger(envLocal)
}

func main() {

	debug := flag.Bool("debug", false, "Enable debugging")
	configPath := flag.String("configFile", DEFAULT_CONFIG_PATH, "Config path")

	flag.Parse()

	cfg = config.MustLoad(*configPath)

	// Если в конфигурации есть переобределения приоритета
	if len(cfg.Severity) > 0 {
		maps.Copy(severity, cfg.Severity)
	}

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
	client := mqtt.NewClient(setupMQTT())

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Error(
			"MQTT - ошибка подключения",
			slog.String("error", token.Error().Error()),
		)
		os.Exit(1)
	}
	defer client.Disconnect(250)

	// Создаем тикер
	ticker := time.NewTicker(cfg.UpdateInterval)
	defer ticker.Stop()

	// Формируем запрос для триггеров
	trigParam := makeTriggerParam()

	// Создаем мапу для хранения хостов и приоритета
	// Сразу заполняем хостами из конфиг файла с приоритетом -1
	trig := make(map[string]int)
	for zabbixHost := range cfg.Topics.Servers {
		trig[zabbixHost] = -1
	}

	go func() {
		for range ticker.C {
			triggers, err := nSession.GetTriggers(trigParam)
			if err != nil && !errors.Is(err, zabbix.ErrNotFound) {
				log.Error(
					"Zabbix - ошибка получения триггеров",
					slog.String("getTriggers", err.Error()),
				)
				continue
			}
			// Тут все хорошо, триггеры получены

			if *debug {
				log.Debug(fmt.Sprintf("Активных триггеров: %d", len(triggers)))
			}

			if client.IsConnectionOpen() {
				if cfg.Topics.TotalTriggers != "" {
					pub(client, cfg.Topics.TotalTriggers, fmt.Sprint(len(triggers)))
				}
				if cfg.Topics.Uptime != "" {
					pub(client, cfg.Topics.Uptime, uptime(startTime))
				}
			}

			// Пропуск, если нет активных триггеров
			if len(triggers) == 0 {
				continue
			}

			// Перебираем все активные триггеры
			for _, vTrig := range triggers {
				// Перебираем все хосты в триггере
				for _, vHost := range vTrig.Hosts {
					// Если текущего хоста нет в мапе, то добавляем
					if val, ok := trig[vHost.Hostname]; !ok {
						trig[vHost.Hostname] = vTrig.Severity
					} else if vTrig.Severity > val {
						// Если хост существует в мапе и его приоритет ниже текущего приоритета триггера
						trig[vHost.Hostname] = vTrig.Severity
					}
				}
			}

			if *debug {
				var s string
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

			// Подключение MQTT не активно
			if !client.IsConnectionOpen() {
				continue
			}

			// Перебираем мапу и публикуем статусы
			for host, priority := range trig {
				if topic, ok := cfg.Topics.Servers[host]; ok {
					// Нашли нужный сервер в конфиг файле и публикуем в топик приоритет
					pub(client, topic, convertPriority(priority))
					trig[host] = -1
				}
			}
		}
	}()

	// Создаем канал, который ждет получения данных о завершении работы
	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-exitCh
}
