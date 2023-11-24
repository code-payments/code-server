package event

import (
	"context"
	"net"

	"github.com/oschwald/maxminddb-golang"

	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/netutil"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/event"
)

// InjectClientDetails injects client details into the provided event record. Metadata
// is provided on a best-effort basis.
//
// todo: We probably need a bit of a refactor for MaxMind (ie. put behind an interface, add to DataProvider, etc)
func InjectClientDetails(ctx context.Context, db *maxminddb.Reader, eventRecord *event.Record, isSource bool) {
	ip, err := client.GetIPAddr(ctx)
	if err != nil {
		return
	}

	if isSource {
		eventRecord.SourceClientIp = pointer.String(ip)
	} else {
		eventRecord.DestinationClientIp = pointer.String(ip)
	}

	if db == nil {
		return
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return
	}

	metadata, err := netutil.GetIpMetadata(ctx, db, ip)
	if err != nil {
		return
	}

	if isSource {
		eventRecord.SourceClientCity = metadata.City
		eventRecord.SourceClientCountry = metadata.Country
	} else {
		eventRecord.DestinationClientCity = metadata.City
		eventRecord.DestinationClientCountry = metadata.Country
	}
}
