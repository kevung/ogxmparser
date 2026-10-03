package ogxmparser

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"
)

// cursor reads the primitives of spec section 3 from a byte slice. Every read
// checks the bound: a field that runs past its record is a hard error (3.2).
type cursor struct {
	b   []byte
	off int
}

func (c *cursor) remaining() int { return len(c.b) - c.off }

func (c *cursor) errorf(format string, args ...any) error {
	return &FormatError{Offset: c.off, Msg: fmt.Sprintf(format, args...)}
}

func (c *cursor) bytes(n int) ([]byte, error) {
	if n < 0 || n > c.remaining() {
		return nil, c.errorf("need %d bytes, %d left", n, c.remaining())
	}
	v := c.b[c.off : c.off+n]
	c.off += n
	return v, nil
}

func (c *cursor) u8() (byte, error) {
	v, err := c.bytes(1)
	if err != nil {
		return 0, err
	}
	return v[0], nil
}

func (c *cursor) u16() (uint16, error) {
	v, err := c.bytes(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(v), nil
}

func (c *cursor) u32() (uint32, error) {
	v, err := c.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(v), nil
}

func (c *cursor) u64() (uint64, error) {
	v, err := c.bytes(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(v), nil
}

// varint reads an unsigned LEB128 of at most width bits (V1). An overlong
// encoding or a value past the width is a hard error.
func (c *cursor) varint(width uint) (uint64, error) {
	start := c.off
	maxBytes := int((width + 6) / 7)
	var v uint64
	for i := 0; ; i++ {
		if i >= maxBytes {
			c.off = start
			return 0, c.errorf("varint longer than %d bytes", maxBytes)
		}
		b, err := c.u8()
		if err != nil {
			return 0, err
		}
		shift := uint(7 * i)
		v |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			if i > 0 && b == 0 {
				c.off = start
				return 0, c.errorf("overlong varint")
			}
			if width < 64 && v>>width != 0 {
				c.off = start
				return 0, c.errorf("varint exceeds %d bits", width)
			}
			if width == 64 && i == 9 && b > 1 {
				c.off = start
				return 0, c.errorf("varint exceeds 64 bits")
			}
			return v, nil
		}
	}
}

func (c *cursor) v32() (uint64, error) { return c.varint(32) }

func (c *cursor) int32le() (int32, error) {
	v, err := c.u32()
	return int32(v), err
}

func (c *cursor) str() (string, error) {
	n, err := c.v32()
	if err != nil {
		return "", err
	}
	if n > MaxString {
		return "", c.errorf("string of %d bytes exceeds MAX_STRING", n)
	}
	b, err := c.bytes(int(n))
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", c.errorf("invalid UTF-8 in string")
	}
	return string(b), nil
}

// blob reads a varint-length-prefixed byte string (signatures, keys).
func (c *cursor) blob() ([]byte, error) {
	n, err := c.v32()
	if err != nil {
		return nil, err
	}
	return c.bytes(int(n))
}

// record is one length-prefixed record (3.2): its presence mask and a cursor
// bounded by its length. Fields of unknown bits are skipped by finish.
type record struct {
	mask uint64
	c    cursor
}

func (c *cursor) record() (*record, error) {
	n, err := c.v32()
	if err != nil {
		return nil, err
	}
	body, err := c.bytes(int(n))
	if err != nil {
		return nil, err
	}
	r := &record{c: cursor{b: body}}
	if r.mask, err = r.c.varint(64); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *record) has(bit uint) bool { return r.mask&(1<<bit) != 0 }

// finish enforces the exact walk of 3.2: when every set bit is known
// (below known), the fields end exactly at the record end. Unknown trailing
// fields are tolerated and skipped (I7).
func (r *record) finish(known uint) error {
	if r.mask>>known != 0 {
		return nil
	}
	if r.c.remaining() != 0 {
		return r.c.errorf("%d trailing bytes in record", r.c.remaining())
	}
	return nil
}
