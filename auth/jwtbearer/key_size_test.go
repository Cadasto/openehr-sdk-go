package jwtbearer_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

// publicOnlySigner is a crypto.Signer of the caller's own type, as a key
// store adapter would be, whose Public reports pub and which never signs.
type publicOnlySigner struct{ pub crypto.PublicKey }

func (p publicOnlySigner) Public() crypto.PublicKey { return p.pub }

func (publicOnlySigner) Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error) {
	return nil, errors.New("not used")
}

// rsaPublicKeyOfBits returns an RSA public key whose modulus is exactly bits
// long: 2^(bits-1) + 1. It is no real key, only a size to check.
func rsaPublicKeyOfBits(bits int) *rsa.PublicKey {
	n := new(big.Int).Lsh(big.NewInt(1), uint(bits-1))
	n.Add(n, big.NewInt(1))
	return &rsa.PublicKey{N: n, E: 65537}
}

// TestREQ068_ShortRSAKeyIsRefused pins REQ-068 (auth.md § Client-assertion
// signing algorithms and § Backend Services asymmetric client auth): an RSA
// key under 1024 bits does not fit RS256 or RS384, and its size is read from
// the signer's public key. A signer of the caller's own type reporting a
// 512-bit or a 1023-bit modulus is refused at construction with
// auth.ErrInvalidConfig, by NewClaimsSigner and NewClientAssertion alike. A
// 1023-bit modulus fills 128 bytes, so a check on the key's byte size would
// let it through. The error names the algorithm, the key's size and the
// minimum, and no part of the key.
func TestREQ068_ShortRSAKeyIsRefused(t *testing.T) { // REQ-068
	t.Parallel()
	builds := []struct {
		name  string
		build func(signer crypto.Signer, alg string) (*jwtbearer.ClaimsSigner, error)
	}{
		{name: "NewClaimsSigner", build: func(signer crypto.Signer, alg string) (*jwtbearer.ClaimsSigner, error) {
			return jwtbearer.NewClaimsSigner(jwtbearer.ClaimsTemplate{Issuer: "c1", Audience: "https://as.example/token"},
				signer, jwtbearer.WithAlgorithm(alg))
		}},
		{name: "NewClientAssertion", build: func(signer crypto.Signer, alg string) (*jwtbearer.ClaimsSigner, error) {
			return jwtbearer.NewClientAssertion("c1", "https://as.example/token", signer, alg, "k1")
		}},
	}
	for _, b := range builds {
		for _, alg := range []string{"RS256", "RS384"} {
			for _, bits := range []int{512, 1023} {
				t.Run(b.name+"/"+alg+"/"+strconv.Itoa(bits), func(t *testing.T) {
					t.Parallel()
					pub := rsaPublicKeyOfBits(bits)
					s, err := b.build(publicOnlySigner{pub: pub}, alg)
					if !errors.Is(err, auth.ErrInvalidConfig) || s != nil {
						t.Fatalf("%s with a %d-bit RSA key for %s = (%v, %v), want (nil, an error matching auth.ErrInvalidConfig)", b.name, bits, alg, s, err)
					}
					msg := err.Error()
					for _, want := range []string{alg, strconv.Itoa(bits), "1024"} {
						if !strings.Contains(msg, want) {
							t.Errorf("error %q does not name %q: it names the algorithm, the key's size and the minimum", msg, want)
						}
					}
					if strings.Contains(msg, pub.N.String()) || strings.Contains(msg, pub.N.Text(16)) {
						t.Errorf("error %q carries the key's modulus", msg)
					}
				})
			}
		}
	}
}

// TestREQ068_1024BitRSAKeyIsAccepted pins REQ-068 (auth.md § Backend Services
// asymmetric client auth): a 1024-bit RSA key, the minimum crypto/rsa signs
// with, fits RS256 and RS384, held as an *rsa.PrivateKey or behind a signer of
// the caller's own type, and the assertion it signs verifies against it.
func TestREQ068_1024BitRSAKeyIsAccepted(t *testing.T) { // REQ-068
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey(1024): %v", err)
	}
	signers := []struct {
		name   string
		signer crypto.Signer
	}{
		{name: "*rsa.PrivateKey", signer: key},
		{name: "caller's own signer", signer: wrappedSigner{key: key}},
	}
	for _, sg := range signers {
		for _, alg := range []string{"RS256", "RS384"} {
			t.Run(sg.name+"/"+alg, func(t *testing.T) {
				t.Parallel()
				s, err := jwtbearer.NewClientAssertion("c1", "https://as.example/token", sg.signer, alg, "k1")
				if err != nil {
					t.Fatalf("NewClientAssertion with a 1024-bit RSA key for %s: %v", alg, err)
				}
				jwt, err := s.Assertion(t.Context())
				if err != nil {
					t.Fatalf("Assertion with a 1024-bit RSA key for %s: %v", alg, err)
				}
				if err := verifyRS(alg, &key.PublicKey, jwt); err != nil {
					t.Errorf("the %s assertion of a 1024-bit RSA key does not verify: %v", alg, err)
				}
			})
		}
	}
}

// TestREQ068_IssueCase512BitPrivateKeyIsRefused pins REQ-068 (auth.md §
// Backend Services asymmetric client auth) on the case the issue reported: a
// 512-bit *rsa.PrivateKey given to NewClientAssertion is refused with
// auth.ErrInvalidConfig when the source is built. The key is made with the
// rsa1024min=0 GODEBUG setting that crypto/rsa keeps for tests, which stays
// set while NewClientAssertion runs: it does not lower the minimum.
func TestREQ068_IssueCase512BitPrivateKeyIsRefused(t *testing.T) { // REQ-068
	t.Setenv("GODEBUG", "rsa1024min=0")
	key, err := rsa.GenerateKey(rand.Reader, 512)
	if err != nil {
		t.Fatalf("rsa.GenerateKey(512) with rsa1024min=0: %v", err)
	}
	for _, alg := range []string{"RS256", "RS384"} {
		s, err := jwtbearer.NewClientAssertion("c1", "https://as.example/token", key, alg, "k1")
		if !errors.Is(err, auth.ErrInvalidConfig) || s != nil {
			t.Errorf("NewClientAssertion with a 512-bit *rsa.PrivateKey for %s = (%v, %v), want (nil, an error matching auth.ErrInvalidConfig)", alg, s, err)
		}
	}
}

// wrappedSigner is a crypto.Signer of the caller's own type that signs with
// key without being an *rsa.PrivateKey.
type wrappedSigner struct{ key crypto.Signer }

func (w wrappedSigner) Public() crypto.PublicKey { return w.key.Public() }

func (w wrappedSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return w.key.Sign(rand, digest, opts)
}

// verifyRS checks the RSA PKCS #1 v1.5 signature of the compact JWS jwt,
// signed with alg, RS256 or RS384, against pub.
func verifyRS(alg string, pub *rsa.PublicKey, jwt string) error {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return errors.New("not a compact JWS of three segments")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return err
	}
	input := []byte(parts[0] + "." + parts[1])
	switch alg {
	case "RS256":
		sum := sha256.Sum256(input)
		return rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig)
	case "RS384":
		sum := sha512.Sum384(input)
		return rsa.VerifyPKCS1v15(pub, crypto.SHA384, sum[:], sig)
	default:
		return errors.New("unexpected algorithm " + alg)
	}
}
