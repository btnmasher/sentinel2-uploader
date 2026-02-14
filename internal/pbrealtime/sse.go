package pbrealtime

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

func readSSEEvents(reader io.Reader, out chan<- Event, errs chan<- error) {
	defer close(out)

	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)

	name := ""
	var data bytes.Buffer
	emit := func() {
		if name == "" && data.Len() == 0 {
			return
		}
		out <- Event{Name: strings.TrimSpace(name), Data: append([]byte{}, data.Bytes()...)}
		name = ""
		data.Reset()
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			emit()
			continue
		}
		if rest, ok := strings.CutPrefix(line, "event:"); ok {
			name = strings.TrimSpace(rest)
			continue
		}
		if segment, ok := strings.CutPrefix(line, "data:"); ok {
			if len(segment) > 0 && segment[0] == ' ' {
				segment = segment[1:]
			}
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(segment)
		}
	}

	emit()
	if scanErr := scanner.Err(); scanErr != nil {
		errs <- scanErr
		return
	}
	errs <- io.EOF
}
