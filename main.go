package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"unicode/utf8"
)

const remote_serverPort int = 9998
const local_port int = 7777

var serial int = 1

var crctable = make([]byte, 256)
var generator byte = 0x1D

func main() {
	// Таблица для вычисления контрольных сумм
	CalculateTable_CRC8()

	// Создаем клиент
	client := &http.Client{}

	// Создаем POST запрос
	body := strings.NewReader(make_packet(777, 16383, 1))
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
	// Считываем body
	resp.TransferEncoding = []string{"base64"}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	// Редактируем строку URL-base64 в правильный формат
	s := Base_decode(string(bodyBytes))
	// Декодируем URL-base64
	decoded, err := base64.StdEncoding.DecodeString(s)
	fmt.Println(resp.Body, decoded)
}

// Формирует пакет закодированный в Base64 строку
func make_packet(src, dst, cmd int) string {

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

func Crc8(data []byte) byte {
	crc := byte(0x00)

	for _, b := range data {
		crc ^= b

		for i := 0; i < 8; i++ {
			if crc&0x80 != 0 {
				crc = (crc << 1) ^ 0x31
			} else {
				crc <<= 1
			}
		}
	}

	return crc
}

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

func ComputeCRC8(bytes []byte) byte {
	crc := byte(0)
	for _, b := range bytes {
		data := b ^ crc
		crc = crctable[data]
	}

	return crc
}
