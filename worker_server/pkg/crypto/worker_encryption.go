package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	// Global encryption instance
	globalEncryptor *Encryptor
	once            sync.Once

	// Errors
	ErrInvalidKey        = errors.New("encryption key must be 32 bytes")
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrDecryptionFailed  = errors.New("decryption failed")
)

// Encryptor handles AES-256-GCM encryption/decryption
type Encryptor struct {
	key []byte
	gcm cipher.AEAD
	mu  sync.RWMutex
}

// NewEncryptor creates a new encryptor with the given key
func NewEncryptor(key []byte) (*Encryptor, error) {
	// Key must be 32 bytes for AES-256
	if len(key) != 32 {
		// If key is not 32 bytes, derive a 32-byte key using SHA-256
		hash := sha256.Sum256(key)
		key = hash[:]
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &Encryptor{
		key: key,
		gcm: gcm,
	}, nil
}

// Init initializes the global encryptor using ENCRYPTION_KEY env var
func Init() error {
	var initErr error
	once.Do(func() {
		key := os.Getenv("ENCRYPTION_KEY")
		if key == "" {
			// Fall back to JWT secret if encryption key not set
			key = os.Getenv("SUPABASE_JWT_SECRET")
		}
		if key == "" {
			initErr = errors.New("ENCRYPTION_KEY or SUPABASE_JWT_SECRET must be set")
			return
		}

		enc, err := NewEncryptor([]byte(key))
		if err != nil {
			initErr = err
			return
		}
		globalEncryptor = enc
	})
	return initErr
}

// GetEncryptor returns the global encryptor instance
func GetEncryptor() *Encryptor {
	return globalEncryptor
}

// Encrypt encrypts plaintext and returns base64-encoded ciphertext
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// Generate random nonce
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt data
	ciphertext := e.gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Return base64-encoded ciphertext
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded ciphertext
func (e *Encryptor) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// Decode base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	// Validate ciphertext length
	nonceSize := e.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrInvalidCiphertext
	}

	// Extract nonce and ciphertext
	nonce, encrypted := data[:nonceSize], data[nonceSize:]

	// Decrypt
	plaintext, err := e.gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}

// EncryptToken encrypts an OAuth token
func (e *Encryptor) EncryptToken(token string) (string, error) {
	return e.Encrypt(token)
}

// DecryptToken decrypts an OAuth token
func (e *Encryptor) DecryptToken(encryptedToken string) (string, error) {
	return e.Decrypt(encryptedToken)
}

// Global convenience functions

// Encrypt encrypts using the global encryptor
func Encrypt(plaintext string) (string, error) {
	if globalEncryptor == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}
	return globalEncryptor.Encrypt(plaintext)
}

// Decrypt decrypts using the global encryptor
func Decrypt(ciphertext string) (string, error) {
	if globalEncryptor == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}
	return globalEncryptor.Decrypt(ciphertext)
}

// EncryptToken encrypts an OAuth token using the global encryptor
func EncryptToken(token string) (string, error) {
	return Encrypt(token)
}

// DecryptToken decrypts an OAuth token using the global encryptor
func DecryptToken(encryptedToken string) (string, error) {
	return Decrypt(encryptedToken)
}

// IsEncrypted checks if a string appears to be encrypted (base64 with proper length)
func IsEncrypted(s string) bool {
	if s == "" {
		return false
	}

	// Try to decode as base64
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return false
	}

	// Minimum length: nonce (12 bytes) + tag (16 bytes) = 28 bytes
	return len(decoded) >= 28
}
