// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// signingFixture mirrors what `cosign sign-blob --key cosign.key blob`
// emits: an ECDSA-P256-SHA256 signature over the blob, ASN.1-DER
// encoded, then base64-encoded.
type signingFixture struct {
	pubKeyPath    string
	signaturePath string
	blobPath      string
	priv          *ecdsa.PrivateKey
}

func newSigningFixture(t *testing.T, blobContent []byte) *signingFixture {
	t.Helper()
	dir := t.TempDir()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	pubKeyPath := filepath.Join(dir, "cosign.pub")
	if err := os.WriteFile(pubKeyPath, pubPEM, 0o644); err != nil {
		t.Fatalf("write pub key: %v", err)
	}

	blobPath := filepath.Join(dir, "blob.tar.gz")
	if err := os.WriteFile(blobPath, blobContent, 0o644); err != nil {
		t.Fatalf("write blob: %v", err)
	}

	digest := sha256.Sum256(blobContent)
	sigDER, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	signaturePath := filepath.Join(dir, "blob.tar.gz.sig")
	if err := os.WriteFile(signaturePath, []byte(base64.StdEncoding.EncodeToString(sigDER)+"\n"), 0o644); err != nil {
		t.Fatalf("write signature: %v", err)
	}

	return &signingFixture{
		pubKeyPath:    pubKeyPath,
		signaturePath: signaturePath,
		blobPath:      blobPath,
		priv:          priv,
	}
}

func TestVerifyBlobSignatureHappyPath(t *testing.T) {
	fx := newSigningFixture(t, []byte("hello, tier A"))

	if err := VerifyBlobSignature(fx.blobPath, fx.signaturePath, fx.pubKeyPath); err != nil {
		t.Fatalf("VerifyBlobSignature: %v", err)
	}
}

func TestVerifyBlobSignatureRejectsTamperedBlob(t *testing.T) {
	fx := newSigningFixture(t, []byte("hello, tier A"))

	if err := os.WriteFile(fx.blobPath, []byte("TAMPERED"), 0o644); err != nil {
		t.Fatalf("rewrite blob: %v", err)
	}

	err := VerifyBlobSignature(fx.blobPath, fx.signaturePath, fx.pubKeyPath)
	if err == nil {
		t.Fatal("VerifyBlobSignature accepted tampered blob")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Fatalf("error did not mention signature: %v", err)
	}
}

func TestVerifyBlobSignatureRejectsWrongPublicKey(t *testing.T) {
	fx := newSigningFixture(t, []byte("hello"))

	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}
	otherDER, _ := x509.MarshalPKIXPublicKey(&other.PublicKey)
	otherPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: otherDER})
	if err := os.WriteFile(fx.pubKeyPath, otherPEM, 0o644); err != nil {
		t.Fatalf("rewrite pub key: %v", err)
	}

	err = VerifyBlobSignature(fx.blobPath, fx.signaturePath, fx.pubKeyPath)
	if err == nil {
		t.Fatal("VerifyBlobSignature accepted signature under a different key")
	}
}

func TestVerifyBlobSignatureMissingKeyFile(t *testing.T) {
	fx := newSigningFixture(t, []byte("hi"))

	err := VerifyBlobSignature(fx.blobPath, fx.signaturePath, fx.pubKeyPath+".missing")
	if err == nil {
		t.Fatal("VerifyBlobSignature accepted missing public key")
	}
	if !strings.Contains(err.Error(), "read public key") {
		t.Fatalf("error did not flag missing key: %v", err)
	}
}

func TestVerifyBlobSignatureMissingSignatureFile(t *testing.T) {
	fx := newSigningFixture(t, []byte("hi"))

	err := VerifyBlobSignature(fx.blobPath, fx.signaturePath+".missing", fx.pubKeyPath)
	if err == nil {
		t.Fatal("VerifyBlobSignature accepted missing signature file")
	}
	if !strings.Contains(err.Error(), "read signature") {
		t.Fatalf("error did not flag missing signature: %v", err)
	}
}

func TestVerifyBlobSignatureMalformedSignature(t *testing.T) {
	fx := newSigningFixture(t, []byte("hi"))

	if err := os.WriteFile(fx.signaturePath, []byte("not base64 !@#$"), 0o644); err != nil {
		t.Fatalf("write bad sig: %v", err)
	}
	err := VerifyBlobSignature(fx.blobPath, fx.signaturePath, fx.pubKeyPath)
	if err == nil {
		t.Fatal("VerifyBlobSignature accepted malformed base64")
	}
	if !strings.Contains(err.Error(), "base64") {
		t.Fatalf("error did not mention base64: %v", err)
	}
}

func TestVerifyTarballSignatureRoutesToV1Raw(t *testing.T) {
	fx := newSigningFixture(t, []byte("hello"))

	scheme, err := VerifyTarballSignature(fx.blobPath, fx.pubKeyPath)
	if err != nil {
		t.Fatalf("VerifyTarballSignature: %v", err)
	}
	if scheme != SignatureSchemeCosignV1Raw {
		t.Fatalf("scheme = %q, want %q", scheme, SignatureSchemeCosignV1Raw)
	}
}

func TestVerifyTarballSignatureRefusesUnsigned(t *testing.T) {
	fx := newSigningFixture(t, []byte("hello"))
	if err := os.Remove(fx.signaturePath); err != nil {
		t.Fatalf("remove sig: %v", err)
	}

	scheme, err := VerifyTarballSignature(fx.blobPath, fx.pubKeyPath)
	if err == nil {
		t.Fatal("VerifyTarballSignature accepted unsigned tarball")
	}
	if scheme != "" {
		t.Fatalf("scheme = %q, want \"\" for unsigned", scheme)
	}
	if !strings.Contains(err.Error(), "unsigned") {
		t.Fatalf("error did not mention unsigned: %v", err)
	}
}

func TestVerifyTarballSignatureRefusesTamperedBlobViaDispatch(t *testing.T) {
	fx := newSigningFixture(t, []byte("hello"))
	if err := os.WriteFile(fx.blobPath, []byte("TAMPERED"), 0o644); err != nil {
		t.Fatalf("rewrite blob: %v", err)
	}

	scheme, err := VerifyTarballSignature(fx.blobPath, fx.pubKeyPath)
	if err == nil {
		t.Fatal("VerifyTarballSignature accepted tampered tarball")
	}
	// Even on verification failure the dispatch returns the scheme
	// that ATTEMPTED to claim the tarball — useful for journal output.
	if scheme != SignatureSchemeCosignV1Raw {
		t.Fatalf("scheme = %q, want %q (the claiming backend)", scheme, SignatureSchemeCosignV1Raw)
	}
}

func TestCosignV1RawHasSignatureDetectsSidecar(t *testing.T) {
	dir := t.TempDir()
	blob := dir + "/tarball.tar.gz"
	if err := os.WriteFile(blob, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blob: %v", err)
	}

	v := cosignV1Raw{}
	if v.HasSignature(blob) {
		t.Fatal("HasSignature returned true with no sidecar")
	}
	if err := os.WriteFile(blob+".sig", []byte("y"), 0o644); err != nil {
		t.Fatalf("write sig: %v", err)
	}
	if !v.HasSignature(blob) {
		t.Fatal("HasSignature returned false with sidecar present")
	}
}

func TestVerifyBlobSignatureMalformedPublicKey(t *testing.T) {
	fx := newSigningFixture(t, []byte("hi"))

	if err := os.WriteFile(fx.pubKeyPath, []byte("not a PEM block"), 0o644); err != nil {
		t.Fatalf("write bad pub key: %v", err)
	}
	err := VerifyBlobSignature(fx.blobPath, fx.signaturePath, fx.pubKeyPath)
	if err == nil {
		t.Fatal("VerifyBlobSignature accepted malformed PEM")
	}
	if !strings.Contains(err.Error(), "PEM") {
		t.Fatalf("error did not mention PEM: %v", err)
	}
}
