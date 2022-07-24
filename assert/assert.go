package assert

import (
	"fmt"

	"github.com/bloeys/nterm/consts"
)

func T(check bool, msg string, args ...any) {
	if consts.Mode_Debug && !check {
		// Sprintf is done inside the assert because putting it as the argument to 'msg' blocks
		// the function from getting fully optimized out on a release build (and slower in general)
		panic("Assert failed: " + fmt.Sprintf(msg, args...))
	}
}
