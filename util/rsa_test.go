package util

import (
	"fmt"
	"testing"
)

func TestUtil_GenerateRSAKeyPair(t *testing.T) {
	privateKey, publicKey, err := GenerateRSAKeyPair()

	if err != nil {
		t.Fatalf("GenerateRSAKeyPair returned an error %s", err)
	}

	if privateKey == nil {
		t.Fatalf("GenerateRSAKeyPair returned invalid privateKey")
	}

	if publicKey == nil {
		t.Fatalf("GenerateRSAKeyPair returned invalid publicKey")
	}
}

func TestUtil_EncryptDecryptRSA(t *testing.T) {
	privateKey, publicKey, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair returned an error %s", err)
	}

	message := "I ate your honey cruller donut btw"
	fmt.Printf("Message to encrypt is %s\n", message)

	ciphertext, err := EncryptRSA(publicKey, message)
	if err != nil {
		t.Fatalf("EncryptRSA returned an error %s", err)
	}

	plaintext, err := DecryptRSA(privateKey, ciphertext)
	if err != nil {
		t.Fatalf("DecryptRSA returned an error %s", err)
	}

	fmt.Printf("Decrypted message is %s\n", plaintext)

	if message != plaintext {
		t.Fatalf("Initial message '%s' differs from decrypted plaintext '%s'", message, plaintext)
	}
}
