package main

import (
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"

	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/config"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	SEVERITY_NORMAL    = 2
	SEVERITY_UNDEFINED = -1
)

type trigger struct {
	topic        string // Топик MQTT, куда публиковать приоритет
	severity     int    // Самый максимальный текущий приоритет хоста
	lastSeverity int    // Предедущий приоритет хоста
	mSeverity    []int  // Массив приоритетов для хоста
	active       bool   // Активность триггера (Активно, если API Zabbix выдал триггер на данный хост)
}

type TriggerManager struct {
	mu              sync.RWMutex
	triggers        map[string]*trigger // Мапа для хостов
	convertSeverity map[int]string      // Мапа для конвертации приоритетов
	client          mqtt.Client         // Клиент MQTT для публикации топиков
	cfg             *config.Config      // Указатель на структуру конфигурации
}

// NewTriggerManager создает новый менеджер триггеров
func NewTriggerManager(cfg *config.Config) *TriggerManager {
	tm := &TriggerManager{
		triggers:        make(map[string]*trigger),
		convertSeverity: map[int]string{0: "2", 1: "2", 2: "3", 3: "3", 4: "4", 5: "4"},
		cfg:             cfg,
	}

	tm.initializeFromConfig()
	return tm
}

// Инициализация из конфигурации
func (tm *TriggerManager) initializeFromConfig() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, host := range tm.cfg.Hosts {
		hostNoSpace := strings.ReplaceAll(host, " ", "_")
		topic := fmt.Sprintf("/devices/%s/controls/%s", tm.cfg.VirtualDevice.Name, hostNoSpace)

		tm.triggers[host] = &trigger{
			topic:        topic,
			severity:     SEVERITY_UNDEFINED,
			lastSeverity: SEVERITY_UNDEFINED,
			active:       false,
		}
	}

	// Если в конфигурации есть переобределения приоритета
	if len(tm.cfg.Severity) > 0 {
		maps.Copy(tm.convertSeverity, tm.cfg.Severity)
	}
}

// Функция конвертации приоритетов
func (tm *TriggerManager) convert(severity int) string {
	if val, ok := tm.convertSeverity[severity]; ok {
		return val
	}
	return fmt.Sprint(SEVERITY_NORMAL)
}

// Задаем MQTT клиента
func (tm *TriggerManager) SetClient(client mqtt.Client) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.client = client
}

// При успешном подключении к MQTT брокеру отправляем все приоритеты
func (tm *TriggerManager) OnConnect(client mqtt.Client) {
	for _, trigger := range tm.triggers {
		tm.publishSeverity(trigger.topic, trigger.severity)
	}
}

// Внутренний метод для публикации в MQTT брокер с конвертацией приоритета
func (tm *TriggerManager) publishSeverity(topic string, severity int) {
	if tm.client == nil {
		return
	}

	token := tm.client.Publish(topic, QOS, true, tm.convert(severity))

	// Не забываем про асинхронность
	go func() {
		<-token.Done()
		if token.Error() != nil {
			log.Error(
				"Error publish MQTT",
				slog.String("error", token.Error().Error()),
			)
		}
	}()
}

// Деактивируем все триггеры перед новым опросом
func (tm *TriggerManager) ActiveOFF() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, trigger := range tm.triggers {
		trigger.active = false
		trigger.mSeverity = trigger.mSeverity[:0]
	}
}

// Записываем приоритет в хост
func (tm *TriggerManager) AppendSeverity(host string, severity int) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if trigger, ok := tm.triggers[host]; ok {
		trigger.mSeverity = append(trigger.mSeverity, severity)
		trigger.active = true
	}
}

// Вычисляем максимальный приоритет в каждом хосте
func (tm *TriggerManager) CalculateMaxSeverities() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, trigger := range tm.triggers {
		if !trigger.active || len(trigger.mSeverity) == 0 {
			continue
		}

		maxSeverity := -1
		for _, value := range trigger.mSeverity {
			if value > maxSeverity {
				maxSeverity = value
			}
		}

		trigger.severity = maxSeverity
	}
}

func (tm *TriggerManager) PublicAllSeverity() {
	tm.mu.Lock()

	publications := make(map[string]int)

	for _, trigger := range tm.triggers {
		if trigger.topic == "" {
			continue
		}

		if !trigger.active {
			if trigger.severity != SEVERITY_UNDEFINED {
				trigger.severity = SEVERITY_UNDEFINED
				trigger.lastSeverity = SEVERITY_UNDEFINED
				publications[trigger.topic] = SEVERITY_UNDEFINED
			}
			continue
		}

		if trigger.severity != trigger.lastSeverity {
			publications[trigger.topic] = trigger.severity
			trigger.lastSeverity = trigger.severity
		}

	}

	tm.mu.Unlock()
	for topic, severity := range publications {
		tm.publishSeverity(topic, severity)
	}
}
