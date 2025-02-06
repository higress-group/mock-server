package chat

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

type stringWriter interface {
	io.Writer
	writeString(string) (int, error)
}

type stringWrapper struct {
	io.Writer
}

func (w stringWrapper) writeString(str string) (int, error) {
	return w.Writer.Write([]byte(str))
}

func checkWriter(writer io.Writer) stringWriter {
	if w, ok := writer.(stringWriter); ok {
		return w
	} else {
		return stringWrapper{writer}
	}
}

var contentType = []string{"text/event-stream"}
var noCache = []string{"no-cache"}

var dataReplacer = strings.NewReplacer(
	"\n", "\ndata:",
	"\r", "\\r")

type streamEvent struct {
	Event string
	Id    string
	Retry uint
	Data  interface{}
}

func encode(writer io.Writer, event streamEvent) error {
	w := checkWriter(writer)
	return writeData(w, event.Data)
}

func writeData(w stringWriter, data interface{}) error {
	dataReplacer.WriteString(w, fmt.Sprint(data))
	if strings.HasPrefix(data.(string), "data") {
		w.writeString("\n\n")
	}
	return nil
}

func (r streamEvent) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	return encode(w, r)
}

func (r streamEvent) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	header["Content-Type"] = contentType

	if _, exist := header["Cache-Control"]; !exist {
		header["Cache-Control"] = noCache
	}
}
