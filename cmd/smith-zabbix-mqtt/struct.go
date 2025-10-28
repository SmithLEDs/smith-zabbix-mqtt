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
	ControlTypeText  ControlType = "text"
	ControlTypeValue ControlType = "value"
)

// Представляет типы контролов виртуального устройства
type ControlType string

// Структура meta-данных для контролов виртуального устройства
type ControlMeta struct {
	Title    Lang            `json:"title"`
	ReadOnly bool            `json:"readonly"`
	Type     ControlType     `json:"type"`
	Value    any             `json:"value"`
	Order    int             `json:"order,omitempty"`
	Enum     map[string]Lang `json:"enum,omitempty"`
}

// Структура meta-данных главного виртуального устройства
type MainDeviceMeta struct {
	Title   Lang   `json:"title"`
	Driver  string `json:"driver"`
	Version string `json:"version"`
	topic   string `json:"-"`
}

// Структура для языков
type Lang struct {
	Rus string `json:"ru"`
	Eng string `json:"en"`
}

// Состояние хоста
type HostTrigger struct {
	topic           string // Топик MQTT, куда публиковать приоритет
	meta            ControlMeta
	currentSeverity int   // Самый максимальный текущий приоритет хоста
	lastSeverity    int   // Предедущий приоритет хоста
	severities      []int // Массив приоритетов для хоста (добавление в append)
	active          bool  // Активность триггера (Активно, если API Zabbix выдал триггер на данный хост)
}

type TriggerManager struct {
	mu            sync.RWMutex
	triggers      map[string]*HostTrigger // Мапа для хостов
	severityMap   map[int]string          // Мапа для конвертации приоритетов
	client        mqtt.Client             // Клиент MQTT для публикации топиков
	meta          MainDeviceMeta
	uptime        Control
	totalTriggers Control
	cfg           *config.Config // Указатель на структуру конфигурации
	debug         bool
	log           *slog.Logger
}

// Структура для сторонних контролов
type Control struct {
	meta  ControlMeta
	topic string
}

func (cm *ControlMeta) getMeta() string {
	return marshalToJSON(cm)
}

func (md *MainDeviceMeta) getMeta() string {
	return marshalToJSON(md)
}

// NewTriggerManager создает новый менеджер триггеров
func NewTriggerManager(cfg *config.Config, logger *slog.Logger, debug bool) *TriggerManager {
	tm := &TriggerManager{
		triggers:    make(map[string]*HostTrigger),
		severityMap: createDefaultSeverityMap(),
		cfg:         cfg,
		log:         logger,
		debug:       debug,
	}

	tm.initializeFromConfig()

	return tm
}

// createDefaultSeverityMap создает маппинг severity по умолчанию
func createDefaultSeverityMap() map[int]string {
	return map[int]string{
		0: "0", 1: "1", 2: "2", 3: "3", 4: "4", 5: "5",
	}
}

// Инициализация из конфигурации
func (tm *TriggerManager) initializeFromConfig() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	order := tm.initializeAdditionalControls()
	tm.initializeHosts(order)
	tm.initializeDeviceMeta()
	tm.applyCustomSeverityMapping()

	if tm.debug {
		tm.log.Debug("TriggerManager initialized",
			slog.Int("hosts_count", len(tm.triggers)),
			slog.Any("severity_map", tm.severityMap),
		)
	}
}

// initializeAdditionalControls инициализирует дополнительные контролы
func (tm *TriggerManager) initializeAdditionalControls() int {
	order := 1

	deviceName := tm.cfg.VirtualDevice.Name

	if tm.cfg.VirtualDevice.Uptime {
		tm.uptime = Control{
			topic: fmt.Sprintf("/devices/%s/controls/uptime", deviceName),
			meta: ControlMeta{
				Value:    "0",
				Type:     ControlTypeText,
				ReadOnly: true,
				Order:    order,
				Title: Lang{
					Rus: "Время работы",
					Eng: "Uptime",
				},
			},
		}
		order++
	}

	if tm.cfg.VirtualDevice.TotalTriggers {
		tm.totalTriggers = Control{
			topic: fmt.Sprintf("/devices/%s/controls/totalTriggers", deviceName),
			meta: ControlMeta{
				Value:    0,
				Type:     ControlTypeValue,
				ReadOnly: true,
				Order:    order,
				Title: Lang{
					Rus: "Активных триггеров",
					Eng: "Total triggers",
				},
			},
		}
		order++
	}

	return order
}

// initializeHosts инициализирует хосты для мониторинга
func (tm *TriggerManager) initializeHosts(startOrder int) {
	enumMap := tm.createEnumMap()
	order := startOrder

	for _, host := range tm.cfg.Hosts {
		hostID := strings.ReplaceAll(host, " ", "_")
		topic := fmt.Sprintf("/devices/%s/controls/%s", tm.cfg.VirtualDevice.Name, hostID)

		tm.triggers[host] = &HostTrigger{
			topic:           topic,
			currentSeverity: SEVERITY_UNDEFINED,
			lastSeverity:    SEVERITY_UNDEFINED,
			active:          false,
			meta: ControlMeta{
				Value:    2, // Normal state
				Type:     ControlTypeValue,
				ReadOnly: false,
				Order:    order,
				Title: Lang{
					Eng: host,
					Rus: host,
				},
				Enum: enumMap,
			},
		}
		order++

		if tm.debug {
			tm.log.Debug("added host",
				slog.String("host", host),
				slog.String("topic", topic))
		}
	}
}

// initializeDeviceMeta инициализирует метаданные устройства
func (tm *TriggerManager) initializeDeviceMeta() {
	tm.meta = MainDeviceMeta{
		topic:   fmt.Sprintf("/devices/%s/meta", tm.cfg.VirtualDevice.Name),
		Driver:  Driver,
		Version: Version,
		Title: Lang{
			Rus: AppNameRus,
			Eng: AppNameEng,
		},
	}
}

// applyCustomSeverityMapping применяет пользовательские настройки severity
func (tm *TriggerManager) applyCustomSeverityMapping() {
	if len(tm.cfg.Severity) > 0 {
		maps.Copy(tm.severityMap, tm.cfg.Severity)
	}
}

// Создание карты enum для контролов
func (tm *TriggerManager) createEnumMap() map[string]Lang {
	return map[string]Lang{
		"2": {Rus: "Норма", Eng: "Normal"},
		"3": {Rus: "Внимание", Eng: "Warning"},
		"4": {Rus: "Авария", Eng: "Alarm"},
	}
}

// Обновление значения uptime
func (tm *TriggerManager) UpdateUptime(uptime string) {
	if tm.uptime.topic != "" {
		tm.publish(tm.uptime.topic, uptime)
	}
}

// Обновление количества триггеров
func (tm *TriggerManager) UpdateTotalTriggers(count int) {
	if tm.cfg.VirtualDevice.TotalTriggers && tm.totalTriggers.topic != "" {
		tm.publish(tm.totalTriggers.topic, fmt.Sprint(count))
	}
}

// Функция конвертации приоритетов
func (tm *TriggerManager) convertSeverity(severity int) string {
	if val, ok := tm.severityMap[severity]; ok {
		return val
	}
	// Если значения для конвертации не нашлось, то возвращаем как есть
	return fmt.Sprint(severity)
}

// Задаем MQTT клиента
func (tm *TriggerManager) SetClient(client mqtt.Client) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.client = client
}

// При успешном подключении к MQTT брокеру отправляем все метаданные и значения
func (tm *TriggerManager) OnConnect(client mqtt.Client) {
	if tm.debug {
		tm.log.Debug("Publishing all metadata on MQTT connect")
	}

	// Публикуем метаданные главного устройства
	tm.publish(tm.meta.topic, tm.meta.getMeta())

	// Публикуем метаданные и значения для дополнительных контролов
	if tm.cfg.VirtualDevice.Uptime {
		tm.publish(tm.uptime.topic+"/meta", tm.uptime.meta.getMeta())
		tm.publish(tm.uptime.topic, "0") // Начальное значение uptime
	}

	if tm.cfg.VirtualDevice.TotalTriggers {
		tm.publish(tm.totalTriggers.topic+"/meta", tm.totalTriggers.meta.getMeta())
		tm.publish(tm.totalTriggers.topic, "0") // Начальное значение totalTriggers
	}

	// Публикуем метаданные и значения для всех триггеров
	for host, trigger := range tm.triggers {
		tm.publish(trigger.topic+"/meta", trigger.meta.getMeta())
		tm.publish(trigger.topic, tm.convertSeverity(trigger.currentSeverity))

		if tm.debug {
			tm.log.Debug("Published host metadata",
				slog.String("host", host),
				slog.String("topic", trigger.topic))
		}
	}
}

// Внутренний метод для публикации в MQTT брокер
func (tm *TriggerManager) publish(topic string, payload string) {
	if tm.client == nil {
		return
	}

	token := tm.client.Publish(topic, QOS, true, payload)

	// Не забываем про асинхронность
	go func() {
		<-token.Done()
		if token.Error() != nil {
			tm.log.Error(
				"MQTT publish failed",
				slog.String("topic", topic),
				slog.String("payload", payload),
				slog.String("error", token.Error().Error()),
			)
		}
	}()
}

// Деактивируем все триггеры перед новым опросом
func (tm *TriggerManager) ResetHosts() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, trigger := range tm.triggers {
		trigger.active = false
		trigger.severities = trigger.severities[:0]
	}
}

// Добавляем приоритет в хост
func (tm *TriggerManager) AppendHostSeverity(host string, severity int) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if trigger, ok := tm.triggers[host]; ok {
		trigger.severities = append(trigger.severities, severity)
		trigger.active = true
	}
}

// Функция возвращает максимальное число из слайса
func (tm *TriggerManager) calculateMaxSeverity(severities []int) int {
	if len(severities) == 0 {
		return SEVERITY_UNDEFINED
	}
	max := severities[0]
	for i := 1; i < len(severities); i++ {
		if severities[i] > max {
			max = severities[i]
		}
	}
	return max
}

// PublishAllSeverities публикует все измененные severity
func (tm *TriggerManager) PublishAllSeverities() {
	publications := tm.collectPublications()

	if len(publications) > 0 {
		for topic, severity := range publications {
			tm.publish(topic, tm.convertSeverity(severity))
		}
	}
}

// Собираем триггеры, которые изменились
func (tm *TriggerManager) collectPublications() map[string]int {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	publications := make(map[string]int)

	for host, trigger := range tm.triggers {
		if trigger.topic == "" {
			continue
		}

		if !trigger.active {
			if trigger.currentSeverity != SEVERITY_UNDEFINED {
				tm.resetHostSeverity(host, trigger, publications)
			}
			continue
		}

		// Для активного хоста вычисляем максимальный severity
		newSeverity := tm.calculateMaxSeverity(trigger.severities)

		if newSeverity != trigger.lastSeverity {
			tm.updateHostSeverity(host, trigger, newSeverity, publications)
		}

	}

	return publications
}

// resetHostSeverity сбрасывает severity для неактивного хоста
func (tm *TriggerManager) resetHostSeverity(host string, trigger *HostTrigger, publications map[string]int) {
	if tm.debug {
		tm.log.Debug("Resetting severity for inactive host",
			slog.String("host", host),
			slog.Int("old_severity", trigger.currentSeverity))
	}
	trigger.currentSeverity = SEVERITY_UNDEFINED
	trigger.lastSeverity = SEVERITY_UNDEFINED
	publications[trigger.topic] = SEVERITY_UNDEFINED
}

// updateHostSeverity обновляет severity для активного хоста
func (tm *TriggerManager) updateHostSeverity(host string, trigger *HostTrigger, newSeverity int, publications map[string]int) {
	if tm.debug {
		tm.log.Debug("Severity changed for host",
			slog.String("host", host),
			slog.Int("old_severity", trigger.currentSeverity),
			slog.Int("new_severity", newSeverity))
	}
	publications[trigger.topic] = newSeverity
	trigger.currentSeverity = newSeverity
	trigger.lastSeverity = newSeverity
}
