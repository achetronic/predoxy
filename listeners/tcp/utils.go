package tcp

import (
	"github.com/achetronic/redis-proxy/api"
	"net"
	"strconv"
)

// getTCPAddress return a pointer to an object TCPAddr built from strings
func getTCPAddress(host string, port int) (address *net.TCPAddr, err error) {
	address, err = net.ResolveTCPAddr(ProtocolTcp, net.JoinHostPort(host, strconv.Itoa(port)))
	return address, err
}

// generateConnectionPoolKey return a new connection track key from a UDP Addr
func generateConnectionPoolKey(conn *net.TCPConn) (conTrackKey *api.ConnectionTrackKey, err error) {

	address := conn.RemoteAddr().String()

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return conTrackKey, err
	}

	parsedPort, err := strconv.Atoi(port)
	if err != nil {
		return conTrackKey, err
	}

	return &api.ConnectionTrackKey{
		IP:   host,
		Port: parsedPort,
	}, err
}
