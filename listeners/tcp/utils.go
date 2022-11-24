package tcp

import (
	"github.com/achetronic/redis-proxy/api"
	"net"
	"regexp"
	"strconv"
)

// getTCPAddress return a pointer to an object TCPAddr built from strings
func getTCPAddress(host string, port int) (address *net.TCPAddr, err error) {
	address, err = net.ResolveTCPAddr(ProtocolTcp, net.JoinHostPort(host, strconv.Itoa(port)))
	return address, err
}

// generateConnectionPoolKey return a new connection track key from a UDP Addr
func generateConnectionPoolKey(conn *net.Conn) (conTrackKey *api.ConnectionTrackKey, err error) {

	address := (*conn).RemoteAddr().String()

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

// getRegexNamedGroups return a map of named groups and their values
// for all the matches of a regex expression into the chunk
func getRegexNamedGroups(r *regexp.Regexp, chunk []byte) (subMatchList []RegexNamedMatchMap) {

	groupNames := r.SubexpNames()

	for _, match := range r.FindAllSubmatch(chunk, -1) {
		currentMatchGroups := RegexNamedMatchMap{}

		for groupIdx, groupValue := range match {
			groupName := groupNames[groupIdx]
			if groupName == "" {
				continue
			}

			currentMatchGroups[groupName] = groupValue
		}
		subMatchList = append(subMatchList, currentMatchGroups)
	}
	return subMatchList
}
