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

func TestClient_Call(t *testing.T) {
	t.Parallel()
	addrCh := make(chan string)
	go startServer(addrCh)
	addr := <-addrCh
	time.Sleep(time.Second)
	t.Run("client timeout", func(t *testing.T) {
		client, _ := DialHTTP("tcp", addr)
		ctx, _ := context.WithTimeout(context.Background(), time.Second)
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
