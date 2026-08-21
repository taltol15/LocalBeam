package main

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	pin := "123456"
	salt, err := newSalt()
	if err != nil {
		t.Fatal(err)
	}
	tid, err := newTransferID()
	if err != nil {
		t.Fatal(err)
	}
	key, err := deriveSessionKey(pin, salt, tid)
	if err != nil {
		t.Fatal(err)
	}

	plain := bytes.Repeat([]byte("LocalBeam Trustity Labs "), 4000)
	var enc bytes.Buffer
	n, err := encryptStream(&enc, bytes.NewReader(plain), key, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(plain)) {
		t.Fatalf("wrote %d want %d", n, len(plain))
	}

	var dec bytes.Buffer
	got, err := decryptStream(&dec, bytes.NewReader(enc.Bytes()), key, int64(len(plain)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != int64(len(plain)) || !bytes.Equal(dec.Bytes(), plain) {
		t.Fatalf("decrypt mismatch got=%d", got)
	}
}

func TestPinProof(t *testing.T) {
	proof := pinProof("654321", "nonce-abc")
	if !verifyPinProof("654321", "nonce-abc", proof) {
		t.Fatal("expected verify ok")
	}
	if verifyPinProof("000000", "nonce-abc", proof) {
		t.Fatal("expected verify fail")
	}
}
