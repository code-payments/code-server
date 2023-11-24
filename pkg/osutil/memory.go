package osutil

import (
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/pbnjay/memory"
)

const (
	// This is the default value for cgroup's limit_in_bytes. This is not a
	// valid value and indicates that the memory is not restricted.
	// See https://unix.stackexchange.com/questions/420906/what-is-the-value-for-the-cgroups-limit-in-bytes-if-the-memory-is-not-restricte
	unrestrictedMemoryLimit = 9223372036854771712
)

const (
	dockerMemoryLimitLocation = "/sys/fs/cgroup/memory/memory.limit_in_bytes"
)

// GetTotalMemory returns the total available memory size. The call is
// container-aware.
func GetTotalMemory() uint64 {
	totalMemory := memory.TotalMemory()

	cgroupLimit, err := ioutil.ReadFile(dockerMemoryLimitLocation)
	if err == nil {
		dockerMemoryLimit, err := strconv.ParseUint(strings.Replace(string(cgroupLimit), "\n", "", 1), 10, 64)
		if err == nil && dockerMemoryLimit != unrestrictedMemoryLimit {
			totalMemory = dockerMemoryLimit
		}
	}
	return totalMemory
}
