package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/config"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/fabiang/go-zabbix"
)

type Application struct {
	logger *slog.Logger
	cfg    *config.Config
	zabbix *zabbix.Session
	mqtt   mqtt.Client
	tm     *TriggerManager
	debug  bool
}

// Создаём новое приложение
func NewApplication(cfg *config.Config, logger *slog.Logger, debug bool) *Application {
	return &Application{
		logger: logger,
		cfg:    cfg,
		debug:  debug,
	}
}

// Инициализируем компоненты приложения
func (app *Application) Initialize() error {
	if err := app.validateConfig(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	if err := app.connectToZabbix(); err != nil {
		return fmt.Errorf("zabbix connection failed: %w", err)
	}

	if err := app.connectToMQTT(); err != nil {
		return fmt.Errorf("mqtt connection failed: %w", err)
	}

	return nil
}

// Освобождаем ресурсы
func (app *Application) Close() {
	if app.mqtt != nil {
		app.mqtt.Disconnect(250)
	}
}

// Проверка конфигурации
func (app *Application) validateConfig() error {
	if app.cfg.Zabbix.Address == "" {
		return errors.New("zabbix address is required")
	}
	if app.cfg.Zabbix.Token == "" {
		return errors.New("zabbix token is required")
	}
	if app.cfg.Mqtt.Address == "" {
		return errors.New("mqtt address is required")
	}
	if app.cfg.UpdateInterval <= 0 {
		return errors.New("update interval must be positive")
	}
	if app.cfg.VirtualDevice.Name == "" {
		return errors.New("virtual device name is required")
	}
	if len(app.cfg.Hosts) == 0 {
		return errors.New("no hosts configured for monitoring")
	}
	return nil
}

// Подключаемся к Zabbix
func (app *Application) connectToZabbix() error {
	session := &zabbix.Session{
		URL:   app.cfg.Zabbix.Address,
		Token: app.cfg.Zabbix.Token,
	}

	version, err := session.GetVersion()
	if err != nil {
		return err
	}

	app.zabbix = session
	app.logger.Info("successfully connected to Zabbix",
		"version", version.String())

	return nil
}

// Подключаемся к MQTT брокеру
func (app *Application) connectToMQTT() error {
	brokerURL := app.cfg.Mqtt.Address
	if brokerURL == DEFAULT_BROKER_URL && isSocket(MOSQUITTO_SOCK_FILE) {
		app.logger.Info("using mosquitto socket")
		brokerURL = "unix://" + MOSQUITTO_SOCK_FILE
	}

	opts := setupMQTT(app.cfg, app.logger)
	app.tm = NewTriggerManager(app.cfg, app.logger, app.debug)

	connectionManager := &ConnectionManager{
		tm:     app.tm,
		logger: app.logger,
	}
	opts.SetOnConnectHandler(connectionManager.GetOnConnectHandler())

	app.mqtt = mqtt.NewClient(opts)
	if token := app.mqtt.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	app.tm.SetClient(app.mqtt)
	return nil
}

// Run запускает основную логику приложения
func (app *Application) Run(ctx context.Context) error {
	app.logger.Info(
		"Starting smith-zabbix-mqtt",
		slog.String("version", Version),
		slog.String("Zabbix", app.cfg.Zabbix.Address),
		slog.String("MQTT", app.cfg.Mqtt.Address),
	)

	// Запускаем обработчики
	errCh := make(chan error, 2)

	go app.runTriggerPolling(ctx, errCh)

	if app.cfg.VirtualDevice.Uptime {
		go func() {
			app.runUptimeUpdates(ctx)
		}()
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// runTriggerPolling запускает опрос триггеров
func (app *Application) runTriggerPolling(ctx context.Context, errCh chan error) {
	ticker := time.NewTicker(app.cfg.UpdateInterval)
	defer ticker.Stop()

	triggerParams := makeTriggerParam(app.cfg)

	for {
		select {
		case <-ticker.C:
			if err := app.pollTriggers(triggerParams); err != nil {
				errCh <- err
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// runUptimeUpdates запускает обновления uptime
func (app *Application) runUptimeUpdates(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ticker.C:
			if app.mqtt.IsConnectionOpen() {
				app.tm.UpdateUptime(uptime(startTime))
			}
		case <-ctx.Done():
			return
		}
	}
}

// pollTriggers опрашивает триггеры Zabbix
func (app *Application) pollTriggers(params *zabbix.TriggerGetParams) error {
	triggers, err := app.zabbix.GetTriggers(*params)

	if err != nil {
		if errors.Is(err, zabbix.ErrNotFound) {
			// Нет активных триггеров - это нормальная ситуация
			app.logger.Debug("No active triggers found in Zabbix")
			triggers = []zabbix.Trigger{} // Явно указываем пустой список
		} else {
			// Реальная ошибка - логируем, но НЕ сбрасываем состояние
			app.logger.Error(
				"Failed to get triggers from Zabbix, keeping previous state",
				slog.String("error", err.Error()),
			)
			return nil // Не прерываем работу, ждем следующей попытки
		}
	}

	app.tm.ResetHosts()

	// Перебираем все активные триггеры
	if len(triggers) > 0 {
		for _, trigger := range triggers {
			// Перебираем все хосты в триггере
			for _, host := range trigger.Hosts {
				app.tm.AppendHostSeverity(host.Hostname, trigger.Severity)
			}
		}
	}

	if app.mqtt != nil && app.mqtt.IsConnectionOpen() {
		app.tm.PublishAllSeverities()
		app.tm.UpdateTotalTriggers(len(triggers))
	}

	if app.debug {
		app.logTriggers(triggers)
	}

	return nil
}

// logTriggers логирует информацию о триггерах
func (app *Application) logTriggers(triggers []zabbix.Trigger) {
	logMsg := fmt.Sprintf("Active triggers: %d\n", len(triggers))
	if app.cfg.Zabbix.Group != "" {
		logMsg += "Group: " + app.cfg.Zabbix.Group + "\n"
	}

	for _, trigger := range triggers {
		hostNames := make([]string, len(trigger.Hosts))
		for i, host := range trigger.Hosts {
			hostNames[i] = host.Hostname
		}
		logMsg += fmt.Sprintf("ID:%s Priority:%d Hosts:%v\n",
			trigger.TriggerID, trigger.Severity, hostNames)
	}

	app.logger.Debug(logMsg)
}
