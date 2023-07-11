package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

const remote_serverPort int = 9998
const local_port int = 7777

var serial int = 1

var crctable = make([]byte, 256)
var generator byte = 0x1D

func main() {
	// Создаем клиент
	client := &http.Client{}

	// Тело запроса
	var pdu []byte
	pdu = append(pdu, Marshal(777)...)
	pdu = append(pdu, Marshal(16383)...)
	pdu = append(pdu, Marshal(serial)...)
	pdu = append(pdu, byte(1))
	pdu = append(pdu, byte(1))
	// pdu = append(pdu, []byte{136, 208, 171, 250, 147, 49}...)
	pdu = append(pdu, []byte("SmartHub")...)
	var new_pdu []string
	for _, elem := range pdu {
		c := strconv.FormatInt(int64(elem), 16)
		new_pdu = append(new_pdu, fmt.Sprintf("%02s", c))
	}
	var send []byte
	send = append(send, byte(len(pdu)))
	send = append(send, pdu...)
	CalculateTable_CRC8()
	send = append(send, ComputeCRC8(send[1:]))
	fmt.Println(send)

	// p := &Net_package{Length: 15, Payload: []byte{137, 6, 255, 127, 1, 1, 1},
	// 	Src8: 5}
	// packet, err := json.Marshal(p)
	// fmt.Println(base64.StdEncoding.EncodeToString(packet))

	encoded := base64.StdEncoding.EncodeToString(send)
	encoded_body := Base_encode(encoded)
	fmt.Println(encoded_body, encoded)

	// encoded_body := Base_encode("DbMG_38BBgaI0Kv6kzGK")
	body := strings.NewReader(encoded_body)

	// Создаем POST запрос
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
	// s := Base_decode(string(bodyBytes))
	s := Base_decode("DbMG_38BBgaI0Kv6kzGK")
	// Декодируем URL-base64
	decoded, err := base64.StdEncoding.DecodeString(s)
	// Создаем экземпляр пакета
	// packet := net_package{length: decoded[0], payload: decoded[1 : decoded[0]+1], src8: decoded[decoded[0]+1]}
	var new_pdu1 []string
	for _, elem := range decoded {
		c := strconv.FormatInt(int64(elem), 16)
		new_pdu1 = append(new_pdu1, fmt.Sprintf("%02s", c))
	}
	fmt.Println(new_pdu1)
	// res := payload_decoded{
	// 	src: decoded[1:3],
	// 	dst: decoded[3:5],
	// }
	fmt.Println(bodyBytes)
}

type Net_package struct {
	Length  byte   `json:"length"`
	Payload []byte `json:"payload"`
	Src8    byte   `json:"src8"`
}

func (obj *Net_package) payload_refact() string {
	var res string
	fmt.Println(Marshal(819))
	for _, elem := range obj.Payload {
		res = fmt.Sprintf("%s%08b", res, elem)
	}
	return res
}

type payload_decoded struct {
	src      []byte
	dst      []byte
	serial   int
	dev_type byte
	cmd      byte
	cmd_body []byte
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
