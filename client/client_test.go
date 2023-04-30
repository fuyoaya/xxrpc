package client

import (
	"context"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"xxrpc/common"
	"xxrpc/server"
)

type Bar int

func (b Bar) Timeout(argv int, reply *int) error {
	time.Sleep(time.Second * 2)
	return nil
}

func startServer(addr chan string) {
	var b Bar
	_ = server.Register(&b)
	// pick a free port
	listener, _ := net.Listen("tcp", ":8888")
	addr <- listener.Addr().String()
	server.Accept(listener)
}

// 用于测试连接超时。NewClient 函数耗时 2s，ConnectionTimeout 分别设置为 1s 和 0 两种场景。
func TestClient_dialTimeout(t *testing.T) {
	t.Parallel()
	l, _ := net.Listen("tcp", ":8888")

	f := func(conn net.Conn, opt *common.Option) (client *Client, err error) {
		_ = conn.Close()
		time.Sleep(time.Second * 2)
		return nil, nil
	}
	t.Run("timeout", func(t *testing.T) {
		_, err := dialTimeout(f, "tcp", l.Addr().String(), &common.Option{ConnectTimeout: time.Second})
		t.Log(err.Error())
	})
	t.Run("0", func(t *testing.T) {
		_, err := dialTimeout(f, "tcp", l.Addr().String(), &common.Option{ConnectTimeout: 0})
		t.Log(err.Error())
	})
}

// 测试处理超时。Bar.Timeout 耗时 2s，场景一：客户端设置超时时间为 1s，服务端无限制；场景二，服务端设置超时时间为1s，客户端无限制。
func TestClient_Call(t *testing.T) {
	t.Parallel()
	addrCh := make(chan string)
	go startServer(addrCh)
	addr := <-addrCh
	time.Sleep(time.Second)
	t.Run("client timeout", func(t *testing.T) {
		client, _ := DialHTTP("tcp", addr)
		ctx, _ := context.WithTimeout(context.Background(), time.Second)
		//ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
		var reply int
		err := client.Call(ctx, "Bar.Timeout", 1, &reply)
		t.Log(err.Error())
	})
	t.Run("server handle timeout", func(t *testing.T) {
		client, _ := DialHTTP("tcp", addr, &common.Option{
			HandleTimeout: time.Second,
		})
		var reply int
		err := client.Call(context.Background(), "Bar.Timeout", 1, &reply)
		t.Log(err.Error())
	})
}

func TestXDial(t *testing.T) {
	if runtime.GOOS == "linux" {
		ch := make(chan struct{})
		addr := "/tmp/geerpc.sock"
		go func() {
			_ = os.Remove(addr)
			l, err := net.Listen("unix", addr)
			if err != nil {
				t.Fatal("failed to listen unix socket")
			}
			ch <- struct{}{}
			server.Accept(l)
		}()
		<-ch
		_, err := XDial("unix@" + addr)
		t.Log(err.Error())
	}
}
