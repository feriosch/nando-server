package core_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// passkey is a software authenticator standing in for Face ID with iCloud
// Keychain: it answers the options the server sends the way the browser's
// navigator.credentials.create and .get would.
type passkey struct {
	id         []byte
	key        *ecdsa.PrivateKey
	userHandle []byte
}

// Authenticator data flags (WebAuthn §6.1). Synced passkeys are backup
// eligible and backed up.
const (
	flagUserPresent        = 0x01
	flagUserVerified       = 0x04
	flagBackupEligible     = 0x08
	flagBackedUp           = 0x10
	flagAttestedCredential = 0x40
	passkeyFlags           = flagUserPresent | flagUserVerified | flagBackupEligible | flagBackedUp
)

var b64 = base64.RawURLEncoding

// excludedBy reports whether the options from /auth/register/begin exclude
// this passkey, in which case the browser refuses with InvalidStateError.
func (p *passkey) excludedBy(t *testing.T, options []byte) bool {
	t.Helper()
	var opts struct {
		PublicKey struct {
			ExcludeCredentials []struct {
				ID string `json:"id"`
			} `json:"excludeCredentials"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &opts); err != nil {
		t.Fatalf("creation options %s: %v", options, err)
	}
	for _, c := range opts.PublicKey.ExcludeCredentials {
		if c.ID == b64.EncodeToString(p.id) {
			return true
		}
	}
	return false
}

// create answers the options from /auth/register/begin with a new passkey and
// returns the body for /auth/register/finish.
func (p *passkey) create(t *testing.T, options []byte) []byte {
	t.Helper()
	var opts struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			RP        struct {
				ID string `json:"id"`
			} `json:"rp"`
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &opts); err != nil {
		t.Fatalf("creation options %s: %v", options, err)
	}
	userHandle, err := b64.DecodeString(opts.PublicKey.User.ID)
	if err != nil {
		t.Fatalf("user.id %q: %v", opts.PublicKey.User.ID, err)
	}

	p.id = make([]byte, 16)
	_, _ = rand.Read(p.id)
	p.key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p.userHandle = userHandle

	ecdh, err := p.key.PublicKey.ECDH()
	if err != nil {
		t.Fatal(err)
	}
	point := ecdh.Bytes() // 0x04 || x || y
	coseKey := mustCBOR(t, map[int]any{1: 2, 3: -7, -1: 1, -2: point[1:33], -3: point[33:]})

	authData := p.authData(opts.PublicKey.RP.ID, passkeyFlags|flagAttestedCredential)
	authData = append(authData, make([]byte, 16)...) // AAGUID
	authData = binary.BigEndian.AppendUint16(authData, uint16(len(p.id)))
	authData = append(authData, p.id...)
	authData = append(authData, coseKey...)

	return mustJSON(t, map[string]any{
		"id":    b64.EncodeToString(p.id),
		"rawId": b64.EncodeToString(p.id),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON": b64.EncodeToString(clientData(t, "webauthn.create", opts.PublicKey.Challenge)),
			"attestationObject": b64.EncodeToString(mustCBOR(t, map[string]any{
				"fmt":      "none",
				"attStmt":  map[string]any{},
				"authData": authData,
			})),
			"transports": []string{"internal", "hybrid"},
		},
		"authenticatorAttachment": "platform",
		"clientExtensionResults":  map[string]any{},
	})
}

// get answers the options from /auth/signin/begin by signing in with this
// passkey and returns the body for /auth/signin/finish.
func (p *passkey) get(t *testing.T, options []byte) []byte {
	t.Helper()
	var opts struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			RPID      string `json:"rpId"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &opts); err != nil {
		t.Fatalf("request options %s: %v", options, err)
	}

	authData := p.authData(opts.PublicKey.RPID, passkeyFlags)
	cdata := clientData(t, "webauthn.get", opts.PublicKey.Challenge)
	cdataHash := sha256.Sum256(cdata)
	digest := sha256.Sum256(append(authData, cdataHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, p.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}

	return mustJSON(t, map[string]any{
		"id":    b64.EncodeToString(p.id),
		"rawId": b64.EncodeToString(p.id),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(cdata),
			"authenticatorData": b64.EncodeToString(authData),
			"signature":         b64.EncodeToString(sig),
			"userHandle":        b64.EncodeToString(p.userHandle),
		},
		"authenticatorAttachment": "platform",
		"clientExtensionResults":  map[string]any{},
	})
}

// authData starts the authenticator data: the RP ID hash, flags and a sign
// count, which synced passkeys always leave at 0.
func (p *passkey) authData(rpID string, flags byte) []byte {
	rpIDHash := sha256.Sum256([]byte(rpID))
	return append(append(rpIDHash[:], flags), 0, 0, 0, 0)
}

func clientData(t *testing.T, typ, challenge string) []byte {
	return mustJSON(t, map[string]any{
		"type":        typ,
		"challenge":   challenge,
		"origin":      webOrigin,
		"crossOrigin": false,
	})
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustCBOR(t *testing.T, v any) []byte {
	t.Helper()
	b, err := cbor.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
