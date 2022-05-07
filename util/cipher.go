package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
)

func GenerateRSAKeys(bits int) (*rsa.PublicKey, *rsa.PrivateKey) {
	prk, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error from generating RSA keys: %s\n", err)
		return nil, nil
	}

	return &prk.PublicKey, prk
}

func GenerateAESKey() []byte {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		fmt.Fprintf(os.Stderr, "Error from generating AES key: %s\n", err)
	}
	return key
}

func EncryptRSAPublic(puk *rsa.PublicKey, msg []byte) []byte {
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, puk, msg, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error from encryption: %s\n", err)
		return nil
	}
	return ciphertext
}

func EncryptAES(key []byte, msg []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error from encryption: %s\n", err)
		return nil
	}

	// iv := make([]byte, aes.BlockSize)
	// ciphertext := make([]byte, len(msg))
	// if _, err := io.ReadFull(rand.Reader, iv); err != nil {
	// 	fmt.Fprintf(os.Stderr, "Error from encryption: %s\n", err)
	// 	return nil
	// }

	// stream := cipher.NewCFBEncrypter(block, iv)
	// stream.XORKeyStream(ciphertext, msg)

	// https://pkg.go.dev/crypto/cipher@go1.18#example-NewCFBEncrypter
	ciphertext := make([]byte, aes.BlockSize+len(msg))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		fmt.Fprintf(os.Stderr, "Error from encryption: %s\n", err)
		return nil
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], msg)
	return ciphertext
}

func DecryptAES(key []byte, ciphertext []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error from decryption: %s\n", err)
		return nil
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	plaintext := make([]byte, len(ciphertext))

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(plaintext, ciphertext)
	return plaintext
}

func DecryptRSAPrivate(prk *rsa.PrivateKey, ciphertext []byte) []byte {
	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, prk, ciphertext, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error from decryption: %s\n", err)
		return nil
	}
	return plaintext
}

func ConvertPublicKeyToBytes(puk *rsa.PublicKey) []byte {
	b := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: x509.MarshalPKCS1PublicKey(puk),
		},
	)
	return b
}

func ConvertBytesToPublicKey(key []byte) *rsa.PublicKey {
	block, _ := pem.Decode(key)
	b, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error from converting bytes to public key: %s\n", err)
		return nil
	}
	return b
}

func EncodeAndEncryptRSA(puk []byte, payload interface{}) []byte {
	b := Encode(payload)
	k := ConvertBytesToPublicKey(puk)
	b = EncryptRSAPublic(k, b)
	return b
}

// res needs to be a pointer
func DecodeAndDecryptRSA(prk *rsa.PrivateKey, ciphertext []byte, res interface{}) {
	p := DecryptRSAPrivate(prk, ciphertext)
	Decode(p, res)
}

// res needs to be a pointer
func DecodeAndDecryptAES(key []byte, ciphertext []byte, res interface{}) {
	p := DecryptAES(key, ciphertext)
	Decode(p, res)
}

func EncodeAndEncryptAES(k []byte, payload interface{}) []byte {
	b := Encode(payload)
	b = EncryptAES(k, b)
	return b
}
