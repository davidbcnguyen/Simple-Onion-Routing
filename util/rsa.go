package util

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
)

func GenerateRSAKeyPair() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	publicKey := privateKey.PublicKey

	return privateKey, &publicKey, nil
}

func EncryptRSA(publicKey *rsa.PublicKey, content string) ([]byte, error) {
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, []byte(content), nil)
	if err != nil {
		return nil, err
	}

	return ciphertext, nil
}

func DecryptRSA(privateKey *rsa.PrivateKey, ciphertext []byte) (string, error) {
	plaintext, err := privateKey.Decrypt(nil, ciphertext, &rsa.OAEPOptions{Hash: crypto.SHA256})
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
