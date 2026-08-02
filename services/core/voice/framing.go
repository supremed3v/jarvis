// framing.go implements the length-prefixed binary framing services/core/voice
// uses to send/receive raw audio (and other binary payloads) over a
// subprocess's stdin/stdout pipes. A line-oriented reader like bufio.Scanner
// is unsafe here: PCM audio routinely contains the 0x0A byte as ordinary
// sample data, and Scanner also caps a single token at ~64KB. Each frame
// instead carries an explicit 4-byte little-endian length prefix followed by
// that many payload bytes.
package voice

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// maxFrameSize bounds a single frame's declared length, guarding against a
// corrupt length prefix causing an unbounded allocation.
const maxFrameSize = 64 * 1024 * 1024 // 64MB - generous for a single audio chunk

// writeFrame writes payload to w as one length-prefixed frame.
func writeFrame(w io.Writer, payload []byte) error {
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

// readFrame reads one length-prefixed frame from r. It returns io.EOF if r
// is exhausted before any bytes of a new frame are read (a clean stream
// end, e.g. the writing subprocess exited); any other error indicates a
// truncated or malformed frame.
func readFrame(r *bufio.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}

	length := binary.LittleEndian.Uint32(header[:])
	if length > maxFrameSize {
		return nil, fmt.Errorf("voice: frame length %d exceeds max %d", length, maxFrameSize)
	}
	if length == 0 {
		return nil, nil
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
