package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Константы и переменные для приложения

const remote_serverPort int = 9998
const local_port int = 7777
const hub_addr int = 777

// Соответсвие адресам их устройств с типом для каждого устройства
var arp_table map[int]interface{} = make(map[int]interface{})
var serial int = 1 // Нумерация отправленных пакетов
var URL string = fmt.Sprintf("http://localhost:%d", remote_serverPort)

// Переменные для CRC-8
var crctable = make([]byte, 256)
var generator byte = 0x1D

func main() {
	CalculateTable_CRC8()    // Таблица для вычисления контрольных сумм
	client := &http.Client{} // Создаем клиент

	var req *http.Request

	req = WHOISHERE_IAMHERE(1) // 1 значит cmd = 1 (WHOISHERE)

	// Отправляем запрос и сохраняем response
	resp, err := client.Do(req)
	serial += 1
	if err != nil {
		fmt.Println("Request error:", err)
		return
	}
	defer resp.Body.Close()

	packets := Decode_response(resp)

	for len(packets) > 0 {
		eks := Packet_resp_init(packets)

		fmt.Println(eks)
		switch eks.cmd {
		case 1:
			_, ok := arp_table[eks.src]
			if !ok {
				Save_device(eks)
			}
			req = WHOISHERE_IAMHERE(2)
			resp, err := client.Do(req)
			serial += 1
			if err != nil {
				fmt.Println("Request error:", err)
				return
			}
			defer resp.Body.Close()
			packets = append(packets, Decode_response(resp)...)

		case 2:
			fmt.Println("Need to save this packet (it's answer to WHOISHERE)")
			_, ok := arp_table[eks.src]
			if !ok {
				Save_device(eks)
			}
		case 4:
			fmt.Println("Need to save STATUS")
		case 6:
			fmt.Println("TICK need to planning events")
		}
		packets = packets[packets[0]+2:]
	}
	fmt.Println(arp_table)
	// for addr, device := range arp_table {
	// 	if device.dev_type > 1 && device.dev_type < 6 {
	// 		// Отправляем запрос
	// 		req = GET_STATUS(addr)
	// 		resp, err := client.Do(req)
	// 		serial += 1
	// 		if err != nil {
	// 			fmt.Println("Request error:", err)
	// 			return
	// 		}
	// 		defer resp.Body.Close()
	// 		// Обрабатываем пришедший STATUS
	// 		packet_bytes := Decode_response(resp)
	// 		packet := Packet_resp_init(packet_bytes)
	// 		fmt.Println(packet)
	// 		switch device.dev_type {
	// 		case 2:
	// 			// dev := EnvSensor{Device: device, }
	// 			fmt.Println("EnvSensor")
	// 		case 3:
	// 			if packet.cmd_body[0] == byte(1) {

	// 			}

	// 		}
	// 	}
	// }
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

// Распаковывает пакет (массив байтов) в переменную типа Packet_resp.
// Предварительно проверяет контрольную сумму
func Packet_resp_init(packet []byte) Packet_resp {

	// Проверка контрольной суммы
	if packet[int(packet[0])+1] != ComputeCRC8(packet[1:int(packet[0])+1]) {
		log.Fatal("CRC-8 isn't correct", packet[int(packet[0])+3], ComputeCRC8(packet[1:int(packet[0])+3]))
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
		for i = 5; packet[last_idx+i] > 127; i++ {
		}
		eks.serial = Unmarshal(packet[last_idx : last_idx+i])
		last_idx += i
	}

	// Поля dev_type, cmd и cmd_body
	eks.dev_type = packet[last_idx]
	eks.cmd = packet[last_idx+1]
	eks.cmd_body = packet[last_idx+2 : packet[0]+1]

	return eks
}

// Декодирует полученный response в массив байтов
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

// Формирует пакет, закодированный в Base64 строку
func Make_packet(dst, cmd int) string {

	// Делаем payload
	var pdu []byte
	pdu = append(pdu, Marshal(hub_addr)...)
	pdu = append(pdu, Marshal(dst)...)
	pdu = append(pdu, Marshal(serial)...)
	pdu = append(pdu, byte(1))
	pdu = append(pdu, byte(cmd))
	pdu = append(pdu, byte(len([]byte("SMARTHUB"))))
	pdu = append(pdu, []byte("SMARTHUB")...)

	// Заворачиваем все в packet
	var packet []byte
	packet = append(packet, byte(len(pdu)))
	packet = append(packet, pdu...)
	packet = append(packet, ComputeCRC8(packet[1:]))

	// Кодируем в Base64
	encoded := base64.StdEncoding.EncodeToString(packet)
	encoded_body := Base_encode(encoded)

	return encoded_body
}

// Создает POST запрос GET_STATUS
func GET_STATUS(addr int) *http.Request {
	packet := Make_packet(addr, 3)
	return Create_POST(packet)
}

// Создает POST запрос WHOISHERE или IAMHERE в зависимости от cmd
func WHOISHERE_IAMHERE(cmd int) *http.Request {
	packet := Make_packet(16383, cmd)
	return Create_POST(packet)
}

func Parse_triggers(dev_props []byte) []Trigger {
	count_sens := strings.Count(strconv.FormatInt(int64(dev_props[0]), 2), "1")
	var trigers_array []Trigger
	for i := 0; i < count_sens; i++ {
		var value []byte
		var last_idx int
		for idx, elem := range dev_props[1:] {
			if elem > 127 {
				value = append(value, elem)
			} else {
				value = append(value, elem)
				last_idx = idx
				break
			}
		}
		trig := Trigger{
			op:    dev_props[0],
			value: Unmarshal(value),
			name:  string(dev_props[last_idx+1 : int(dev_props[last_idx+1])+last_idx]),
		}
		trigers_array = append(trigers_array, trig)
	}
	return trigers_array
}

func Create_POST(body_string string) *http.Request {
	// Создаем POST запрос
	body := strings.NewReader(body_string)
	URL := fmt.Sprintf("http://localhost:%d", remote_serverPort)

	req, err := http.NewRequest(http.MethodPost, URL, body)
	if err != nil {
		log.Fatal("Error creating request:", err)
	}
	// Меняем значение заголовка Transfer-Encoding
	req.TransferEncoding = []string{"base64"}

	return req
}

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
}

// Marshal converts an int into a uleb128-encoded byte array.
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

// Unmarshal converts a uleb128-encoded byte array into an int.
func Unmarshal(r []byte) (total int) {
	var shift uint
	var len int

	for {
		b := r[len]
		len++
		total |= (int(b&0x7F) << shift)
		if b&0x80 == 0 {
			break
		}
		shift += 7
	}

	return
}

// Подготовка к декодированию
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

// Обратная Base_decode функция
func Base_encode(s string) string {
	col := utf8.RuneCountInString(s)
	if string(s[col-2:]) == "==" {
		s = strings.TrimSuffix(s, "==")
	} else if string(s[col-1]) == "=" {
		s = strings.TrimSuffix(s, "=")
	}
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "+", "-")

	return s
}

// Создает таблицу для вычисления контрольных сумм CRC-8
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
}

// Вычисляет контрольную сумму по алгоритму CRC-8
func ComputeCRC8(bytes []byte) byte {
	crc := byte(0)
	for _, b := range bytes {
		data := b ^ crc
		crc = crctable[data]
	}

	return crc
}
