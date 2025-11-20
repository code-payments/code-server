package client

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	versionPattern = "\\d+(.\\d+){0,2}"
)

var (
	versionRegex = regexp.MustCompile(fmt.Sprintf("^%s$", versionPattern))

	// todo: configurable
	minVersionByDevice = map[DeviceType]*Version{
		DeviceTypeIOS: {
			Major: 0,
			Minor: 0,
			Patch: 0,
		},
		DeviceTypeAndroid: {
			Major: 0,
			Minor: 0,
			Patch: 0,
		},
	}
)

type Version struct {
	Major int
	Minor int
	Patch int
}

func ParseVersion(value string) (*Version, error) {
	value = strings.TrimSpace(value)

	if !versionRegex.MatchString(value) {
		return nil, errors.New("version doesn't match regex")
	}

	versionComponents := strings.Split(value, ".")

	major, err := strconv.Atoi(versionComponents[0])
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse major part as an int")
	}

	var minor int
	if len(versionComponents) > 1 {
		minor, err = strconv.Atoi(versionComponents[1])
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse minor part as an int")
		}
	}

	var patch int
	if len(versionComponents) > 2 {
		patch, err = strconv.Atoi(versionComponents[2])
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse patch part as an int")
		}
	}

	return &Version{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}

func (v *Version) GreaterThanOrEqualTo(other *Version) bool {
	if other == nil {
		return true
	}

	// Compare major versions first
	if v.Major > other.Major {
		return true
	} else if v.Major < other.Major {
		return false
	}

	// Major versions match, so compare minor version next
	if v.Minor > other.Minor {
		return true
	} else if v.Minor < other.Minor {
		return false
	}

	// Minor versions match, so compare patch version next
	return v.Patch >= other.Patch
}

func (v *Version) Before(other *Version) bool {
	return !v.GreaterThanOrEqualTo(other)
}

func (v *Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// MinVersionUnaryServerInterceptor prevents versions below the minimum
// version from accessing outdated APIs.
func MinVersionUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Health checks are internal and don't have an external client user agent
		//
		// todo: Generic utility for whether the method is internal. Not needed right
		//       now because all RPC endpoints except health are external.
		if strings.Contains(info.FullMethod, "grpc.health.v1.Health/Check") {
			return handler(ctx, req)
		}

		userAgent, err := GetUserAgent(ctx)
		if err != nil {
			// Just continue on errors because we have a breaking change wrt the
			// user agent header value atm
			return handler(ctx, req)
		}

		if err := checkMinVersion(userAgent); err != nil {
			return nil, err
		}

		return handler(ctx, req)
	}
}

// MinVersionStreamServerInterceptor prevents versions below the minimum
// version from accessing lower version APIs.
func MinVersionStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		userAgent, err := GetUserAgent(ss.Context())
		if err != nil {
			// Just continue on errors because we have a breaking change wrt the
			// user agent header value atm
			return handler(srv, ss)
		}

		if err := checkMinVersion(userAgent); err != nil {
			return err
		}

		return handler(srv, ss)
	}
}

func checkMinVersion(userAgent *UserAgent) error {
	minVersion, ok := minVersionByDevice[userAgent.DeviceType]
	if !ok {
		return status.Error(codes.FailedPrecondition, "unsupported client type")
	}

	if userAgent.Version.Before(minVersion) {
		return status.Error(codes.FailedPrecondition, "version too low")
	}

	return nil
}
