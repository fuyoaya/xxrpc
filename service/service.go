package service

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

type Service struct {
	Name   string                 //映射的结构体的名称
	Typ    reflect.Type           //结构体的类型
	Rcvr   reflect.Value          //结构体的实例本身
	Method map[string]*MethodType //存储映射的结构体的所有符合条件的方法
}

// 入参是任意需要映射为服务的结构体实例
func NewService(rcvr interface{}) *Service {
	s := new(Service)
	s.Typ = reflect.TypeOf(rcvr)
	s.Rcvr = reflect.ValueOf(rcvr)
	s.Name = reflect.Indirect(s.Rcvr).Type().Name()
	if !ast.IsExported(s.Name) {
		log.Fatalf("rpc server: %s is not a valid service name", s.Name)
	}
	s.RegisterMethods()
	return s
}

func (s *Service) RegisterMethods() {
	s.Method = make(map[string]*MethodType)
	for i := 0; i < s.Typ.NumMethod(); i++ {
		method := s.Typ.Method(i)
		mType := method.Type
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.Method[method.Name] = &MethodType{
			Method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server: register %s.%s\n", s.Name, method.Name)
	}
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

// 通过反射值调用方法
func (s *Service) Call(m *MethodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.NumCalls, 1)
	f := m.Method.Func
	returnValues := f.Call([]reflect.Value{s.Rcvr, argv, replyv})
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}
