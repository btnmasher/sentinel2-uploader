package evelogs

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"
	"unicode/utf16"
)

func (t *Tailer) Prime() error {
	file, err := os.Open(t.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	if t.Encoding == "" {
		header := make([]byte, 2)
		n, _ := io.ReadFull(file, header)
		if n == 2 && header[0] == 0xFF && header[1] == 0xFE {
			t.Encoding = "utf16le"
		} else if n == 2 && header[0] == 0xFE && header[1] == 0xFF {
			t.Encoding = "utf16be"
		} else {
			t.Encoding = "utf8"
		}
	}

	if t.Offset == 0 {
		t.Offset = info.Size()
	}
	return nil
}

func (t *Tailer) ReadNewLines() ([]string, error) {
	file, err := os.Open(t.Path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if info.Size() < t.Offset {
		t.Offset = 0
	}
	if _, err := file.Seek(t.Offset, io.SeekStart); err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, file); err != nil {
		return nil, err
	}
	t.Offset += int64(buf.Len())

	raw := append([]byte{}, buf.Bytes()...)
	if len(t.PendingBytes) > 0 {
		raw = append(t.PendingBytes, raw...)
		t.PendingBytes = nil
	}
	decoded := decodeLogChunk(raw, t)

	scanner := bufio.NewScanner(strings.NewReader(decoded))
	lines := []string{}
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, nil
}

func decodeLogChunk(raw []byte, t *Tailer) string {
	if len(raw) == 0 {
		return ""
	}
	switch t.Encoding {
	case "utf16le", "utf16be":
		if len(raw)%2 != 0 {
			t.PendingBytes = []byte{raw[len(raw)-1]}
			raw = raw[:len(raw)-1]
		}
		if len(raw) == 0 {
			return ""
		}

		u16 := make([]uint16, 0, len(raw)/2)
		for i := 0; i+1 < len(raw); i += 2 {
			var value uint16
			if t.Encoding == "utf16le" {
				value = binary.LittleEndian.Uint16(raw[i : i+2])
			} else {
				value = binary.BigEndian.Uint16(raw[i : i+2])
			}
			u16 = append(u16, value)
		}
		runes := utf16.Decode(u16)
		if len(runes) > 0 && runes[0] == '\ufeff' {
			runes = runes[1:]
		}
		return string(runes)
	default:
		return string(raw)
	}
}

func NormalizeLogLine(line string) string {
	line = strings.TrimPrefix(line, "\ufeff")
	return strings.TrimRight(line, "\r")
}
