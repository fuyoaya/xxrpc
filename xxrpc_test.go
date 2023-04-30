package main

import (
	"reflect"
	"testing"
)

/*
	type Service struct {
		Name   string                 //映射的结构体的名称
		Typ    reflect.Type           //结构体的类型
		Rcvr   reflect.Value          //结构体的实例本身
		Method map[string]*MethodType //存储映射的结构体的所有符合条件的方法
	}

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
*/
func TestReflect(t *testing.T) {
	type people struct {
		name string
		age  int
	}

	p := &people{
		name: "zhang3",
		age:  18,
	}

	typeOfa := reflect.TypeOf(p)
	t.Log(typeOfa, "----------------------------------")

	valueOfa := reflect.ValueOf(p)
	t.Log(valueOfa, "----------------------------------")

	name := reflect.Indirect(valueOfa).Type().Name()
	t.Log(name, "----------------------------------")

}
