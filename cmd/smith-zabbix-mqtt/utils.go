package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/config"
	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/lib/logger/handlers/slogpretty"
	"github.com/fabiang/go-zabbix"
)

// Общая функция для сериализации в JSON
func marshalToJSON(v any) (string, error) {
	jsonData, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
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

func Err(err error) slog.Attr {
	return slog.Any("error", err)
}

// Возвращаем время работы в строковом представлении
func uptime(startTime time.Time) string {
	durationSec := uint64(time.Since(startTime).Seconds())

	d := durationSec / 86400
	r := (durationSec - d*86400)
	h := r / 3600
	r -= h * 3600
	m := r / 60
	sec := r - m*60

	return fmt.Sprintf("%dд. %02d:%02d:%02d", d, h, m, sec)
}
