package xxcode

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

var _ Code = (*GobCode)(nil)

type GobCode struct {
	conn io.ReadWriteCloser //由构建函数传入，通常是通过 TCP 或者 Unix 建立 socket 时得到的链接实例
	buf  *bufio.Writer      //防止阻塞而创建的带缓冲的 Writer，一般这么做能提升性能。
	dec  *gob.Decoder       // decoder
	enc  *gob.Encoder       // encoder
}

func NewGobCode(conn io.ReadWriteCloser) Code {
	return &GobCode{
		conn: conn,
		buf:  bufio.NewWriter(conn),
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(conn),
	}
}

func (c *GobCode) Close() error {
	return c.conn.Close()
}

func (c *GobCode) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *GobCode) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *GobCode) Write(h *Header, body interface{}) (err error) {
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()
	if err = c.enc.Encode(h); err != nil {
		log.Println("rpc: gob error encoding header:", err)
		return
	}
	if err = c.enc.Encode(body); err != nil {
		log.Println("rpc: gob error encoding body:", err)
		return
	}
	return
}
