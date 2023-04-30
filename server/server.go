package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"

	"xxrpc/common"
	"xxrpc/service"
	"xxrpc/xxcode"
)

// Server represents an RPC Server.
type Server struct {
	serviceMap sync.Map
}

// NewServer returns a new Server.
func NewServer() *Server {
	return &Server{}
}

// DefaultServer is the default instance of *Server.
var DefaultServer = NewServer()

// ServeConn 在单一链接上运行服务，并阻塞直到客户端断开链接
// ServeConn blocks, serving the connection until the client hangs up.
func (s *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() {
		_ = conn.Close()
	}()

	// json.NewDecoder 反序列化得到 Option 实例，检查MagicNumber和CodeType
	var opt common.Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: options error: ", err)
		return
	}
	if opt.MagicNumber != common.MagicNumber {
		log.Printf("rpc server: invalid magic number %x", opt.MagicNumber)
		return
	}
	f := xxcode.NewCodeFuncMap[opt.CodeType]
	if f == nil {
		log.Printf("rpc server: invalid codec type %s", opt.CodeType)
		return
	}
	s.serveCode(f(conn), &opt)
}

// invalidRequest 是发生错误时响应 argv 的占位符
var invalidRequest = struct{}{}

func (s *Server) serveCode(cc xxcode.Code, opt *common.Option) {
	sending := new(sync.Mutex) // 确保发送完整的回复
	wg := new(sync.WaitGroup)  // 等到所有请求都被处理
	for {
		// 读取请求
		req, err := s.readRequest(cc)
		if err != nil {
			if req == nil {
				break // 无法恢复，所以关闭连接
			}
			req.head.Error = err.Error()
			s.sendResponse(cc, req.head, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		go s.handleRequest(cc, req, sending, wg, opt.HandleTimeout)
	}

	wg.Wait()
	_ = cc.Close()
}

// request stores all information of a call
type request struct {
	head         *xxcode.Header // 请求头
	argv, replyv reflect.Value  // argv and replyv of request
	mtype        *service.MethodType
	svc          *service.Service
}

func (s *Server) readRequestHeader(cc xxcode.Code) (*xxcode.Header, error) {
	var h xxcode.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

func (s *Server) readRequest(cc xxcode.Code) (*request, error) {
	h, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{head: h}
	req.svc, req.mtype, err = s.findService(h.ServiceMethod)
	if err != nil {
		return req, err
	}

	// 通过 newArgv() 和 newReplyv() 两个方法创建出两个入参实例，
	//然后通过 cc.ReadBody() 将请求报文反序列化为第一个入参 argv
	req.argv = req.mtype.NewArgv()
	req.replyv = req.mtype.NewReplyv()

	// make sure that argvi is a pointer, ReadBody need a pointer as parameter
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	if err = cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read body err:", err)
		return req, err
	}
	return req, nil
}

func (s *Server) sendResponse(cc xxcode.Code, h *xxcode.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

// 通过 req.svc.call 完成方法调用，将 replyv 传递给 sendResponse 完成序列化即可。
func (s *Server) handleRequest(cc xxcode.Code, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	called := make(chan struct{})
	sent := make(chan struct{})
	go func() {
		err := req.svc.Call(req.mtype, req.argv, req.replyv)
		called <- struct{}{}
		if err != nil {
			req.head.Error = err.Error()
			s.sendResponse(cc, req.head, invalidRequest, sending)
			sent <- struct{}{}
			return
		}
		s.sendResponse(cc, req.head, req.replyv.Interface(), sending)
		sent <- struct{}{}
	}()

	if timeout == 0 {
		<-called
		<-sent
		return
	}
	select {
	case <-time.After(timeout):
		req.head.Error = fmt.Sprintf("rpc server: request handle timeout: expect within %s", timeout)
		s.sendResponse(cc, req.head, invalidRequest, sending)
	case <-called:
		<-sent
	}
}

// 通过 ServiceMethod 从 serviceMap 中找到对应的 service
// ServiceMethod 的构成是 “Service.Method”，
// 因此先将其分割成 2 部分，第一部分是 Service 的名称，第二部分即方法名。
// 先从 serviceMap 中找到对应的 service 实例，再从 service 实例的 method 中，找到对应的 methodType。
func (s *Server) findService(serviceMethod string) (svc *service.Service, mtype *service.MethodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svci, ok := s.serviceMap.Load(serviceName)
	fmt.Printf("%s---------%s\n", serviceName, methodName)
	fmt.Printf("%T------------------\n", svci)
	if !ok {
		err = errors.New("rpc server: can't find service " + serviceName)
		return
	}
	svc = svci.(*service.Service)
	mtype = svc.Method[methodName]
	if mtype == nil {
		err = errors.New("rpc server: can't find method " + methodName)
	}
	return
}

// accept 接受监听net.listener上的链接，并为每一个链接启动一个服务
func (s *Server) accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		go s.ServeConn(conn)
	}
}

func Accept(lis net.Listener) {
	DefaultServer.accept(lis) // DefaultServer 是一个默认的 Server 实例，主要为了用户使用方便。
}

// Register publishes in the server the set of methods of the
// receiver value that satisfy the following conditions:
//   - exported method of exported type
//   - two arguments, both of exported type
//   - the second argument is a pointer
//   - one return value, of type error
func (s *Server) Register(rcvr interface{}) error {
	svc := service.NewService(rcvr)
	if _, dup := s.serviceMap.LoadOrStore(svc.Name, svc); dup {
		return errors.New("rpc: service already defined: " + svc.Name)
	}
	return nil
}

// Register publishes the receiver's methods in the DefaultServer.
func Register(rcvr interface{}) error {
	return DefaultServer.Register(rcvr)
}
