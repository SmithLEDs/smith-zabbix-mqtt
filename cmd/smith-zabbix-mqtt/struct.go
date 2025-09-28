package main

import (
	"maps"

	"github.com/SmithLEDs/smith-zabbix-mqtt/internal/config"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type allTriggers struct {
	topic        string
	severity     int
	lastSeverity int
	active       bool
}

type triggerStruct struct {
	m              map[string]allTriggers
	converSeverity map[int]string
}

func makeTriggerStruct() *triggerStruct {
	var cfg triggerStruct
	cfg.m = make(map[string]allTriggers)
	cfg.converSeverity = map[int]string{0: "2", 1: "2", 2: "3", 3: "3", 4: "4", 5: "4"}
	return &cfg

}

// Читаем из конфигурации все хосты и топики для публикации
func (t *triggerStruct) readConfig(conf *config.Config) {
	for host, topic := range conf.Topics.Servers {
		val := allTriggers{
			topic:        topic,
			severity:     -1,
			lastSeverity: 0,
			active:       true,
		}
		t.m[host] = val
	}
	// Если в конфигурации есть переобределения приоритета
	if len(conf.Severity) > 0 {
		maps.Copy(t.converSeverity, conf.Severity)
	}
}

func (t *triggerStruct) activeOFF() {
	for host, trigger := range t.m {
		trigger.active = false
		t.m[host] = trigger
	}
}

func (t *triggerStruct) writeSeverity(host string, severity int) {
	if val, ok := t.m[host]; ok {
		if severity > val.severity {
			val.severity = severity
		}
		val.active = true
		t.m[host] = val
	}
}

func (t *triggerStruct) publicSeverity(client mqtt.Client) {
	for host, trigger := range t.m {
		if trigger.topic == "" {
			continue
		}

		//fmt.Printf("\t%s \t(%d : %d)\t[%v]\n", trigger.topic, trigger.severity, trigger.lastSeverity, trigger.active)

		if trigger.severity == trigger.lastSeverity {
			continue
		}

		trigger.lastSeverity = trigger.severity
		t.m[host] = trigger

		pub(client, trigger.topic, t.convertPriority(trigger.severity))
	}
}

func (t *triggerStruct) convertPriority(priority int) string {
	if val, ok := t.converSeverity[priority]; ok {
		return val
	}
	return "2"
}
