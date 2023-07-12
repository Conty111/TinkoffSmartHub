package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"
)

const remote_serverPort int = 9998
const local_port int = 7777
const hub_addr int = 777

var arp_table map[int]string

var serial int = 1

var crctable = make([]byte, 256)
var generator byte = 0x1D

func main() {
	// Таблица для вычисления контрольных сумм
	CalculateTable_CRC8()

	// Создаем клиент
	client := &http.Client{}

	// Создаем POST запрос
	body := strings.NewReader(Make_packet(hub_addr, 16383, 1))
	URL := fmt.Sprintf("http://localhost:%d", remote_serverPort)

	req, err := http.NewRequest(http.MethodPost, URL, body)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}
	// Меняем значение заголовка Transfer-Encoding
	req.TransferEncoding = []string{"base64"}

	// Отправляем запрос и сохраняем response
	resp, err := client.Do(req)
	serial += 1
	if err != nil {
		fmt.Println("Request error:", err)
		return
	}
	defer resp.Body.Close()

	packets := Decode_response(resp)
	fmt.Println(packets)

	for len(packets) > 0 {
		eks := Packet_resp_init(packets)

		switch eks.cmd {
		case 1:
			fmt.Println("Need to send IAMHERE")
			// _, ok := arp_table[eks.src]
			// if !ok {
			// 	arp_table[eks.src] = eks.dev_type
			// }
		case 2:
			fmt.Println("Need to save this packet (it's answer to WHOISHERE)")
			// _, ok := arp_table[eks.src]
			// if !ok {
			// 	arp_table[eks.src] = eks.dev_type
			// }
		case 4:
			fmt.Println("Need to save STATUS")
		case 6:
			fmt.Println("TICK need to planning events")
		}
		fmt.Println(eks)
		packets = packets[packets[0]+2:]

	}
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

// Распаковывает пакет (массив байтов) в переменную типа Packet_resp.
// Предварительно
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
func Make_packet(src, dst, cmd int) string {

	// Делаем payload
	var pdu []byte
	pdu = append(pdu, Marshal(src)...)
	pdu = append(pdu, Marshal(dst)...)
	pdu = append(pdu, Marshal(serial)...)
	pdu = append(pdu, byte(1))
	pdu = append(pdu, byte(cmd))
	pdu = append(pdu, byte(len([]byte("SmartHub"))))
	pdu = append(pdu, []byte("SmartHub")...)

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
