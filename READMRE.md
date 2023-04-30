# 什么是rpc

RPC(Remote Procedure Call，远程过程调用)是一种计算机通信协议，允许调用不同进程空间的程序。
RPC的客户端和服务器可以在一台机器上，也可以在不同的机器上。
程序员使用时，就像调用本地程序一样，无需关注内部的实现细节。

不同于 `Restful` 接口需要额外的定义，无论是客户端还是服务端，都需要额外的代码来处理，而 `RPC` 调用则更接近于直接调用。
基于 `HTTP` 协议的 `Restful` 报文冗余，承载了过多的无效信息，
而 `RPC` 通常使用自定义的协议格式，减少冗余报文。
`RPC` 可以采用更高效的序列化协议，将文本转为二进制传输，获得更高的性能。
因为 `RPC` 的灵活性，所以更容易扩展和集成诸如注册中心、负载均衡等功能。


RPC 框架的一个基础能力是：像调用本地程序一样调用远程服务。
那如何将程序映射为服务呢？
那么对 Go 来说，这个问题就变成了如何将结构体的方法映射为服务。
对 net/rpc 而言，一个函数需要能够被远程调用，需要满足如下五个条件：
* the method’s type is exported. – 方法所属类型是导出的。
* the method is exported. – 方式是导出的。
* the method has two arguments, both exported (or builtin) types. – 两个入参，均为导出或内置类型。
* the method’s second argument is a pointer. – 第二个入参必须是一个指针。
* the method has return type error. – 返回值为 error 类型。

更直观一些：
`func (t *T) MethodName(argType T1, replyType *T2) error`