package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/config"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var topicUptime string
var topicTotalTriggers string

// Структура данных главного виртуального устройства
type mainDeviceMQTT struct {
	Driver  string `json:"driver"`
	Title   lang   `json:"title"`
	Version string `json:"version"`
}

// Структура данных для контролов виртуального устройства
type controlMQTT struct {
	Title    lang            `json:"title"`
	ReadOnly bool            `json:"readonly"`
	Type     string          `json:"type"`
	Value    int             `json:"value"`
	Order    int             `json:"order,omitempty"`
	Enum     map[string]lang `json:"enum,omitempty"`
}

type lang struct {
	Rus string `json:"ru"`
	Eng string `json:"en"`
}

// Публикаем meta данные для виртуального устройства
func publicMetaData(client mqtt.Client, cfg *config.Config) {

	// Публикуем общие сведения о виртуальном устройстве
	mainTopic := fmt.Sprintf("/devices/%s/meta", cfg.VirtualDevice.Name)
	mainMessage := mainDeviceMQTT{
		Driver:  driver,
		Version: version,
		Title: lang{
			Rus: rus,
			Eng: eng,
		},
	}

	if jsonData, err := json.Marshal(mainMessage); err == nil {
		pub(client, mainTopic, string(jsonData))
	}

	order := 1

	// Создаем meta топик для uptime
	if cfg.VirtualDevice.Uptime {
		topicUptime = fmt.Sprintf("/devices/%s/controls/uptime", cfg.VirtualDevice.Name)
		message := controlMQTT{
			Value:    0,
			Type:     "text",
			ReadOnly: true,
			Order:    order,
			Title: lang{
				Rus: "Время работы",
				Eng: "Uptime",
			},
		}
		if jsonData, err := json.Marshal(message); err == nil {
			pub(client, topicUptime+"/meta", string(jsonData))
			order++
		}
	}

	// Создаем meta топик для totalTriggers
	if cfg.VirtualDevice.TotalTriggers {
		topicTotalTriggers = fmt.Sprintf("/devices/%s/controls/totalTriggers", cfg.VirtualDevice.Name)
		message := controlMQTT{
			Value:    0,
			Type:     "value",
			ReadOnly: true,
			Order:    order,
			Title: lang{
				Rus: "Активных триггеров",
				Eng: "Total triggers",
			},
		}
		if jsonData, err := json.Marshal(message); err == nil {
			pub(client, topicTotalTriggers+"/meta", string(jsonData))
			order++
		}
	}

	// Мапа для перечислений (enum) для контрола
	e := map[string]lang{
		"2": {
			Rus: "Норма",
			Eng: "Normal",
		},
		"3": {
			Rus: "Внимание",
			Eng: "Warning",
		},
		"4": {
			Rus: "Авария",
			Eng: "Alarm",
		},
	}

	// Перебираем все хосты и создаем meta-топики контролов для них
	for _, host := range cfg.Hosts {
		hostNoSpace := strings.ReplaceAll(host, " ", "_")
		topicMeta := fmt.Sprintf("/devices/%s/controls/%s/meta", cfg.VirtualDevice.Name, hostNoSpace)
		message := controlMQTT{
			Value:    2,
			Type:     "value",
			ReadOnly: true,
			Order:    order,
			Title: lang{
				Eng: host,
			},
			Enum: e,
		}

		if jsonData, err := json.Marshal(message); err == nil {
			pub(client, topicMeta, string(jsonData))
			order++
		}
	}

}
