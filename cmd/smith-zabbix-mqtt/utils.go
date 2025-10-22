package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/config"
	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/lib/logger/handlers/slogpretty"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/fabiang/go-zabbix"
)

// Публикация в MQTT
func pub(client mqtt.Client, topic string, msg string) {

	t := client.Publish(topic, QOS, true, msg)
	go func() {
		<-t.Done()
		if t.Error() != nil {
			log.Error(
				"Error publish MQTT",
				slog.String("error", t.Error().Error()),
			)
		}
	}()
}

// Проверяем, установлен или нет брокер Mosquitto на этом же сервере
func isSocket(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}

// Функция заполняет структуру запроса для триггеров
func makeTriggerParam(cfg *config.Config) *zabbix.TriggerGetParams {
	triggerParam := &zabbix.TriggerGetParams{
		SelectHosts: []string{"host"},
	}
	triggerParam.Filter = map[string]any{
		"value":  1, // Только активные триггеры
		"status": 0, // И только НЕ деактивированные
	}
	triggerParam.SortField = []string{"priority"}
	triggerParam.SortOrder = "DESC"
	triggerParam.OutputFields = []string{"triggerid", "priority"}

	if cfg.Zabbix.Group != "" {
		triggerParam.Group = cfg.Zabbix.Group
	}

	return triggerParam
}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case envLocal:
		log = setupPrettySlog()
	case envDev:
		log = slog.New(
			slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	default: // If env config is invalid, set prod settings by default due to security
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		)
	}

	return log
}

func setupPrettySlog() *slog.Logger {
	opts := slogpretty.PrettyHandlerOptions{
		SlogOpts: &slog.HandlerOptions{
			Level: slog.LevelDebug,
		},
	}

	handler := opts.NewPrettyHandler(os.Stdout)

	return slog.New(handler)
}

func uptime(startTime time.Time) string {

	durationSec := uint64(time.Since(startTime).Seconds())
	d := durationSec / 86400
	h := (durationSec - d*86400) / 3600
	m := (durationSec - d*86400 - h*3600) / 60
	sec := durationSec - d*86400 - h*3600 - m*60

	return fmt.Sprintf("%dд. %02d:%02d:%02d\n", d, h, m, sec)
}
