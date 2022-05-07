package util

import (
	"testing"
)

func TestUtil_RSA(t *testing.T) {
	prk, puk, _ := GenerateRSAKeyPair()
	plaintext := "hello darkness my old friend RSA"
	cyphertext := EncodeAndEncryptRSA(ConvertPublicKeyToBytes(puk), plaintext)
	plaintextback := ""
	DecodeAndDecryptRSA(prk, cyphertext, &plaintextback)
	if plaintext != plaintextback {
		t.Fatalf("Initial message '%s' differs from decrypted '%s'", plaintext, plaintextback)
	}
}

func TestUtil_AES(t *testing.T) {
	key := GenerateAESKey()
	plaintext := "hello darkness my old friend AES"
	cyphertext := EncodeAndEncryptAES(key, plaintext)
	plaintextback := ""
	DecodeAndDecryptAES(key, cyphertext, &plaintextback)
	if plaintext != plaintextback {
		t.Fatalf("Initial message '%s' differs from decrypted '%s'", plaintext, plaintextback)
	}
}
