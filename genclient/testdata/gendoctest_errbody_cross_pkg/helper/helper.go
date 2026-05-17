package helper

import "github.com/mackee/tanukirpc"

type ErrorBody struct {
	Message string `json:"message"`
}

func build(err error) ErrorBody { return ErrorBody{Message: err.Error()} }

// NewRouter is an exported factory living in another package than the one the
// analyzer inspects. The analyzer cannot see this function's SSA body, so the
// underlying WithErrorBody is invisible to gentypescript. The analyzer must
// surface a warning in that situation instead of silently emitting the default
// error shape.
func NewRouter() *tanukirpc.Router[struct{}] {
	return tanukirpc.NewRouter(
		struct{}{},
		tanukirpc.WithErrorBody[struct{}](build),
	)
}
