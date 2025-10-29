package main

import "github.com/SmithLEDs/smith-zabbix-mqtt/internal/config"

// ControlDefinition описывает конфигурацию дополнительного контрола
type ControlDefinition struct {
	IsEnabled   func(*config.Config) bool // Функция проверки включен ли контрол
	CtrlID      string                    // Идентификатор контрола
	Type        ControlType               // Тип контрола (text/value)
	DefaultVal  any                       // Значение по умолчанию
	ReadOnly    bool                      // Режим только для чтения
	TitleRus    string                    // Русский заголовок
	TitleEng    string                    // Английский заголовок
	Description string                    // Описание контрола для документации
}

// defaultControls содержит список всех дополнительных контролов
var defaultControls = []ControlDefinition{
	{
		IsEnabled:   func(cfg *config.Config) bool { return cfg.VirtualDevice.Uptime },
		CtrlID:      CTRL_UPTIME,
		Type:        ControlTypeText,
		DefaultVal:  "0",
		ReadOnly:    true,
		TitleRus:    "Время работы",
		TitleEng:    "Uptime",
		Description: "Показывает время работы сервиса в формате дни чч:мм:сс",
	},
	{
		IsEnabled:   func(cfg *config.Config) bool { return cfg.VirtualDevice.TotalTriggers },
		CtrlID:      CTRL_TOTAL_TRIGGERS,
		Type:        ControlTypeValue,
		DefaultVal:  0,
		ReadOnly:    true,
		TitleRus:    "Активных триггеров",
		TitleEng:    "Total triggers",
		Description: "Показывает общее количество активных триггеров Zabbix",
	},
}
