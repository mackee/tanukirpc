package sessions

import "net/http"

// ReqResp is an interface for request and response. uses for SessionAccessor.
type ReqResp interface {
	Request() *http.Request
	Response() http.ResponseWriter
}

// Accessor is an interface for session access.
type Accessor interface {
	Set(key, value any) error
	Get(key string) (any, bool)
	Remove(key string) error
	Save(ctx ReqResp) error
}

type RegistryWithAccessor interface {
	Session() Accessor
}

type Store interface {
	GetAccessor(req *http.Request) (Accessor, error)
}
