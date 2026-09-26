package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Claims are the JWT access token claims.
// sub (RegisteredClaims.Subject) carries the user_id.
// No refresh token, username, or email is ever placed in the payload.
type Claims struct {
	ClientID  string `json:"client_id"`
	SessionID string `json:"session_id"`
	jwt.RegisteredClaims
}

const (
	jwtIssuer         = "auth-service"
	accessTokenExpiry = 30 * time.Minute
)

var (
	keyOnce    sync.Once
	signingKey *rsa.PrivateKey
	keyErr     error
)

// getSigningKey loads the RS256 private key.
//   - JWT_PRIVATE_KEY_FILE set and file exists: load PEM (PKCS#1 or PKCS#8).
//   - JWT_PRIVATE_KEY_FILE set but file missing: generate a 2048-bit dev key
//     and persist it to that path (0600).
//   - JWT_PRIVATE_KEY_FILE unset: generate an ephemeral in-memory dev key
//     (tokens do not survive a restart) and log a warning.
func getSigningKey() (*rsa.PrivateKey, error) {
	keyOnce.Do(func() {
		path := os.Getenv("JWT_PRIVATE_KEY_FILE")
		if path == "" {
			log.Printf("WARNING: JWT_PRIVATE_KEY_FILE not set; generating ephemeral dev signing key (tokens will not survive restart)")
			signingKey, keyErr = rsa.GenerateKey(rand.Reader, 2048)
			return
		}

		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			key, gerr := rsa.GenerateKey(rand.Reader, 2048)
			if gerr != nil {
				keyErr = gerr
				return
			}
			pemBytes := pem.EncodeToMemory(&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(key),
			})
			if werr := os.WriteFile(path, pemBytes, 0o600); werr != nil {
				keyErr = werr
				return
			}
			log.Printf("Generated new dev RSA signing key at configured JWT_PRIVATE_KEY_FILE path")
			signingKey = key
			return
		}
		if err != nil {
			keyErr = err
			return
		}

		block, _ := pem.Decode(data)
		if block == nil {
			keyErr = fmt.Errorf("JWT_PRIVATE_KEY_FILE does not contain a PEM block")
			return
		}
		if key, perr := x509.ParsePKCS1PrivateKey(block.Bytes); perr == nil {
			signingKey = key
			return
		}
		parsed, perr := x509.ParsePKCS8PrivateKey(block.Bytes)
		if perr != nil {
			keyErr = fmt.Errorf("failed to parse private key: %w", perr)
			return
		}
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			keyErr = fmt.Errorf("JWT_PRIVATE_KEY_FILE is not an RSA private key")
			return
		}
		signingKey = rsaKey
	})
	return signingKey, keyErr
}

func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateJWTToken issues an RS256-signed access token with claims:
// sub (user_id), client_id, session_id, iat, exp, iss. 30-minute lifetime.
func GenerateJWTToken(userID, clientID, sessionID string) (string, time.Time, error) {
	key, err := getSigningKey()
	if err != nil {
		return "", time.Time{}, err
	}

	now := time.Now()
	expirationTime := now.Add(accessTokenExpiry)
	claims := &Claims{
		ClientID:  clientID,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    jwtIssuer,
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(key)
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expirationTime, nil
}

// ValidateJWTToken verifies the RS256 signature, expiry, and issuer.
func ValidateJWTToken(tokenString string) (*Claims, error) {
	key, err := getSigningKey()
	if err != nil {
		return nil, err
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return &key.PublicKey, nil
	}, jwt.WithIssuer(jwtIssuer))

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	if claims.Subject == "" || claims.ClientID == "" || claims.SessionID == "" {
		return nil, fmt.Errorf("missing required claims")
	}

	return claims, nil
}

// Refresh token format: "<session_id>.<secret>" where secret is 256 bits of
// randomness (hex). The session_id prefix is the indexed lookup key (session
// primary key); only a bcrypt hash of the secret half is stored on the
// session row. Presenting a well-formed token whose secret does not match
// the stored hash (i.e. a rotated/stale token) is treated as a theft signal.

// GenerateRefreshToken returns the opaque token to hand to the client and
// the bcrypt hash of its secret half for storage.
func GenerateRefreshToken(sessionID string) (token string, secretHash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	secret := hex.EncodeToString(bytes)
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	return sessionID + "." + secret, string(hash), nil
}

// ParseRefreshToken splits an opaque refresh token into its session_id
// lookup key and secret.
func ParseRefreshToken(token string) (sessionID, secret string, ok bool) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// VerifyRefreshSecret compares a presented refresh secret against the stored
// bcrypt hash (bcrypt comparison is constant-time).
func VerifyRefreshSecret(secret, secretHash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(secretHash), []byte(secret)) == nil
}

// CheckAdminSecret constant-time-compares the provided secret against the
// ADMIN_SECRET env var. An unset ADMIN_SECRET always fails (fail closed).
func CheckAdminSecret(provided string) bool {
	want := os.Getenv("ADMIN_SECRET")
	if want == "" || provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(want)) == 1
}

func GenerateUUID() string {
	return uuid.NewString()
}

func GenerateClientSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
