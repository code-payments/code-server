package headers

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// HeaderKey is the key to store all the header information in the context
type HeaderKey string

// Note, as edge layers will need to manually set the root header using a HeaderKey type
// There is no API to set root headers, only retrieve them.
const (
	rootHeaderKey           HeaderKey = "root-bin-header"
	propagatingHeaderKey    HeaderKey = "propagating-bin-header"
	inboundBinaryHeaderKey  HeaderKey = "inbound-bin-header"
	outboundBinaryHeaderKey HeaderKey = "outbound-bin-header"
	asciiHeaderKey          HeaderKey = "ascii-header"
)

// SetHeader sets the outbound header in the given context.
func SetHeader(ctx context.Context, data proto.Message) error {
	return setHeader(ctx, data, getBinaryHeaderName(data), Outbound)
}

// SetHeaderByName sets the outbound header in the given context.
func SetHeaderByName(ctx context.Context, data proto.Message, name string) error {
	return setHeader(ctx, data, name, Outbound)
}

// SetPropagatingHeader sets the propagating header in the given context.
func SetPropagatingHeader(ctx context.Context, data proto.Message) error {
	return setHeader(ctx, data, getBinaryHeaderName(data), Propagating)
}

// SetASCIIHeader sets an ASCII header in the given context.
func SetASCIIHeader(ctx context.Context, name string, data string) error {
	header, err := getHeadersFromContext(ctx, ASCII)
	if err != nil {
		return err
	}
	header[strings.ToLower(name)] = data
	return nil
}

// setHeader adds the named proto to the desired header. Note, only one instance of the named proto can be
// set per header.
func setHeader(ctx context.Context, data proto.Message, name string, headerType Type) (err error) {
	selectedHeader, err := getHeadersFromContext(ctx, headerType)
	if err != nil {
		return err
	}

	headerName := getPrefixedHeaderName(name, headerType)
	selectedHeader[headerName], err = proto.Marshal(data)
	return err
}

// GetHeader takes the inbound header and unmarshal the data into the destination message.
// If no header exists, destination will have all default values.
func GetHeader(ctx context.Context, destination proto.Message) error {
	return getHeader(ctx, destination, getBinaryHeaderName(destination), Inbound)
}

// GetHeaderByName takes the inbound header and unmarshal the data into the destination messaging.
// If no header exists, destination will have all default values.
func GetHeaderByName(ctx context.Context, destination proto.Message, name string) error {
	return getHeader(ctx, destination, name, Inbound)
}

// GetStringHeaderByName takes the inbound header and returns the data, if it is a string.
// If no header exists or the header value is not a string, an empty string will be returned.
func GetASCIIHeaderByName(ctx context.Context, name string) (string, error) {
	selectedHeader, err := getHeadersFromContext(ctx, ASCII)
	if err != nil {
		return "", err
	}
	data, exists := selectedHeader[name]
	if !exists {
		logrus.StandardLogger().Tracef("Header %s not found in type %s. An empty string will be returned", name, ASCII)
		return "", nil
	}
	if val, ok := data.(string); ok {
		return val, nil
	}

	return "", fmt.Errorf("header %s does not have an ASCII value (%T)", name, data)
}

// GetStringHeaderByName takes the inbound header and returns the data, if it is a string.
// If no header exists or the header value is not a string, an empty string will be returned.
func GetStringHeaderByName(ctx context.Context, name string) (string, error) {
	return getStringHeader(ctx, name, Inbound)
}

// GetPropagatingHeader takes the propagating header and unmarshal the data into the destination message.
// If no header exists, destination will have all default values
func GetPropagatingHeader(ctx context.Context, destination proto.Message) error {
	return getHeader(ctx, destination, getBinaryHeaderName(destination), Propagating)
}

// GetRootHeader takes the root header and unmarshal the data into the destination message.
// If no header exists, destination will have all default values.
func GetRootHeader(ctx context.Context, destination proto.Message) error {
	return getHeader(ctx, destination, getBinaryHeaderName(destination), Root)
}

// GetRootHeaderByName takes the root header and unmarshal the data into the destination message.
// If no header exists, destination will have all default values.
func GetRootHeaderByName(ctx context.Context, destination proto.Message, name string) error {
	return getHeader(ctx, destination, name, Root)
}

// GetAnyBinaryHeader will try to find the header by checking all the binary headers, in the following order.
//
// 1. GetRootHeader()
// 2. GetHeader()
// 3. GetPropagatingHeader()
//
// Note, it is not recommended to to use this method to retrieve headers, as it may induce
// some ambiguity. For example, XiUUid may have a different meaning in each Type.
func GetAnyBinaryHeader(ctx context.Context, destination proto.Message) error {
	return GetAnyBinaryHeaderByName(ctx, destination, getBinaryHeaderName(destination))
}

// GetAnyBinaryHeaderByName will try to find the header by checking all the binary headers, in the following order.
//
// 1. GetRootHeader()
// 2. GetHeader()
// 3. GetPropagatingHeader()
//
// Note, it is not recommended to to use this method to retrieve headers, as it may induce
// some ambiguity. For example, XiUUid may have a different meaning in each Type.
func GetAnyBinaryHeaderByName(ctx context.Context, destination proto.Message, name string) error {
	headers := []Type{Root, Inbound, Propagating}
	for _, headerType := range headers {
		err := getHeader(ctx, destination, name, headerType)
		if err == nil && destination.String() != "" {
			return nil
		} else if !strings.Contains(err.Error(), "header not found") {
			return errors.Wrapf(err, "issue finding header %s in type %s", proto.MessageName(destination), headerType)
		}
	}

	return fmt.Errorf("could not find header information for %s in any binary header", proto.MessageName(destination))
}

// getHeader takes the given header and unmarshal the data into the destination message.
// If no header exists, destination will have all default values.
func getHeader(ctx context.Context, destination proto.Message, name string, headerType Type) error {
	selectedHeader, err := getHeadersFromContext(ctx, headerType)
	if err != nil {
		return err
	}
	headerName := getPrefixedHeaderName(name, headerType)
	data, exists := selectedHeader[headerName]
	if !exists {
		logrus.StandardLogger().Tracef("Header %s not found in type %s. Proto %s will have default values", headerName, headerType, proto.MessageName(destination))
		return fmt.Errorf("%s %s header not found", headerType, proto.MessageName(destination))
	}
	if rawBytes, ok := data.([]byte); ok {
		err := proto.Unmarshal(rawBytes, destination)
		return errors.Wrapf(err, "failed to unmarshal %s header %s", headerType, proto.MessageName(destination))
	}

	return fmt.Errorf("header %s in type %s does not have a binary value (%T)", proto.MessageName(destination), headerType, data)
}

// getStringHeader takes the given header and returns the data, if it is a string.
// If no header exists or the value is not a string, an empty string will be returned.
func getStringHeader(ctx context.Context, name string, headerType Type) (string, error) {
	selectedHeader, err := getHeadersFromContext(ctx, headerType)
	if err != nil {
		return "", err
	}
	headerName := getPrefixedHeaderName(name, headerType)
	data, exists := selectedHeader[headerName]
	if !exists {
		logrus.StandardLogger().Tracef("Header %s not found in type %s.", headerName, headerType)
		return "", fmt.Errorf("%s string header not found", headerType)
	}
	if val, ok := data.(string); ok {
		return val, nil
	}

	return "", fmt.Errorf("header %s in type %s does not have a string value (%T)", name, headerType, data)
}

// getHeadersFromContext returns all the headers of headerType from the given context.
func getHeadersFromContext(ctx context.Context, headerType Type) (Headers, error) {
	var ok bool
	var selectedHeader Headers
	switch headerType {
	case Propagating:
		selectedHeader, ok = ctx.Value(propagatingHeaderKey).(Headers)
	case Root:
		selectedHeader, ok = ctx.Value(rootHeaderKey).(Headers)
	case Outbound:
		selectedHeader, ok = ctx.Value(outboundBinaryHeaderKey).(Headers)
	case ASCII:
		selectedHeader, ok = ctx.Value(asciiHeaderKey).(Headers)
	default:
		selectedHeader, ok = ctx.Value(inboundBinaryHeaderKey).(Headers)
	}

	if !ok {
		// Either headers weren't initialized or something horrible happened
		return nil, fmt.Errorf("headers of type %s were not initalized", headerType)
	}

	return selectedHeader, nil
}

// getBinaryHeaderName returns the name of the header using data's ProtoName() func and appending "-bin" to symbolize it is a binary header.
// An absence of "-bin" symbolizes a ascii header.
func getBinaryHeaderName(data proto.Message) string {
	return proto.MessageName(data) + "-bin"
}

// getPrefixedHeaderName returns the prefixed header name.
func getPrefixedHeaderName(name string, headerType Type) string {
	return strings.ToLower(headerType.prefix() + name)
}

// headerSetterFunc is a func that sets a header in a context. Should be either SetHeader or SetPropagatingHeader.
type headerSetterFunc func(ctx context.Context, data proto.Message) error

// HeaderSetter a pair of a headerSetterFunc and the proto header to set.
// It is meant to be used with ContextWithHeaders to create a new context with all the proper Header information.
type HeaderSetter struct {
	HeaderSetterFunc headerSetterFunc
	Data             proto.Message
}

// DefaultHeaderSetter returns a HeaderSetter using the SetHeader and the namedProto passed in as the Data.
func DefaultHeaderSetter(namedProto proto.Message) HeaderSetter {
	return HeaderSetter{HeaderSetterFunc: SetHeader, Data: namedProto}
}

// PropagatingHeaderSetter returns a HeaderSetter using SetPropagatingHeader and the namedProto passed in as the Data.
func PropagatingHeaderSetter(namedProto proto.Message) HeaderSetter {
	return HeaderSetter{HeaderSetterFunc: SetPropagatingHeader, Data: namedProto}
}

// AreHeadersInitialized determines whether headers have been initialized for the provided context. We assume headers
// are always initialized via ContextWithHeaders.
func AreHeadersInitialized(ctx context.Context) bool {
	// Only need to check one, since they'll all be set at the same time by ContextWithHeaders
	_, ok := ctx.Value(asciiHeaderKey).(Headers)
	return ok
}

// ContextWithHeaders creates a new context with predefined headerPrefixes. Needed when starting from a fresh context,
// as they will not come with the Header keys needed to send the header to the next service.
func ContextWithHeaders(ctx context.Context, headersSetters ...HeaderSetter) (context.Context, error) {
	ctx = context.WithValue(ctx, inboundBinaryHeaderKey, Headers{})
	ctx = context.WithValue(ctx, asciiHeaderKey, Headers{})
	ctx = context.WithValue(ctx, outboundBinaryHeaderKey, Headers{})
	ctx = context.WithValue(ctx, propagatingHeaderKey, Headers{})
	ctx = context.WithValue(ctx, rootHeaderKey, Headers{})

	for _, v := range headersSetters {
		err := v.HeaderSetterFunc(ctx, v.Data)
		if err != nil {
			// Something went wrong, probably safest to return an empty context
			return context.Background(), err
		}
	}

	return ctx, nil
}
