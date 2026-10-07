package runner

import (
	"bytes"
	"os"
	"strconv"
)

// isZombie reports whether pid has exited and waits to be reaped.
func isZombie(pid int) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	i := bytes.LastIndexByte(data, ')')
	return i >= 0 && i+2 < len(data) && data[i+2] == 'Z'
}
