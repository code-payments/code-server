package netutil

import (
	"context"
	"net"

	"github.com/oschwald/maxminddb-golang"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/pointer"
)

type IpMetadata struct {
	City    *string
	Country *string
}

type maxMindRecord struct {
	City struct {
		Names struct {
			En string `maxminddb:"en"`
		} `maxminddb:"names"`
	} `maxminddb:"city"`
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

// GetOutboundIP gets the locally preferred outbound IP address
//
// From https://stackoverflow.com/questions/23558425/how-do-i-get-the-local-ip-address-in-go
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

// GetIpMetadata gets metadata about an IP. Information is provided on a best-effort
// basis.
func GetIpMetadata(ctx context.Context, db *maxminddb.Reader, ip string) (*IpMetadata, error) {
	if db == nil {
		return &IpMetadata{}, nil
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil, errors.New("cannot parse ip")
	}

	var metadata maxMindRecord
	err := db.Lookup(parsed, &metadata)
	if err != nil {
		return nil, errors.Wrap(err, "error looking up ip metadata")
	}

	return &IpMetadata{
		City:    pointer.StringIfValid(len(metadata.City.Names.En) > 0, metadata.City.Names.En),
		Country: pointer.StringIfValid(len(metadata.Country.ISOCode) > 0, metadata.Country.ISOCode),
	}, nil
}
