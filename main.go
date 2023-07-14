package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Здесь хранятся все устройства
// Адрес: переменная типа Устройство
// Example -> [2: EnvSensor, 819: Switch]
var arp_table map[int]interface{} = make(map[int]interface{})
var hub_addr int   // Адрес хаба в сети
var serial int = 1 // Нумерация отправленных пакетов

// Переменные для http запросов
var URL string
var client *http.Client

// Переменные для CRC-8
var crctable = make([]byte, 256)
var generator byte = 0x1D

// Точка входа приложения
func main() {
	// Сохраняем аргументы при запуске
	args := os.Args
	URL = args[1]
	addr, err := strconv.ParseInt(args[2], 16, 64)
	if err != nil {
		log.Fatal(err)
	}
	hub_addr = int(addr)

	CalculateTable_CRC8()
	client = &http.Client{} // Создаем клиент для http запросов

	req := WHOISHERE_IAMHERE(1)

	resp := Send_request(req)
	for resp.Status == "200 OK" {
		packets := Decode_response(resp)
		resp.Body.Close()

		// Делаем 2 тика, ждем ответы от всех устройств и сохраняем ответы
		packets = append(packets, Communicate_2ticks()...)
		req_packet := Read_packets(packets)
		packets = nil

		encoded := base64.StdEncoding.EncodeToString(req_packet)
		encoded_right := Base_encode(encoded)
		req = Create_POST(encoded_right)
		resp = Send_request(req)
	}
}

// Communicate_2ticks() отправляет 2 пустых запроса и
// возвращает ответы в виде массива байтов
func Communicate_2ticks() []byte {
	req := TICK()
	resp := Send_request(req)
	packets := Decode_response(resp)
	resp.Body.Close()
	req = TICK()
	resp = Send_request(req)
	packets = append(packets, Decode_response(resp)...)
	resp.Body.Close()
	req_packet := Read_packets(packets)
	return req_packet
}

type Packet_resp struct {
	length   byte
	src      int
	dst      int
	serial   int
	dev_type byte
	cmd      byte
	cmd_body []byte
}

type Device struct {
	addr      int
	dev_name  string
	dev_type  byte
	dev_props []byte
}

type EnvSensor struct {
	Device
	sensors  byte
	triggers []Trigger
	values   []byte
}

type Trigger struct {
	op    byte
	value int
	name  string
}

type Switch struct {
	Device
	devices []string
	status  byte
}

type Lamp struct {
	Device
	status byte
}

type Socket struct {
	Device
	status byte
}

// Packet_parse() распаковывает пакет (массив байтов) в переменную типа Packet_resp
// Предварительно проверяет контрольную сумму
func Packet_parse(packet []byte) Packet_resp {
	// Проверка контрольной суммы
	my_crc := ComputeCRC8(packet[1 : int(packet[0])+1])
	crc := packet[int(packet[0])+1]
	if crc != my_crc {
		log.Fatal("CRC-8 isn't correct", crc, my_crc)
	}

	var eks Packet_resp // Переменная экземляр пакета
	var last_idx int    // Переменная для хранения индекса, на котором остановилась распаковка

	eks.length = packet[0] // Длина payload

	// Поле src
	if packet[1] > 127 {
		eks.src = Unmarshal(packet[1:3])
		last_idx = 3
	} else {
		eks.src = int(packet[1])
		last_idx = 2
	}

	// Поле dst
	if packet[last_idx] > 127 {
		eks.dst = Unmarshal(packet[last_idx : last_idx+2])
		last_idx += 2
	} else {
		eks.dst = int(packet[last_idx])
		last_idx += 1
	}

	// Поле serial
	if packet[last_idx] < 128 {
		eks.serial = int(packet[last_idx])
		last_idx += 1
	} else {
		var i int
		for i = last_idx; packet[i] > 127; i++ {
		}
		eks.serial = Unmarshal(packet[last_idx : last_idx+i-1])
		last_idx = i + 1
	}

	// Поля dev_type, cmd и cmd_body
	eks.dev_type = packet[last_idx]
	eks.cmd = packet[last_idx+1]
	eks.cmd_body = packet[last_idx+2 : packet[0]+1]

	return eks
}

// Decode_response() декодирует полученный response в массив байтов
// Массив байтов - последовательность пакетов
func Decode_response(resp *http.Response) []byte {

	// Считываем body
	resp.TransferEncoding = []string{"base64"}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	// Редактируем строку URL-base64 в правильный формат
	s := Base_decode(string(bodyBytes))

	// Декодируем URL-base64 в массив байтов
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		log.Fatal(err)
	}
	return decoded
}

// Make_packet() формирует из переданного payload пакет
func Make_packet(pdu []byte) []byte {
	// Заворачиваем все в packet
	var packet []byte
	packet = append(packet, byte(len(pdu)))
	packet = append(packet, pdu...)
	packet = append(packet, ComputeCRC8(packet[1:]))
	serial += 1
	return packet
}

// Make_payload() формирует поле payload для пакета
func Make_payload(dst int, dev_type, cmd, cmd_body byte) []byte {
	var pdu []byte
	pdu = append(pdu, Marshal(hub_addr)...)
	pdu = append(pdu, Marshal(dst)...)
	pdu = append(pdu, Marshal(serial)...)
	pdu = append(pdu, dev_type)
	pdu = append(pdu, cmd)
	if cmd == 1 || cmd == 2 {
		pdu = append(pdu, byte(len([]byte("SMARTHUB"))))
		pdu = append(pdu, []byte("SMARTHUB")...)
	} else if cmd == 5 {
		pdu = append(pdu, byte(cmd_body))
	}
	return pdu
}

// Создает POST запрос WHOISHERE или IAMHERE в зависимости от cmd
func WHOISHERE_IAMHERE(cmd byte) *http.Request {
	packet := Make_packet(Make_payload(16383, 1, cmd, 0))

	// Кодируем в Base64
	encoded := base64.StdEncoding.EncodeToString(packet)
	encoded_body := Base_encode(encoded)

	return Create_POST(encoded_body)
}

// Пустой запрос для того, чтобы "пикать" сервер и ждать ответ
func TICK() *http.Request {
	return Create_POST(" ")
}

// Parse_triggers() преобразует dev_props
// (массив байтов) в массив тригерров типа Trigger
func Parse_triggers(dev_props []byte) []Trigger {
	count_sens := int(dev_props[1])
	dev_props = dev_props[2:]
	triggers_array := make([]Trigger, count_sens)
	for i := 0; i < count_sens; i++ {
		var value []byte
		var last_idx int
		for idx, elem := range dev_props[1:] {
			if elem > 127 {
				value = append(value, elem)
			} else {
				value = append(value, elem)
				last_idx = idx + 2
				break
			}
		}
		trig := Trigger{
			op:    dev_props[0],
			value: Unmarshal(value),
			name:  string(dev_props[last_idx+1 : int(dev_props[last_idx])+last_idx+1]),
		}
		dev_props = dev_props[int(dev_props[last_idx])+last_idx+1:]
		triggers_array[i] = trig
	}
	return triggers_array
}

// Создает POST запрос для функции Send_request()
// В body записывает переданную строку
func Create_POST(body_string string) *http.Request {
	body := strings.NewReader(body_string)
	req, err := http.NewRequest(http.MethodPost, URL, body)
	if err != nil {
		log.Fatal("Error creating request:", err)
	}
	// Меняем значение заголовка Transfer-Encoding
	req.TransferEncoding = []string{"base64"}

	return req
}

// Функция, которая отправляет запрос и возвращает ответ
func Send_request(req *http.Request) *http.Response {
	resp, err := client.Do(req)
	if resp.Status == "204 No Content" {
		os.Exit(0)
	} else if err != nil {
		os.Exit(99)
	}
	return resp
}

// Сохраняет устройство в соответствии с его типом в arp_table
// На вход принимает переменную типа Packet_resp
func Save_device(eks Packet_resp) {
	basic := Device{
		addr:      eks.src,
		dev_name:  string(eks.cmd_body[1 : eks.cmd_body[0]+1]),
		dev_type:  eks.dev_type,
		dev_props: eks.cmd_body[eks.cmd_body[0]+1:],
	}
	switch eks.dev_type {
	case 2:
		triggers_array := Parse_triggers(basic.dev_props)
		arp_table[eks.src] = EnvSensor{
			Device:   basic,
			sensors:  basic.dev_props[0],
			triggers: triggers_array,
		}
	case 3:
		var words_count int = int(basic.dev_props[0])
		var string_len int = int(basic.dev_props[1])
		var string_array []string
		for i, c := 1, 0; c < words_count; i += string_len + 1 {
			string_array = append(string_array, string(basic.dev_props[i:i+string_len+1]))
			c += 1
		}
		arp_table[eks.src] = Switch{
			Device:  basic,
			devices: string_array,
		}
	case 4:
		arp_table[eks.src] = Lamp{
			Device: basic,
		}
	case 5:
		arp_table[eks.src] = Socket{
			Device: basic,
		}
	case 6:
		arp_table[eks.src] = basic
	}
	return
}

// Проверяет наличие устройства в arp_table
// Если его нет или изменились его свойства, сохраняет его
func Check_saved(packet Packet_resp) {
	device, ok := arp_table[packet.src]
	if !ok {
		Save_device(packet)
	} else if packet.cmd == 1 {
		var cmd_body []byte
		switch device.(type) {
		case Switch:
			cmd_body = append(cmd_body, byte(len([]byte(device.(Switch).dev_name))))
			cmd_body = append(cmd_body, []byte(device.(Switch).dev_name)...)
			cmd_body = append(cmd_body, device.(Switch).dev_props...)
		case EnvSensor:
			cmd_body = append(cmd_body, byte(len([]byte(device.(EnvSensor).dev_name))))
			cmd_body = append(cmd_body, []byte(device.(EnvSensor).dev_name)...)
			cmd_body = append(cmd_body, device.(EnvSensor).dev_props...)
		case Lamp:
			cmd_body = append(cmd_body, byte(len([]byte(device.(Lamp).dev_name))))
			cmd_body = append(cmd_body, []byte(device.(Lamp).dev_name)...)
			cmd_body = append(cmd_body, device.(Lamp).dev_props...)
		case Socket:
			cmd_body = append(cmd_body, byte(len([]byte(device.(Socket).dev_name))))
			cmd_body = append(cmd_body, []byte(device.(Socket).dev_name)...)
			cmd_body = append(cmd_body, device.(Socket).dev_props...)
		}
		if !bytes.Equal(cmd_body, packet.cmd_body) {
			Save_device(packet)
		}
	}
	return
}

// Marshal конвертирует тип int в uleb128 как массив байтов
func Marshal(i int) (r []byte) {
	var len int
	if i == 0 {
		r = []byte{0}
		return
	}

	for i > 0 {
		r = append(r, 0)
		r[len] = byte(i & 0x7F)
		i >>= 7
		if i != 0 {
			r[len] |= 0x80
		}
		len++
	}
	return
}

// Unmarshal конвертирует uleb128 массив байтов в число типа int
func Unmarshal(buf []byte) (total int) {
	var i int
	var shift uint

	for {
		b := buf[0]
		buf = buf[1:]
		i |= int(b&0x7F) << shift
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}

	return i
}

// Base_decode() добавляет отступы к base64 строке
func Base_decode(s string) string {
	if utf8.RuneCountInString(s)%4 == 2 {
		s = fmt.Sprintf("%s==", s)
	} else if utf8.RuneCountInString(s)%4 == 3 {
		s = fmt.Sprintf("%s=", s)
	}
	s = strings.ReplaceAll(s, "_", "/")
	s = strings.ReplaceAll(s, "-", "+")

	return s
}

// Base_encode() убирает все отступы в base64 строке
func Base_encode(s string) string {
	col := utf8.RuneCountInString(s)
	if col > 0 {
		if string(s[col-2:]) == "==" {
			s = strings.TrimSuffix(s, "==")
		} else if string(s[col-1]) == "=" {
			s = strings.TrimSuffix(s, "=")
		}
		s = strings.ReplaceAll(s, "/", "_")
		s = strings.ReplaceAll(s, "+", "-")
		return s
	} else {
		return ""
	}

}

// Read_packets() обрабатывает все пришедшие пакеты
// Возвращает массив байтов с пакетами для отправки
func Read_packets(packets []byte) []byte {
	var req_packet []byte
	for len(packets) > 0 {
		eks := Packet_parse(packets)
		switch eks.cmd {
		case 1:
			Check_saved(eks)
			req := WHOISHERE_IAMHERE(2)
			resp := Send_request(req)
			packets = append(packets, Decode_response(resp)...)
			resp.Body.Close()
		case 2:
			Check_saved(eks)
			if eks.dev_type != 6 {
				get_stat_packet := Make_packet(Make_payload(eks.src, eks.dev_type, 3, 0))
				req_packet = append(req_packet, get_stat_packet...)
			}
		case 4:
			switch eks.dev_type {
			case 2:
				elem := arp_table[eks.src].(EnvSensor)
				elem.values = eks.cmd_body
				arp_table[eks.src] = elem
				packets := Check_sensors(elem)
				req_packet = append(req_packet, packets...)
			case 3:
				elem := arp_table[eks.src].(Switch)
				elem.status = eks.cmd_body[0]
				arp_table[eks.src] = elem
				req_packet = append(req_packet, Set_switch_device_status(elem)...)
			case 4:
				elem := arp_table[eks.src].(Lamp)
				elem.status = byte(eks.cmd_body[0])
				arp_table[eks.src] = elem
			case 5:
				elem := arp_table[eks.src].(Socket)
				elem.status = byte(eks.cmd_body[0])
				arp_table[eks.src] = elem
			}
		}
		packets = packets[packets[0]+2:]
	}
	return req_packet
}

// Check_sensor() проверяет сенсор, проверяя каждый триггер
// Если значения триггера превышают норму, создает пакет типа SET_STATUS
// с src и dev_type устройства, указанного в триггере.
// Возвращает пачку пакетов для запроса
// Если устройство триггера не найдено, пакет для него не создается
func Check_sensors(elem EnvSensor) []byte {
	var array_values []int
	var tmp_arr []byte
	for _, value := range elem.values {
		if value > 128 {
			tmp_arr = append(tmp_arr, value)
		} else {
			tmp_arr = append(tmp_arr, value)
			array_values = append(array_values, Unmarshal(tmp_arr))
			tmp_arr = nil
		}
	}
	var packets []byte
	for idx, trigger := range elem.triggers {
		op := fmt.Sprintf("%04s", strconv.FormatInt(int64(trigger.op), 2))
		if op[2] == 49 {
			// Сравнивать по условию больше
			if trigger.value > array_values[idx] {
				packets = append(packets, Find_related_dev(trigger)...)
			}
		} else {
			// Сравнивать по условию меньше
			if trigger.value < array_values[idx] {
				packets = append(packets, Find_related_dev(trigger)...)
			}
		}
	}
	return packets
}

// Set_switch_device_status() формирует пачку пакетов
// для установления всем устройствам, связанным с Switch
// аналогичного статуса (0 или 1)
func Set_switch_device_status(elem Switch) []byte {
	var req_packet []byte
	for _, dev_name := range elem.devices {
		for _, device := range arp_table {
			switch device.(type) {
			case Lamp:
				if device.(Lamp).dev_name == dev_name {
					packet := Make_packet(Make_payload(device.(Lamp).addr, 4, 5, elem.status))
					req_packet = append(req_packet, packet...)
				}
			case Socket:
				if device.(Socket).dev_name == dev_name {
					packet := Make_packet(Make_payload(device.(Lamp).addr, 4, 5, elem.status))
					req_packet = append(req_packet, packet...)
				}
			}
		}
	}
	return req_packet
}

// Find_related_dev() находит связанное с триггером
// устройство в arp_table и возвращает пакет SET_STATUS
// с нужными параметрами
// Вызывается если значения триггера больше или меньше нормы
func Find_related_dev(trigger Trigger) []byte {
	var packet []byte
	var src int
	var dev_type byte
	OnOrOff := trigger.op % 2
	for i, dev := range arp_table {
		switch dev.(type) {
		case Lamp:
			if dev.(Lamp).dev_name == trigger.name {
				src = i
				dev_type = 4
				packet = Make_packet(Make_payload(src, dev_type, 5, OnOrOff))
			}
		case Socket:
			if dev.(Socket).dev_name == trigger.name {
				src = i
				dev_type = 4
				packet = Make_packet(Make_payload(src, dev_type, 5, OnOrOff))
			}
		}
	}
	return packet
}

// CalculateTable_CRC8() создает таблицу для вычисления контрольных сумм CRC-8
func CalculateTable_CRC8() {
	for dividend := 0; dividend < 256; dividend++ {
		currByte := byte(dividend)

		for bit := 0; bit < 8; bit++ {
			if (currByte & 0x80) != 0 {
				currByte <<= 1
				currByte ^= generator
			} else {
				currByte <<= 1
			}
		}
		crctable[dividend] = currByte
	}
	return
}

// ComputeCRC8() вычисляет контрольную сумму по алгоритму CRC-8
func ComputeCRC8(bytes []byte) byte {
	crc := byte(0)
	for _, b := range bytes {
		data := b ^ crc
		crc = crctable[data]
	}

	return crc
}
