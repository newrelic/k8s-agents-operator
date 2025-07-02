package svcctx

import (
	"context"
)

type txid struct{}

var txID = txid{}

func ContextWithTXID(ctx context.Context, txid string) context.Context {
	return context.WithValue(ctx, txID, txid)
}

func TXIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(txID).(string)
	return id, ok
}
