package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/config"
	"gopkg.in/yaml.v3"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type ConnectionManager struct {
	tm     *TriggerManager
	logger *slog.Logger
}

func (cm *ConnectionManager) GetOnConnectHandler() mqtt.OnConnectHandler {
	return func(client mqtt.Client) {
		cm.logger.Info("MQTT connection established")

		// Вызываем все нужные обработчики
		cm.tm.OnConnect(client)

	}
}

func main() {

	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(Version)
		os.Exit(0)
	}

	debug := flag.Bool("debug", false, "enable debugging")
	configPath := flag.String("configFile", DEFAULT_CONFIG_PATH, "config path")
	printConfig := flag.Bool("print-config", false, "print current configuration and exit")

	flag.Parse()

	// Читаем конфигурацию
	cfg := config.MustLoad(*configPath)

	if *printConfig {
		cfg.Mqtt.Password = "***"
		cfg.Zabbix.Token = "***"
		if data, err := yaml.Marshal(&cfg); err != nil {
			fmt.Fprint(os.Stderr, err)
		} else {
			fmt.Print(string(data))
		}
		os.Exit(0)
	}

	log := setupLogger(cfg.Env)

	// Создание и запуск приложения
	app := NewApplication(cfg, log, *debug)

	if err := app.Initialize(); err != nil {
		log.Error(
			"application initialization failed",
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
	defer app.Close()

	// Настройка graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
		<-sigCh
		cancel()
	}()

	if err := app.Run(ctx); err != nil {
		log.Error(
			"application finished with error",
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}

	log.Info("application stopped")
}
