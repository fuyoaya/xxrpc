package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"xxrpc/common"
	"xxrpc/xxcode"
)

// Call represents an active RPC.
type Call struct {
	Seq           uint64
	ServiceMethod string      // format "<service>.<method>"
	Args          interface{} // arguments to the function 函数的参数
	Reply         interface{} // reply from the function 函数的返回值
	Error         error       // if error occurs, it will be set
	Done          chan *Call  // Strobes when call is complete.
}

// done 为了支持异步调用，当调用结束时，会调用 call.done() 通知调用方。
func (call *Call) done() {
	call.Done <- call
}

// Client 客户端代表一个RPC客户端。
// 一个客户端可能有多个未完成的调用
// 一个客户端可能有多个未完成的调用，并且一个客户端可能同时被
// 一个客户端可以被多个程序同时使用。
type Client struct {
	cc       xxcode.Code //消息的编解码器，和服务端类似，用来序列化将要发送出去的请求，以及反序列化接收到的响应。
	opt      *common.Option
	sending  sync.Mutex       //互斥锁，和服务端类似，为了保证请求的有序发送，即防止出现多个请求报文混淆。
	header   xxcode.Header    //每个请求的消息头，header 只有在请求发送时才需要，而请求发送是互斥的，因此每个客户端只需要一个，声明在 Client 结构体中可以复用。
	mu       sync.Mutex       // protect following
	seq      uint64           //seq 用于给发送的请求编号，每个请求拥有唯一编号。
	pending  map[uint64]*Call //存储未处理完的请求，键是编号，值是 Call 实例。
	closing  bool             // user has called Close,用户主动关闭的
	shutdown bool             // server has told us to stop, 一般是有错误发生。
}

var _ io.Closer = (*Client)(nil)

// Close the connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return errors.New("connection is closed")
	}
	c.closing = true
	return c.cc.Close()
}

// IsAvailable return true if the client does work
func (c *Client) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.shutdown && !c.closing
}

// 将参数 call 添加到 client.pending 中，并更新 client.seq。
func (c *Client) registerCall(call *Call) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing || c.shutdown {
		return 0, errors.New("connection is closed")
	}

	c.pending[c.seq] = call
	call.Seq = c.seq
	c.seq++
	return call.Seq, nil
}

// 根据 seq，从 client.pending 中移除对应的 call，并返回
func (c *Client) removeCall(seq uint64) *Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	call := c.pending[seq]
	delete(c.pending, seq)
	return call
}

// 服务端或客户端发生错误时调用，将 shutdown 设置为 true，且将错误信息通知所有 pending 状态的 call
func (c *Client) terminateCalls(err error) {
	// TODO:
	c.sending.Lock()
	defer c.sending.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdown = true
	for _, call := range c.pending {
		call.Error = err
		call.done()
	}
}

// 接收响应
// call 不存在，可能是请求没有发送完整，或者因为其他原因被取消，但是服务端仍旧处理了。
// call 存在，但服务端处理出错，即 head.Error 不为空。
// call 存在，服务端处理正常，那么需要从 body 中读取 Reply 的值。
func (c *Client) receive() {
	var err error
	for err == nil {
		var h xxcode.Header
		if err = c.cc.ReadHeader(&h); err != nil {
			break
		}
		// 从c.pending中依取出call
		call := c.removeCall(h.SeqId)
		switch {
		case call == nil: // 写入失败或者调用已经被删除
			err = c.cc.ReadBody(nil)
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = c.cc.ReadBody(nil)
			call.done()
		default:
			err = c.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}
	// 发生错误，终止c.pending中待定的调用
	c.terminateCalls(err)
}

// 创建 Client 实例时，首先需要完成一开始的协议交换，即发送 Option 信息给服务端。
// 协商好消息的编解码方式之后，再创建一个子协程调用 receive() 接收响应。

func NewClient(conn net.Conn, opt *common.Option) (*Client, error) {
	// TODO:
	f := xxcode.NewCodeFuncMap[opt.CodeType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodeType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	// send options with server
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: options error: ", err)
		_ = conn.Close()
		return nil, err
	}
	return newClientCode(f(conn), opt), nil
}

func newClientCode(cc xxcode.Code, opt *common.Option) *Client {
	client := &Client{
		seq:     1, // seq starts with 1, 0 means invalid call
		cc:      cc,
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}

func parseOptions(opts ...*common.Option) (*common.Option, error) {
	// if opts is nil or pass nil as parameter
	if len(opts) == 0 || opts[0] == nil {
		return common.DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = common.DefaultOption.MagicNumber
	if opt.CodeType == "" {
		opt.CodeType = common.DefaultOption.CodeType
	}
	return opt, nil
}

// 发送请求
func (c *Client) send(call *Call) {
	// make sure that the client will send a complete request
	c.sending.Lock()
	defer c.sending.Unlock()

	// register this call.
	seqId, err := c.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	// prepare request header
	c.header.ServiceMethod = call.ServiceMethod
	c.header.SeqId = seqId
	c.header.Error = ""

	// encode and send the request
	if err := c.cc.Write(&c.header, call.Args); err != nil {
		call := c.removeCall(seqId)
		// call may be nil, it usually means that Write partially failed,
		// client has received the response and handled
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// Go 以异步方式调用函数，返回代表调用的Call结构。
// Go 和 Call 是客户端暴露给用户的两个 RPC 服务调用接口，Go 是一个异步接口，返回 call 实例。
func (c *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}

	c.send(call)
	return call
}

// Call 调用命名的函数，等待它完成，并返回其错误状态。
// Call 是对 Go 的封装，阻塞 call.Done，等待响应返回，是一个同步接口
func (c *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	call := c.Go(serviceMethod, args, reply, make(chan *Call, 1))

	select {
	case <-ctx.Done():
		c.removeCall(call.Seq)
		return errors.New("rpc client: call failed: " + ctx.Err().Error())
	case call := <-call.Done:
		return call.Error
	}
}

type clientResult struct {
	client *Client
	err    error
}

type newClientFunc func(conn net.Conn, opt *common.Option) (client *Client, err error)

func dialTimeout(f newClientFunc, network, address string, opts ...*common.Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}

	// 将 net.Dial 替换为 net.DialTimeout，如果连接创建超时，将返回错误。
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	// close the connection if client is nil
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	ch := make(chan clientResult)
	go func() {
		client, err := f(conn, opt)
		ch <- clientResult{client: client, err: err}
	}()
	if opt.ConnectTimeout == 0 {
		result := <-ch
		return result.client, result.err
	}
	select {
	// 如果 time.After() 信道先接收到消息，则说明 NewClient 执行超时，返回错误。
	case <-time.After(opt.ConnectTimeout):
		return nil, fmt.Errorf("rpc client: connect timeout: expect within %s", opt.ConnectTimeout)
	case result := <-ch:
		return result.client, result.err
	}
}

// Dial connects to an RPC server at the specified network address
func Dial(network, address string, opts ...*common.Option) (*Client, error) {
	return dialTimeout(NewClient, network, address, opts...)
}
