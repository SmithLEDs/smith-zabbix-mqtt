package main

import (
	"maps"

	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/config"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type trigger struct {
	topic        string // Топик MQTT, куда публиковать приоритет
	severity     int    // Самый максимальный текущий приоритет хоста
	lastSeverity int    // Предедущий приоритет хоста
	mSeverity    []int  // Массив приоритетов для хоста
	active       bool   // Активность триггера (Активно, если API Zabbix выдал триггер на данный хост)
}

type triggers struct {
	m               map[string]trigger // Мапа для хостов
	convertSeverity map[int]string     // Мапа для конвертации приоритетов
}

// Создаем структуру для хранения триггеров
func makeTriggerStruct() *triggers {
	var cfg triggers
	cfg.m = make(map[string]trigger)
	cfg.convertSeverity = map[int]string{0: "2", 1: "2", 2: "3", 3: "3", 4: "4", 5: "4"}
	return &cfg

}

// Читаем из конфигурации все хосты и топики для публикации
func (t *triggers) readConfig(conf *config.Config) {
	for host, topic := range conf.Topics.Servers {
		val := trigger{
			topic:        topic,
			severity:     0,
			lastSeverity: 0,
			active:       false,
		}
		t.m[host] = val
	}
	// Если в конфигурации есть переобределения приоритета
	if len(conf.Severity) > 0 {
		maps.Copy(t.convertSeverity, conf.Severity)
	}
}

func (t *triggers) activeOFF() {
	for host, trigger := range t.m {
		trigger.active = false
		trigger.mSeverity = trigger.mSeverity[:0]
		t.m[host] = trigger
	}
}

// Записываем приоритет в хост
func (t *triggers) writeSeverity(host string, severity int) {
	if val, ok := t.m[host]; ok {
		val.mSeverity = append(val.mSeverity, severity)

		maxSeverity := -1
		for _, v := range val.mSeverity {
			if v > maxSeverity {
				maxSeverity = v
			}
		}

		val.severity = maxSeverity

		val.active = true
		t.m[host] = val
	}
}

func (t *triggers) publicSeverity(client mqtt.Client) {
	for host, trigger := range t.m {
		if trigger.topic == "" {
			continue
		}

		//fmt.Printf("\t%s \t(%d : %d)\t[%v]\n", trigger.topic, trigger.severity, trigger.lastSeverity, trigger.active)

		if !trigger.active {
			if trigger.severity != -1 {
				trigger.severity = -1
				trigger.lastSeverity = -1
				t.m[host] = trigger
				pub(client, trigger.topic, t.convertPriority(trigger.severity))
			}
			continue
		}

		if trigger.severity == trigger.lastSeverity {
			continue
		}

		pub(client, trigger.topic, t.convertPriority(trigger.severity))

		trigger.lastSeverity = trigger.severity

		t.m[host] = trigger
	}
}

func (t *triggers) convertPriority(priority int) string {
	if val, ok := t.convertSeverity[priority]; ok {
		return val
	}
	return "2"
}
