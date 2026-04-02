package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type EncryptedPayload struct {
	Nonce  string `json:"nonce"`
	Cipher string `json:"cipher"`
	KeyID  string `json:"key_id,omitempty"`
}

type AESGCMEncryptor struct {
	key []byte
}

func NewAESGCMEncryptor(key []byte) (*AESGCMEncryptor, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes (256 bits)")
	}
	return &AESGCMEncryptor{key: key}, nil
}

func NewAESGCMEncryptorFromPassword(password string, salt []byte) (*AESGCMEncryptor, error) {
	key := deriveKey(password, salt)
	return NewAESGCMEncryptor(key)
}

func deriveKey(password string, salt []byte) []byte {
	h := sha256.New()
	h.Write([]byte(password))
	h.Write(salt)
	return h.Sum(nil)
}

func (e *AESGCMEncryptor) Encrypt(plaintext []byte) (*EncryptedPayload, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	return &EncryptedPayload{
		Nonce:  base64.StdEncoding.EncodeToString(nonce),
		Cipher: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

func (e *AESGCMEncryptor) Decrypt(payload *EncryptedPayload) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(payload.Nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(payload.Cipher)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

func (e *AESGCMEncryptor) EncryptJSON(v interface{}) (*EncryptedPayload, error) {
	plaintext, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return e.Encrypt(plaintext)
}

func (e *AESGCMEncryptor) DecryptJSON(payload *EncryptedPayload, v interface{}) error {
	plaintext, err := e.Decrypt(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(plaintext, v)
}

func GenerateRandomKey(bits int) ([]byte, error) {
	keyLen := bits / 8
	if keyLen != 16 && keyLen != 24 && keyLen != 32 {
		return nil, errors.New("invalid key size: must be 128, 192, or 256 bits")
	}

	key := make([]byte, keyLen)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	return key, nil
}

func GenerateSalt() ([]byte, error) {
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

func KeyToBase64(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

func KeyFromBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

func GenerateKeyID() string {
	b := make([]byte, 8)
	io.ReadFull(rand.Reader, b)
	return fmt.Sprintf("key-%x", b)
}
