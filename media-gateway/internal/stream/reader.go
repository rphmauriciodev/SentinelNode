package stream

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
)

type FrameReader struct {
	br       *bufio.Reader
	boundary string
}

func NewFrameReader(r io.Reader, boundary string) *FrameReader {
	boundary = strings.TrimSpace(boundary)
	boundary = strings.TrimPrefix(boundary, "--")
	boundary = strings.Trim(boundary, "\"")
	return &FrameReader{
		br:       bufio.NewReaderSize(r, 64*1024),
		boundary: boundary,
	}
}

func (fr *FrameReader) NextFrame() ([]byte, error) {
	tp := textproto.NewReader(fr.br)

	for {
		line, err := tp.ReadLine()
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "--") {
			break
		}

		if strings.HasPrefix(strings.ToLower(trimmed), "content-type:") ||
			strings.HasPrefix(strings.ToLower(trimmed), "content-length:") {

			return fr.readFrameWithInitialHeader(trimmed, tp)
		}
	}

	hdr, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}

	contentLengthStr := hdr.Get("Content-Length")
	if contentLengthStr != "" {
		length, err := strconv.Atoi(strings.TrimSpace(contentLengthStr))
		if err == nil && length > 0 {
			frame := make([]byte, length)
			if _, err := io.ReadFull(fr.br, frame); err != nil {
				return nil, err
			}
			return sanitizeJPEG(frame), nil
		}
	}

	return fr.scanUntilEOI()
}

func (fr *FrameReader) readFrameWithInitialHeader(initialHeader string, tp *textproto.Reader) ([]byte, error) {
	headers := make(map[string]string)
	parts := strings.SplitN(initialHeader, ":", 2)
	if len(parts) == 2 {
		headers[strings.ToLower(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
	}

	for {
		line, err := tp.ReadLine()
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 {
			headers[strings.ToLower(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
		}
	}

	if clStr, exists := headers["content-length"]; exists {
		length, err := strconv.Atoi(strings.TrimSpace(clStr))
		if err == nil && length > 0 {
			frame := make([]byte, length)
			if _, err := io.ReadFull(fr.br, frame); err != nil {
				return nil, err
			}
			return sanitizeJPEG(frame), nil
		}
	}

	return fr.scanUntilEOI()
}

func (fr *FrameReader) scanUntilEOI() ([]byte, error) {
	var buf bytes.Buffer
	var prevByte byte
	soiFound := false

	for {
		b, err := fr.br.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) && buf.Len() > 0 && soiFound {
				return sanitizeJPEG(buf.Bytes()), nil
			}
			return nil, err
		}

		if !soiFound {
			if prevByte == 0xFF && b == 0xD8 {
				soiFound = true
				buf.WriteByte(0xFF)
				buf.WriteByte(0xD8)
			}
			prevByte = b
			continue
		}

		buf.WriteByte(b)
		if prevByte == 0xFF && b == 0xD9 {

			return buf.Bytes(), nil
		}
		prevByte = b

		if buf.Len() > 5*1024*1024 {
			return nil, fmt.Errorf("frame size exceeded maximum limit (5MB)")
		}
	}
}

func sanitizeJPEG(data []byte) []byte {
	soi := bytes.Index(data, []byte{0xFF, 0xD8})
	if soi > 0 {
		return data[soi:]
	}
	return data
}
