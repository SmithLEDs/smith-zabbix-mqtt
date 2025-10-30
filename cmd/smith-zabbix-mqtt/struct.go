package main

import (
	"fmt"
	"log/slog"
	"strconv"
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
	mu          sync.RWMutex
	triggers    map[string]*HostTrigger // Мапа для хостов
	severityMap map[int]string          // Мапа для конвертации приоритетов
	client      mqtt.Client             // Клиент MQTT для публикации топиков
	meta        MainDeviceMeta
	//uptime        Control
	//totalTriggers Control
	cfg      *config.Config // Указатель на структуру конфигурации
	debug    bool
	log      *slog.Logger
	controls map[string]*Control
}

// Структура для сторонних контролов
type Control struct {
	meta  ControlMeta
	topic string
}

// NewTriggerManager создает новый менеджер триггеров
func NewTriggerManager(cfg *config.Config, logger *slog.Logger, debug bool) *TriggerManager {
	tm := &TriggerManager{
		triggers:    make(map[string]*HostTrigger),
		controls:    make(map[string]*Control),
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

// initializeAdditionalControls инициализирует дополнительные контролы на основе
// предопределенных описаний из controls.go
func (tm *TriggerManager) initializeAdditionalControls() int {
	order := 1
	deviceName := tm.cfg.VirtualDevice.Name

	for _, ctrl := range defaultControls {
		if !ctrl.IsEnabled(tm.cfg) {
			continue
		}

		tm.controls[ctrl.CtrlID] = &Control{
			topic: fmt.Sprintf("/devices/%s/controls/%s", deviceName, ctrl.CtrlID),
			meta: ControlMeta{
				Value:    ctrl.DefaultVal,
				Type:     ctrl.Type,
				ReadOnly: ctrl.ReadOnly,
				Order:    order,
				Title: Lang{
					Rus: ctrl.TitleRus,
					Eng: ctrl.TitleEng,
				},
			},
		}
		order++

		if tm.debug {
			tm.log.Debug("initialized control",
				slog.String("id", ctrl.CtrlID),
				slog.String("type", string(ctrl.Type)))
		}
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
		for s, newS := range tm.cfg.Severity {
			i, err := strconv.Atoi(s)
			if err != nil {
				tm.log.Error("severity Atoi", Err(err))
				continue
			}
			tm.severityMap[i] = newS
		}
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
	if val, ok := tm.controls[CTRL_UPTIME]; ok && val.topic != "" {
		tm.publish(val.topic, uptime)
	}
}

// Обновление количества триггеров
func (tm *TriggerManager) UpdateTotalTriggers(count int) {
	if val, ok := tm.controls[CTRL_TOTAL_TRIGGERS]; ok && val.topic != "" {
		tm.publish(val.topic, fmt.Sprint(count))
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
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.debug {
		tm.log.Debug("Publishing all metadata on MQTT connect")
	}

	// Публикуем метаданные главного устройства
	if tm.meta.topic != "" {
		if m, err := marshalToJSON(tm.meta); err != nil {
			tm.log.Error("get meta main device", Err(err))
		} else {
			tm.publish(tm.meta.topic, m)
		}
	}

	// Публикуем meta-данные дополнительных контролов
	for control, data := range tm.controls {
		if m, err := marshalToJSON(data.meta); err != nil {
			tm.log.Error("get meta", slog.String("control", control), Err(err))
		} else {
			tm.publish(data.topic+META_SUFFIX, m)
		}
	}

	// Публикуем метаданные и текущие значения для всех триггеров
	for host, trigger := range tm.triggers {
		if trigger.topic == "" {
			continue
		}
		if m, err := marshalToJSON(trigger.meta); err != nil {
			tm.log.Error("get meta trigger", slog.String("host", host), Err(err))
		} else {
			tm.publish(trigger.topic+META_SUFFIX, m)
		}

		// Значение публикуем только если топик не пустой
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

	// If connection is not open, skip publish
	if !tm.client.IsConnectionOpen() {
		if tm.debug {
			tm.log.Debug("mqtt connection not open, skipping publish",
				slog.String("topic", topic))
		}
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
