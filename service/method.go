package service

import (
	"reflect"
	"sync/atomic"
)

type MethodType struct {
	Method    reflect.Method //方法本身
	ArgType   reflect.Type   //第一个参数的类型
	ReplyType reflect.Type   //第二个参数的类型
	NumCalls  uint64         //统计方法调用次数s
}

func (m *MethodType) NumCall() uint64 {
	return atomic.LoadUint64(&m.NumCalls)
}

// TODO:
func (m *MethodType) NewArgv() reflect.Value {
	var argv reflect.Value
	// arg may be a pointer type, or a value type
	if m.ArgType.Kind() == reflect.Ptr {
		argv = reflect.New(m.ArgType.Elem())
	} else {
		argv = reflect.New(m.ArgType).Elem()
	}
	return argv
}

func (m *MethodType) NewReplyv() reflect.Value {
	// reply must be a pointer type
	replyv := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyv
}
