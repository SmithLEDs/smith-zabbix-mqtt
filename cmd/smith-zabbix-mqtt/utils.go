package main

import (
	"log/slog"
	"os"
	"time"

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
func makeTriggerParam() zabbix.TriggerGetParams {
	triggerParam := zabbix.TriggerGetParams{
		SelectHosts: []string{"host"},
	}
	filter := make(map[string]any)
	filter["value"] = 1
	triggerParam.Filter = filter
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

type Timespan time.Duration

func (t Timespan) Format(format string) string {
	return time.Unix(0, 0).UTC().Add(time.Duration(t)).Format(format)
}

func uptime(startTime time.Time) string {
	t := time.Since(startTime)
	return Timespan(t).Format("15:04:05")
}
