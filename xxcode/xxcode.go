package xxcode

import (
	"io"
)

type Header struct {
	ServiceMethod string // 服务名和方法名
	SeqId         uint64 // 请求序列号
	Error         string
}

type Code interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

// TODO:

type NewCodeFunc func(closer io.ReadWriteCloser) Code

type Type string

const (
	Type_Gob  Type = "application/gob"
	Type_Json Type = "application/json" // not implemented
)

var NewCodeFuncMap map[Type]NewCodeFunc

func init() {
	NewCodeFuncMap = make(map[Type]NewCodeFunc)
	NewCodeFuncMap[Type_Gob] = NewGobCode
}
