package voice

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestWriteReadFrame_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{"small", []byte("hello")},
		{"empty", []byte{}},
		{"contains newline bytes", []byte{0x0A, 0x0A, 0x00, 0x0A, 0xFF}},
		{"larger than bufio.Scanner's 64KB token cap", bytes.Repeat([]byte{0x0A, 0x42}, 40000)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeFrame(&buf, tt.payload); err != nil {
				t.Fatalf("writeFrame() error = %v", err)
			}
			got, err := readFrame(bufio.NewReader(&buf))
			if err != nil {
				t.Fatalf("readFrame() error = %v", err)
			}
			if !bytes.Equal(got, tt.payload) {
				t.Errorf("readFrame() = %v (len %d), want %v (len %d)", got, len(got), tt.payload, len(tt.payload))
			}
		})
	}
}

func TestReadFrame_MultipleFramesSequentially(t *testing.T) {
	var buf bytes.Buffer
	frames := [][]byte{[]byte("first"), []byte("second\nwith\nnewlines"), []byte("third")}
	for _, f := range frames {
		if err := writeFrame(&buf, f); err != nil {
			t.Fatalf("writeFrame() error = %v", err)
		}
	}

	r := bufio.NewReader(&buf)
	for i, want := range frames {
		got, err := readFrame(r)
		if err != nil {
			t.Fatalf("readFrame() [%d] error = %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("readFrame() [%d] = %v, want %v", i, got, want)
		}
	}
}

func TestReadFrame_EOF(t *testing.T) {
	_, err := readFrame(bufio.NewReader(bytes.NewReader(nil)))
	if err != io.EOF {
		t.Fatalf("readFrame() error = %v, want io.EOF", err)
	}
}

func TestReadFrame_TruncatedFrame(t *testing.T) {
	var buf bytes.Buffer
	if err := writeFrame(&buf, []byte("hello world")); err != nil {
		t.Fatalf("writeFrame() error = %v", err)
	}
	truncated := buf.Bytes()[:6] // 4-byte header + 2 of the 11 payload bytes

	_, err := readFrame(bufio.NewReader(bytes.NewReader(truncated)))
	if err == nil {
		t.Fatal("readFrame() error = nil, want an error for a truncated frame")
	}
}

func TestReadFrame_OversizedLengthRejected(t *testing.T) {
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], maxFrameSize+1)

	_, err := readFrame(bufio.NewReader(bytes.NewReader(header[:])))
	if err == nil {
		t.Fatal("readFrame() error = nil, want an error for an oversized frame length")
	}
}
