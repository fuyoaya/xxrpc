package common

import (
	"time"

	"xxrpc/xxcode"
)

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber    int           // MagicNumber marks this is a rpc request
	CodeType       xxcode.Type   // client may choose different Codec to encode body
	ConnectTimeout time.Duration // 0 means no limit
	HandleTimeout  time.Duration
}

var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodeType:       xxcode.Type_Gob,
	ConnectTimeout: time.Second * 10,
}
