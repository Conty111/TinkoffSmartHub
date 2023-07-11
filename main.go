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

func main() {
	// Создаем клиент
	client := &http.Client{}
	// Тело запроса
	msg := ""
	encoded := base64.StdEncoding.EncodeToString([]byte(msg))
	body := strings.NewReader(encoded)

	// Создаем POST запрос
	URL := fmt.Sprintf("http://localhost:%d", remote_serverPort)
	req, err := http.NewRequest(http.MethodPost, URL, body)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}
	// Меняем значение заголовка Transfer-Encoding
	req.TransferEncoding = []string{"base64"}

	// Отправляем запрос
	resp, err := client.Do(req)
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
	// Создаем экземпляр пакета
	packet := net_package{length: decoded[0], payload: decoded[1 : decoded[0]+1], src8: decoded[decoded[0]+1]}

	bp := packet.payload_refact()

	src, _ := strconv.ParseInt(bp[0:14], 2, 64)
	dst, _ := strconv.ParseInt(bp[14:28], 2, 64)
	// other, _ := strconv.ParseInt(bp[28:], 2, 64)

	res := payload_decoded{
		src: Marshal(int(src)),
		dst: Marshal(int(dst)),
	}
	fmt.Println(res.src, res.dst, dst)

}

type net_package struct {
	length  byte
	payload []byte
	src8    byte
}

func (obj *net_package) payload_refact() string {
	var res string
	fmt.Println(Marshal(819))
	for _, elem := range obj.payload {
		num, _ := strconv.ParseInt(fmt.Sprintf("%d", elem), 8, 64)
		c := strconv.FormatInt(num, 2)
		res = fmt.Sprintf("%s%08s", res, c)
	}
	return res
}

type payload_decoded struct {
	src      []byte
	dst      []byte
	serial   []byte
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
func Unmarshal(r []byte) (total int, len int) {
	var shift uint

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
