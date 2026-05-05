package ctx

import (
	"net/http"
)

type BlockInfo struct {
	Middleware string
	Reason     string
}

func CreateAndAddBlockInfo(r *http.Request, middleware, reason string) {
	b := &BlockInfo{
		Middleware: middleware,
		Reason:     reason,
	}
	SetBlockInfo(r, b)
}

func SetBlockInfo(r *http.Request, info *BlockInfo) {
	ctxStruct := GetContextStruct(r)
	ctxStruct.BlockInfo = info
}

func GetBlockInfo(r *http.Request) *BlockInfo {
	ctxStruct := GetContextStruct(r)
	return ctxStruct.BlockInfo
}
