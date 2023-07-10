package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

const serverPort int = 9998

type MyResponseWriter struct {
	writer http.ResponseWriter
}

func (w *MyResponseWriter) Header() http.Header {
	return w.writer.Header()
}

func (w *MyResponseWriter) WriteHeader(statusCode int) {
	w.writer.WriteHeader(statusCode)
}

func (w *MyResponseWriter) Write(data []byte) (int, error) {
	// Установка нужных заголовков перед записью данных
	w.Header().Set("Transfer-Encoding", "base64")
	return w.writer.Write(data)
}

func main() {
	msg := ""
	encoded := base64.StdEncoding.EncodeToString([]byte(msg))
	body := strings.NewReader(encoded)
	requestURL := fmt.Sprintf("http://localhost:%d", serverPort)
	resp, err := http.Post(requestURL, "application/x-www-form-urlencoded", body)

	// http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

	// })
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	fmt.Println(resp)
	// pay, err :=
	decoded, err := base64.StdEncoding.DecodeString(string(bodyBytes))

	fmt.Println(decoded)
	fmt.Printf("%b\n%b\n%b", decoded[0], decoded[1], decoded[2])

}

type payload struct {
	encoded_payload []byte
}

func (obj *payload) payload_decode() {

}

type payload_decoded struct {
	src      int
	dst      int
	serial   int
	dev_type byte
	cmd      byte
	cmd_body byte
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
