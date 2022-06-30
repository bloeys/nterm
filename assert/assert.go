package assert

import (
	"github.com/bloeys/nterm/consts"
)

func T(check bool, msg string) {
	if consts.Mode_Debug && !check {
		panic("Assert failed: " + msg)
	}
}
