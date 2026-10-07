package runner

import (
	"bytes"
	"errors"
	"os"
	"strconv"
	"syscall"
)

// processEnv returns the environment pid was started with
// (/proc/<pid>/environ), for the caller's own processes only.
func processEnv(pid int) ([]string, error) {
	dir := "/proc/" + strconv.Itoa(pid)
	fi, err := os.Stat(dir)
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller skips the process
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Getuid() {
		return nil, errors.New("not the caller's process")
	}
	data, err := os.ReadFile(dir + "/environ")
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller skips the process
	}
	var env []string
	for _, kv := range bytes.Split(data, []byte{0}) {
		if len(kv) > 0 {
			env = append(env, string(kv))
		}
	}
	return env, nil
}
