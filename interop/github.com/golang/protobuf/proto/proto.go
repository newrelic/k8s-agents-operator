package proto

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoiface"
	"google.golang.org/protobuf/runtime/protoimpl"
)

func Marshal(m protoiface.MessageV1) ([]byte, error) {
	return proto.Marshal(protoimpl.X.ProtoMessageV2Of(m))
}

func Unmarshal(b []byte, m protoiface.MessageV1) error {
	return proto.Unmarshal(b, protoimpl.X.ProtoMessageV2Of(m))
}
