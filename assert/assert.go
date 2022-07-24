package assert

import (
	"fmt"

	"github.com/bloeys/nterm/consts"
)

func T(check bool, msg string, args ...any) {
	if consts.Mode_Debug && !check {
		panic("Assert failed: " + fmt.Sprintf(msg, args...))
	}
}
