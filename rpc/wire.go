package rpc

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/RomiChan/protobuf/proto"
)

// maxFrameSize bounds a single wire frame to protect against malicious
// or corrupted length prefixes.
const maxFrameSize = 16 << 20

// frame is the wire envelope. A frame with a non-empty Method is a
// request (ID present) or a notification (ID absent); a frame with an
// empty Method is a response.
type frame struct {
	ID     proto.Option[int64] `protobuf:"varint,1,opt"`
	Method string              `protobuf:"bytes,2,opt"`
	Params proto.RawMessage    `protobuf:"bytes,3,opt"`
	Result proto.RawMessage    `protobuf:"bytes,4,opt"`
	Error  *Error              `protobuf:"bytes,5,opt"`
}

// writeFrame marshals f and writes it with a 4-byte big-endian length
// prefix.
func writeFrame(w io.Writer, f *frame) error {
	payload, err := proto.Marshal(f)
	if err != nil {
		return fmt.Errorf("rpc: marshal frame: %w", err)
	}
	buf := make([]byte, 4, 4+len(payload))
	binary.BigEndian.PutUint32(buf, uint32(len(payload)))
	buf = append(buf, payload...)
	n, err := w.Write(buf)
	if err == nil && n != len(buf) {
		err = io.ErrShortWrite
	}
	return err
}

// readFrame reads one length-prefixed frame from r.
func readFrame(r io.Reader) (*frame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxFrameSize {
		return nil, fmt.Errorf("rpc: frame size %d exceeds limit %d", n, maxFrameSize)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	f := &frame{}
	if err := proto.Unmarshal(buf, f); err != nil {
		return nil, fmt.Errorf("rpc: unmarshal frame: %w", err)
	}
	return f, nil
}
