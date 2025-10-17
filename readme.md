[![latest](https://img.shields.io/github/v/release/SmithLEDs/smith-zabbix-mqtt.svg?color=brightgreen)](https://github.com/SmithLEDs/smith-zabbix-mqtt/releases/latest)
[![Foo](https://img.shields.io/badge/Telegram-2CA5E0?style=social&logo=telegram&color=blue)](https://t.me/SmithLEDs)

<h1 align="left">
  <br>
  <img height="150" src="logo.png">
  <br>
  <b>Конвертер zabbix-trigger в MQTT</b>
  <br>
</h1>


Данный микро сервис предназначен для получения активных триггеров с сервера Zabbix и пересылки в топики MQTT. 

## 1. Принцип работы

1. Сервис подключается к серверу Zabbix по API, используя протокол JSON-RPC 2.0.
    - В коде используется библиотека [`fabiang/go-zabbix v1.1.0`](https://github.com/fabiang/go-zabbix)
    - Используется метод `trigger.get` [(Описание метода)](https://www.zabbix.com/documentation/current/en/manual/api/reference/trigger/get)
    - Авторизация происходит по токену [(Инструкция получения токена в WEB интерфейсе)](https://www.zabbix.com/documentation/current/en/manual/web_interface/frontend_sections/users/api_tokens)

2. После получения активных триггеров идет пересылка в брокер MQTT.
    - В коде используется библиотека [`eclipse/paho.mqtt.golang v1.5.1`](https://github.com/eclipse-paho/paho.mqtt.golang)
    - Сервис сканирует все триггеры и в каждом анализирует, к каким узлам сети принадлежит данный триггер. Если данный узел сети присутствует в конфигурационном файле, то анализирует приоритеты и публикует самый высокий приоритет в брокер MQTT.

3. Файл конфигурации по умолчанию расположен в каталоге `/etc/smith-zabbix-mqtt/config.yaml`

## 2. Параметры командной строки

| Параметр     | Тип      | Значение по умолчанию                | Описание                        |
| ----------   | -------- | ------------------------------------ | ------------------------------- |
| `--debug`    | `bool`   | `false`                              | Включение отладочных сообщений  |
| `configFile` | `string` | `/etc/smith-zabbix-mqtt/config.yaml` | Расположение файла конфигурации |


## 3. Пример конфигурационного файла

```yaml
env: local

# Переодичность чтения триггеров Zabbix
update_interval: 2s

# Настройки для подключения к MQTT брокеру
mqtt:
    address: tcp://localhost:1883
    client_id: smith-zabbix-mqtt

    # Если false, то логин и пароль игнорируются
    authorization: false
    login: 
    password: 

# Настройки для подключения к API Zabbix
zabbix:
    address: http://localhost:8080/api_jsonrpc.php
    token: 

    # Если указать группу, то будут запрашиваться только те триггеры,
    # в которых хосты принадлежат данной группе
    group: 

# Переопределение приоритетов Zabbix в приоритеты сервера подсветки "Хамелеон"
#
# Приритеты триггеров в Zabbix:
#   0: Не классифицировано
#   1: Информация
#   2: Предупреждение
#   3: Средняя
#   4: Высокая
#   5: Чрезвычайная
# Приоритеты сервера "Хамелеон":
#   2: Все хорошо (Синий цвет)
#   3: Внимание (Желтый цвет)
#   4: Авария (Красный цвет)
severity:
    0: 2
    1: 2
    2: 3
    3: 3
    4: 4
    5: 4

# Перечисление хостов Zabbix, за которыми нужно следить (Имя узла сети)
hosts:
    - Zabbix server # Сервер Zabbix 

# Настройки создания виртуального устройства в MQTT WirenBoard
# Создается топик "/device/имя_устройства/controls/перечисления_хостов"
mqtt_virtual_device: 

    # Имя используется для формирования топика устройства
    name: statusServers

    # Публиковать или нет общее количество активных триггеров Zabbix
    total_triggers: true

    # Публиковать или нет общий аптайм работы сервиса
    uptime: true

```