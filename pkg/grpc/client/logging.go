package client

import (
	"context"

	"github.com/sirupsen/logrus"
)

// InjectLoggingMetadata injects client metadata into a logrus log entry
func InjectLoggingMetadata(ctx context.Context, log *logrus.Entry) *logrus.Entry {
	userAgent, err := GetUserAgent(ctx)
	if err == nil {
		log = log.WithField("user_agent", userAgent.String())
	}

	ip, err := GetIPAddr(ctx)
	if err == nil {
		log = log.WithField("client_ip", ip)
	}

	return log
}
