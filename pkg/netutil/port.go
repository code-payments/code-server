package netutil

import (
	"fmt"
	"net"
	"strconv"
)

// GetAvailablePortForAddress returns an open port on the specified address
func GetAvailablePortForAddress(address string) (int32, error) {
	server, err := net.Listen("tcp", fmt.Sprintf("%s:0", address))
	if err != nil {
		return 0, err
	}
	defer server.Close()
	_, portString, err := net.SplitHostPort(server.Addr().String())
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(portString)
	return int32(port), err
}
