package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

const (
	enrollmentCodeLifetime = 15 * time.Minute
	// ceremonyTimeout is how long the Owner has to answer the Face ID prompt.
	ceremonyTimeout = 5 * time.Minute
	// enrollmentCodeAlphabet has 32 symbols, leaving out I, L, O and U, which
	// are easily misread.
	enrollmentCodeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	// signInRefused is the answer to every refused passkey, so a copied key
	// isn't told what gave it away.
	signInRefused = "that passkey is unknown or could not be verified"
)

// IdentityService signs the Owner in under /auth/: registering a passkey
// with an Enrollment Code, and signing in with a passkey. Both end by having
// the SessionsService start a session.
type IdentityService struct {
	enrollmentCodes model.EnrollmentCodesRepository
	ceremonies      model.CeremoniesRepository
	passkeys        model.PasskeysRepository
	sessions        *SessionsService
	webauthn        *webauthn.WebAuthn
	now             func() time.Time
}

// NewIdentity returns the IdentityService for passkeys of domain, used from
// origins, with now as its clock.
func NewIdentity(
	enrollmentCodes model.EnrollmentCodesRepository,
	ceremonies model.CeremoniesRepository,
	passkeys model.PasskeysRepository,
	sessions *SessionsService,
	domain string,
	origins []string,
	now func() time.Time,
) (*IdentityService, error) {
	relyingParty, err := webauthn.New(&webauthn.Config{
		RPID:          domain,
		RPDisplayName: "Nando",
		RPOrigins:     origins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTimeout},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTimeout},
		},
	})
	if err != nil {
		return nil, err
	}
	return &IdentityService{
		enrollmentCodes: enrollmentCodes,
		ceremonies:      ceremonies,
		passkeys:        passkeys,
		sessions:        sessions,
		webauthn:        relyingParty,
		now:             now,
	}, nil
}

// Routes adds the signing-in routes to mux, under /auth/.
func (s *IdentityService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register/begin", s.beginRegistration)
	mux.HandleFunc("POST /auth/register/finish", s.finishRegistration)
	mux.HandleFunc("POST /auth/signin/begin", s.beginSignIn)
	mux.HandleFunc("POST /auth/signin/finish", s.finishSignIn)
}

// IssueEnrollmentCode stores and returns a new Enrollment Code.
func (s *IdentityService) IssueEnrollmentCode(ctx context.Context) (string, error) {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = enrollmentCodeAlphabet[int(b[i])%len(enrollmentCodeAlphabet)]
	}
	code := string(b[:5]) + "-" + string(b[5:])
	err := s.enrollmentCodes.Insert(ctx, model.EnrollmentCode{
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
func (s *IdentityService) beginRegistration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `body must be {"code": "..."}`, http.StatusBadRequest)
		return
	}
	codeHash := sha256Of(req.Code)
	if _, err := s.enrollmentCodes.Find(r.Context(), codeHash, s.now()); err != nil {
		enrollmentCodeError(w, err)
		return
	}

	// Excluding the Owner's passkeys stops a device that already has one from
	// silently replacing it, which would leave the old one listed but unusable.
	o, err := s.loadOwner(r.Context())
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	exclusions := webauthn.Credentials(o.credentials).CredentialDescriptors()
	creation, sessionData, err := s.webauthn.BeginRegistration(o, webauthn.WithExclusions(exclusions))
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	if err := s.beginCeremony(r.Context(), sessionData, codeHash); err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, creation)
}

// finishRegistration stores the new passkey from navigator.credentials.create,
// uses up the Enrollment Code and signs this device in.
func (s *IdentityService) finishRegistration(w http.ResponseWriter, r *http.Request) {
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
		utils.ServerError(w, err)
		return
	}
	err = s.passkeys.Insert(r.Context(), model.Passkey{ID: credential.ID, Credential: stored, CreatedAt: s.now()})
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	// The code is used up only once the passkey is stored, so a cancelled
	// Face ID prompt or a failed insert doesn't waste it. If another
	// registration used it first, this passkey is taken back out.
	if err := s.enrollmentCodes.Use(r.Context(), ceremony.EnrollmentCodeHash, s.now()); err != nil {
		if deleteErr := s.passkeys.Delete(r.Context(), credential.ID); deleteErr != nil {
			utils.ServerError(w, errors.Join(err, deleteErr))
			return
		}
		enrollmentCodeError(w, err)
		return
	}
	if err := s.sessions.startSession(w, r); err != nil {
		utils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// beginSignIn answers with the options for navigator.credentials.get, letting
// the Owner pick any of their passkeys.
func (s *IdentityService) beginSignIn(w http.ResponseWriter, r *http.Request) {
	assertion, sessionData, err := s.webauthn.BeginDiscoverableLogin()
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	if err := s.beginCeremony(r.Context(), sessionData, nil); err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, assertion)
}

// finishSignIn checks the passkey's answer from navigator.credentials.get,
// saves its new sign count and signs this device in.
func (s *IdentityService) finishSignIn(w http.ResponseWriter, r *http.Request) {
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
	var passkey model.Passkey
	_, credential, err := s.webauthn.ValidatePasskeyLogin(s.findOwner(r.Context(), &passkey), sessionData, parsed)
	if err != nil {
		http.Error(w, signInRefused, http.StatusUnauthorized)
		return
	}
	// A sign count that didn't go up means another copy of the passkey may
	// exist. Synced passkeys always report 0, which never sets this, so it
	// guards hardware security keys.
	if credential.Authenticator.CloneWarning {
		log.Printf("sign-in refused: passkey %x reported sign count %d, not above the stored %d; it may have been copied",
			credential.ID, parsed.Response.AuthenticatorData.Counter, credential.Authenticator.SignCount)
		http.Error(w, signInRefused, http.StatusUnauthorized)
		return
	}
	stored, err := json.Marshal(credential)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	// Saved only if unchanged since it was read: of two sign-ins checked
	// against the same sign count, only the first counts, and a passkey
	// deleted meanwhile can't sign in.
	err = s.passkeys.Update(r.Context(), passkey.ID, passkey.Credential, stored)
	if errors.Is(err, utils.ErrNotFound) {
		log.Printf("sign-in refused: passkey %x was deleted or signed in again meanwhile", passkey.ID)
		http.Error(w, signInRefused, http.StatusUnauthorized)
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	if err := s.sessions.startSession(w, r); err != nil {
		utils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// findOwner finds the Owner by the passkey they signed in with, keeping that
// passkey as it was stored in found.
func (s *IdentityService) findOwner(ctx context.Context, found *model.Passkey) webauthn.DiscoverableUserHandler {
	return func(rawID, userHandle []byte) (webauthn.User, error) {
		passkey, err := s.passkeys.Find(ctx, rawID)
		if err != nil {
			return nil, err
		}
		*found = passkey
		credential, err := credentialOf(passkey)
		if err != nil {
			return nil, err
		}
		return owner{credentials: []webauthn.Credential{credential}}, nil
	}
}

// loadOwner returns the Owner with all their passkeys.
func (s *IdentityService) loadOwner(ctx context.Context) (owner, error) {
	passkeys, err := s.passkeys.List(ctx)
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

func enrollmentCodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, utils.ErrNotFound) {
		http.Error(w, "that Enrollment Code is unknown, used or expired", http.StatusUnauthorized)
		return
	}
	utils.ServerError(w, err)
}

// beginCeremony keeps sessionData, go-webauthn's state, until the ceremony's finish request.
func (s *IdentityService) beginCeremony(ctx context.Context, sessionData *webauthn.SessionData, enrollmentCodeHash []byte) error {
	stored, err := json.Marshal(sessionData)
	if err != nil {
		return err
	}
	return s.ceremonies.Insert(ctx, model.Ceremony{
		Challenge:          sessionData.Challenge,
		SessionData:        stored,
		EnrollmentCodeHash: enrollmentCodeHash,
		ExpiresAt:          s.now().Add(ceremonyTimeout),
	})
}

// errNoCeremony means a finish request has no begun, unfinished ceremony.
var errNoCeremony = errors.New("no ceremony")

// takeCeremony ends the ceremony with this challenge, so it can finish only once.
func (s *IdentityService) takeCeremony(ctx context.Context, challenge string) (model.Ceremony, webauthn.SessionData, error) {
	var sessionData webauthn.SessionData
	ceremony, err := s.ceremonies.Take(ctx, challenge, s.now())
	if errors.Is(err, utils.ErrNotFound) {
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
	utils.ServerError(w, err)
}

// sha256Of hashes a secret (an Enrollment Code or a session token) for storing.
func sha256Of(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}
