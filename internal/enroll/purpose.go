package enroll

import (
	"context"
	"time"
)

// Purpose says who an enrollment profile is generated for; it decides the
// lifetime of the SCEP challenge minted into it.
type Purpose string

// Profile purposes.
const (
	PurposeUser         Purpose = "user"         // self-service download from /enroll
	PurposeAgent        Purpose = "agent"        // fetched by the agent to install MDM
	PurposeAdmin        Purpose = "admin"        // downloaded by an admin from the console
	PurposeConfigurator Purpose = "configurator" // Apple Configurator, reused for a batch of devices
)

// ChallengePolicy returns the challenge lifetime and whether it is single-use.
func (p Purpose) ChallengePolicy() (time.Duration, bool) {
	switch p {
	case PurposeAdmin:
		return 7 * 24 * time.Hour, true
	case PurposeConfigurator:
		return 24 * time.Hour, false
	case PurposeAgent:
		return time.Hour, true
	}
	return 2 * time.Hour, true
}

type purposeKey struct{}

// WithPurpose tags a context with a profile purpose.
func WithPurpose(ctx context.Context, p Purpose) context.Context {
	return context.WithValue(ctx, purposeKey{}, p)
}

// PurposeFrom returns the purpose (default PurposeUser).
func PurposeFrom(ctx context.Context) Purpose {
	if p, ok := ctx.Value(purposeKey{}).(Purpose); ok {
		return p
	}
	return PurposeUser
}
