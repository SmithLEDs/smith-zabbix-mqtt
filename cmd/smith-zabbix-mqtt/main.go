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
	version  = "0.0.3"
	driver   = "smith-zabbix-mqtt"
	rus      = "Zabbix2MQTT"
	eng      = "Zabbix2MQTT"

	DEFAULT_CONFIG_PATH = "/etc/smith-zabbix-mqtt/config.yaml"
	MOSQUITTO_SOCK_FILE = "/var/run/mosquitto/mosquitto.sock"
	DEFAULT_BROKER_URL  = "tcp://localhost:1883"
)

var cfg *config.Config
var log *slog.Logger
var startTime time.Time

var s *triggers

func init() {
	startTime = time.Now()
	log = setupLogger(envLocal)
	s = makeTriggerStruct()
}

func main() {

	debug := flag.Bool("debug", false, "Enable debugging")
	configPath := flag.String("configFile", DEFAULT_CONFIG_PATH, "Config path")

	flag.Parse()

	cfg = config.MustLoad(*configPath)

	s.readConfig(cfg)

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

	// Публикация для виртуального устройства в WirenBoard
	order := publicMainDevice(client, cfg.VirtualDevice)
	publicControlsDevice(client, cfg, order)

	// Создаем основной тикер для чтения триггеров
	ticker := time.NewTicker(cfg.UpdateInterval)
	defer ticker.Stop()

	// Тикер uptime, если указали true для публикации
	if cfg.VirtualDevice.Uptime {
		tickerUptime := time.NewTicker(time.Second)
		defer tickerUptime.Stop()
		go func() {
			for range tickerUptime.C {
				if client.IsConnectionOpen() {
					pub(client, topicUptime, uptime(startTime))
				}
			}
		}()
	}

	// Формируем запрос для триггеров
	trigParam := makeTriggerParam()

	go func() {
		for range ticker.C {
			// Подключение MQTT не активно
			if !client.IsConnectionOpen() {
				continue
			}

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
				if cfg.VirtualDevice.TotalTriggers {
					pub(client, topicTotalTriggers, fmt.Sprint(len(triggers)))
				}
			}

			s.activeOFF()

			// Перебираем все активные триггеры
			if len(triggers) != 0 {
				for _, vTrig := range triggers {
					// Перебираем все хосты в триггере
					for _, vHost := range vTrig.Hosts {
						s.writeSeverity(vHost.Hostname, vTrig.Severity)
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

			s.publicSeverity(client)

		}
	}()

	// Создаем канал, который ждет получения данных о завершении работы
	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-exitCh
}
