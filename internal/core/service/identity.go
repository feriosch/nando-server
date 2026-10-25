package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/feriosch/nando-server/internal/core/model"
)

const (
	enrollmentCodeLifetime = 15 * time.Minute
	// ceremonyTimeout is how long the Owner has to answer the Face ID prompt.
	ceremonyTimeout = 5 * time.Minute
	// enrollmentCodeAlphabet has 32 symbols, leaving out I, L, O and U, which
	// are easily misread.
	enrollmentCodeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

// IssueEnrollmentCode stores and returns a new Enrollment Code.
func (s *Service) IssueEnrollmentCode(ctx context.Context) (string, error) {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = enrollmentCodeAlphabet[int(b[i])%len(enrollmentCodeAlphabet)]
	}
	code := string(b[:5]) + "-" + string(b[5:])
	err := s.repo.InsertEnrollmentCode(ctx, model.EnrollmentCode{
		Hash:      sha256Of(code),
		ExpiresAt: s.now().Add(enrollmentCodeLifetime),
	})
	if err != nil {
		return "", err
	}
	return code, nil
}

// owner is the Owner as go-webauthn sees them. There is only one, so the user
// handle is fixed.
type owner struct {
	credentials []webauthn.Credential
}

func (owner) WebAuthnID() []byte                           { return []byte("owner") }
func (owner) WebAuthnName() string                         { return "Nando" }
func (owner) WebAuthnDisplayName() string                  { return "Nando" }
func (o owner) WebAuthnCredentials() []webauthn.Credential { return o.credentials }

// beginRegistration answers {"code": "..."} with the options for
// navigator.credentials.create, if the code is a valid Enrollment Code.
func (s *Service) beginRegistration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `body must be {"code": "..."}`, http.StatusBadRequest)
		return
	}
	codeHash := sha256Of(req.Code)
	if _, err := s.repo.FindEnrollmentCode(r.Context(), codeHash, s.now()); err != nil {
		enrollmentCodeError(w, err)
		return
	}

	// Excluding the Owner's passkeys stops a device that already has one from
	// silently replacing it, which would leave the old one listed but unusable.
	o, err := s.loadOwner(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	exclusions := webauthn.Credentials(o.credentials).CredentialDescriptors()
	creation, sessionData, err := s.webauthn.BeginRegistration(o, webauthn.WithExclusions(exclusions))
	if err != nil {
		serverError(w, err)
		return
	}
	if err := s.beginCeremony(r.Context(), sessionData, codeHash); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, creation)
}

// finishRegistration stores the new passkey from navigator.credentials.create,
// uses up the Enrollment Code and signs this device in.
func (s *Service) finishRegistration(w http.ResponseWriter, r *http.Request) {
	parsed, err := protocol.ParseCredentialCreationResponseBody(r.Body)
	if err != nil {
		http.Error(w, "body must be a PublicKeyCredential", http.StatusBadRequest)
		return
	}
	ceremony, sessionData, err := s.takeCeremony(r.Context(), parsed.Response.CollectedClientData.Challenge)
	if err != nil {
		ceremonyError(w, err)
		return
	}
	if ceremony.EnrollmentCodeHash == nil {
		http.Error(w, "that challenge was not for a registration", http.StatusBadRequest)
		return
	}
	credential, err := s.webauthn.CreateCredential(owner{}, sessionData, parsed)
	if err != nil {
		http.Error(w, "the passkey could not be verified", http.StatusUnauthorized)
		return
	}

	stored, err := json.Marshal(credential)
	if err != nil {
		serverError(w, err)
		return
	}
	err = s.repo.InsertPasskey(r.Context(), model.Passkey{ID: credential.ID, Credential: stored, CreatedAt: s.now()})
	if err != nil {
		serverError(w, err)
		return
	}
	// The code is used up only once the passkey is stored, so a cancelled
	// Face ID prompt or a failed insert doesn't waste it. If another
	// registration used it first, this passkey is taken back out.
	if err := s.repo.UseEnrollmentCode(r.Context(), ceremony.EnrollmentCodeHash, s.now()); err != nil {
		if deleteErr := s.repo.DeletePasskey(r.Context(), credential.ID); deleteErr != nil {
			serverError(w, errors.Join(err, deleteErr))
			return
		}
		enrollmentCodeError(w, err)
		return
	}
	if err := s.startSession(w, r); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// beginSignIn answers with the options for navigator.credentials.get, letting
// the Owner pick any of their passkeys.
func (s *Service) beginSignIn(w http.ResponseWriter, r *http.Request) {
	assertion, sessionData, err := s.webauthn.BeginDiscoverableLogin()
	if err != nil {
		serverError(w, err)
		return
	}
	if err := s.beginCeremony(r.Context(), sessionData, nil); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, assertion)
}

// finishSignIn checks the passkey's answer from navigator.credentials.get and
// signs this device in.
func (s *Service) finishSignIn(w http.ResponseWriter, r *http.Request) {
	parsed, err := protocol.ParseCredentialRequestResponseBody(r.Body)
	if err != nil {
		http.Error(w, "body must be a PublicKeyCredential", http.StatusBadRequest)
		return
	}
	ceremony, sessionData, err := s.takeCeremony(r.Context(), parsed.Response.CollectedClientData.Challenge)
	if err != nil {
		ceremonyError(w, err)
		return
	}
	if ceremony.EnrollmentCodeHash != nil {
		http.Error(w, "that challenge was not for a sign-in", http.StatusBadRequest)
		return
	}
	_, _, err = s.webauthn.ValidatePasskeyLogin(s.findOwner(r.Context()), sessionData, parsed)
	if err != nil {
		http.Error(w, "that passkey is unknown or could not be verified", http.StatusUnauthorized)
		return
	}
	if err := s.startSession(w, r); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// findOwner finds the Owner by the passkey they signed in with.
func (s *Service) findOwner(ctx context.Context) webauthn.DiscoverableUserHandler {
	return func(rawID, userHandle []byte) (webauthn.User, error) {
		passkey, err := s.repo.FindPasskey(ctx, rawID)
		if err != nil {
			return nil, err
		}
		credential, err := credentialOf(passkey)
		if err != nil {
			return nil, err
		}
		return owner{credentials: []webauthn.Credential{credential}}, nil
	}
}

// loadOwner returns the Owner with all their passkeys.
func (s *Service) loadOwner(ctx context.Context) (owner, error) {
	passkeys, err := s.repo.ListPasskeys(ctx)
	if err != nil {
		return owner{}, err
	}
	var o owner
	for _, passkey := range passkeys {
		credential, err := credentialOf(passkey)
		if err != nil {
			return owner{}, err
		}
		o.credentials = append(o.credentials, credential)
	}
	return o, nil
}

func credentialOf(passkey model.Passkey) (webauthn.Credential, error) {
	var credential webauthn.Credential
	if err := json.Unmarshal(passkey.Credential, &credential); err != nil {
		return credential, fmt.Errorf("passkey %x: %w", passkey.ID, err)
	}
	return credential, nil
}

// listPasskeys lists the Owner's passkeys.
func (s *Service) listPasskeys(w http.ResponseWriter, r *http.Request) {
	passkeys, err := s.repo.ListPasskeys(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	type passkeyJSON struct {
		ID        string    `json:"id"`
		CreatedAt time.Time `json:"createdAt"`
	}
	list := make([]passkeyJSON, len(passkeys))
	for i, passkey := range passkeys {
		list[i] = passkeyJSON{ID: base64.RawURLEncoding.EncodeToString(passkey.ID), CreatedAt: passkey.CreatedAt}
	}
	writeJSON(w, list)
}

// deletePasskey stops a passkey signing in. Devices already signed in with it
// stay signed in until their sessions are deleted.
func (s *Service) deletePasskey(w http.ResponseWriter, r *http.Request) {
	id, err := base64.RawURLEncoding.DecodeString(r.PathValue("id"))
	if err != nil {
		http.Error(w, "that is not a passkey id", http.StatusBadRequest)
		return
	}
	if err := s.repo.DeletePasskey(r.Context(), id); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func enrollmentCodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, model.ErrNotFound) {
		http.Error(w, "that Enrollment Code is unknown, used or expired", http.StatusUnauthorized)
		return
	}
	serverError(w, err)
}

// beginCeremony keeps sessionData, go-webauthn's state, until the ceremony's finish request.
func (s *Service) beginCeremony(ctx context.Context, sessionData *webauthn.SessionData, enrollmentCodeHash []byte) error {
	stored, err := json.Marshal(sessionData)
	if err != nil {
		return err
	}
	return s.repo.InsertCeremony(ctx, model.Ceremony{
		Challenge:          sessionData.Challenge,
		SessionData:        stored,
		EnrollmentCodeHash: enrollmentCodeHash,
		ExpiresAt:          s.now().Add(ceremonyTimeout),
	})
}

// errNoCeremony means a finish request has no begun, unfinished ceremony.
var errNoCeremony = errors.New("no ceremony")

// takeCeremony ends the ceremony with this challenge, so it can finish only once.
func (s *Service) takeCeremony(ctx context.Context, challenge string) (model.Ceremony, webauthn.SessionData, error) {
	var sessionData webauthn.SessionData
	ceremony, err := s.repo.TakeCeremony(ctx, challenge, s.now())
	if errors.Is(err, model.ErrNotFound) {
		return ceremony, sessionData, errNoCeremony
	}
	if err != nil {
		return ceremony, sessionData, err
	}
	if err := json.Unmarshal(ceremony.SessionData, &sessionData); err != nil {
		return ceremony, sessionData, fmt.Errorf("ceremony %s: %w", challenge, err)
	}
	return ceremony, sessionData, nil
}

func ceremonyError(w http.ResponseWriter, err error) {
	if errors.Is(err, errNoCeremony) {
		http.Error(w, "that challenge is unknown, used or expired; start again", http.StatusUnauthorized)
		return
	}
	serverError(w, err)
}

// sha256Of hashes a secret (an Enrollment Code or a session token) for storing.
func sha256Of(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}
