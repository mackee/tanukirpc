package gendoctest_no_target

import "github.com/mackee/tanukirpc"

func unrelatedClosures() {
	_ = tanukirpc.NewRouter(struct{}{})
	ids := func(n int) int { return n }
	_ = ids(1)
	_ = ids(2)
}
