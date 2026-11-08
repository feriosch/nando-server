package core

import (
	"context"
	"fmt"

	"google.golang.org/api/idtoken"
)

// GoogleSchedulerTokens returns the production VerifySchedulerToken: it
// accepts only a Google-signed OIDC token issued for audience (the tick URL
// Cloud Scheduler calls) to the scheduler's service account.
func GoogleSchedulerTokens(audience, serviceAccount string) func(ctx context.Context, token string) error {
	return func(ctx context.Context, token string) error {
		payload, err := idtoken.Validate(ctx, token, audience)
		if err != nil {
			return err
		}
		email, _ := payload.Claims["email"].(string)
		verified, _ := payload.Claims["email_verified"].(bool)
		if email != serviceAccount || !verified {
			return fmt.Errorf("token is for %q, not the scheduler's service account", email)
		}
		return nil
	}
}
