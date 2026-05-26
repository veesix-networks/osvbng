// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Named signature schemes the dispatch table maps over. Future schemes
// (sigstore-keyless, in-toto attestations, etc.) are added by appending
// to DefaultVerifiers; the current scheme's semantics are frozen so
// existing signatures continue to verify under future osvbngcli builds.
const (
	// SignatureSchemeCosignV1Raw is the current scheme. Detached
	// signature at "<tarball>.sig" containing base64-encoded
	// ASN.1-DER ECDSA / RSA-PSS / Ed25519 over the tarball bytes,
	// verified against a static PEM-encoded public key.
	SignatureSchemeCosignV1Raw = "cosign-v1-raw"
)

// SignatureVerifier is the dispatch interface. Each backend knows how
// to detect its own signature artefact (HasSignature) and verify it
// against a trust anchor. The Runner consults DefaultVerifiers in
// order and uses the first that claims the tarball.
type SignatureVerifier interface {
	Scheme() string
	HasSignature(tarballPath string) bool
	Verify(tarballPath, trustAnchorPath string) error
}

// cosignV1Raw is the current-scheme backend. Looks for a `.sig`
// sidecar alongside the tarball.
type cosignV1Raw struct{}

// Scheme returns the canonical scheme name.
func (cosignV1Raw) Scheme() string { return SignatureSchemeCosignV1Raw }

// HasSignature reports whether the tarball has a `.sig` sidecar.
func (cosignV1Raw) HasSignature(tarballPath string) bool {
	_, err := os.Stat(tarballPath + ".sig")
	return err == nil
}

// Verify checks the `.sig` against the trust anchor.
func (cosignV1Raw) Verify(tarballPath, trustAnchorPath string) error {
	return VerifyBlobSignature(tarballPath, tarballPath+".sig", trustAnchorPath)
}

// DefaultVerifiers is the ordered dispatch table. The first verifier
// whose HasSignature returns true claims the tarball. Append new
// backends at the END so existing tarballs (which only ship the v1
// `.sig` artefact) continue to dispatch correctly.
var DefaultVerifiers = []SignatureVerifier{
	cosignV1Raw{},
}

// VerifyTarballSignature is the entrypoint the Runner calls. Iterates
// DefaultVerifiers, finds the first that claims the tarball, and
// invokes its Verify method. Returns the scheme name that succeeded
// so the apply flow can journal which trust path was used.
//
// An unsigned tarball (no verifier claims it) is refused with a clear
// error — explicit refusal of unsigned tarballs is the load-bearing
// safety property of the upgrade flow.
func VerifyTarballSignature(tarballPath, trustAnchorPath string) (string, error) {
	for _, v := range DefaultVerifiers {
		if !v.HasSignature(tarballPath) {
			continue
		}
		if err := v.Verify(tarballPath, trustAnchorPath); err != nil {
			return v.Scheme(), fmt.Errorf("verify %s: %w", v.Scheme(), err)
		}
		return v.Scheme(), nil
	}
	return "", errors.New("tarball is unsigned (no recognised signature sidecar found alongside it)")
}

// VerifyBlobSignature verifies a detached signature against a blob using
// the supplied PEM-encoded public key. The signature file is expected to
// hold the base64-encoded signature (the format `cosign sign-blob`
// emits to stdout / a sidecar file). Supports ECDSA-P256, RSA-PSS, and
// Ed25519 keys — the signing algorithm is inferred from the key type.
//
// blobPath:      path to the blob whose signature is being verified
// signaturePath: path to the detached signature file (base64-encoded)
// publicKeyPath: path to the PEM-encoded public key (SubjectPublicKeyInfo)
//
// Returns nil if verification succeeds. All errors are wrapped with
// context so the apply flow can surface a clear distinction between
// "key missing", "signature malformed", and "signature does not match".
func VerifyBlobSignature(blobPath, signaturePath, publicKeyPath string) error {
	pub, err := loadPublicKey(publicKeyPath)
	if err != nil {
		return err
	}

	sigBytes, err := loadSignature(signaturePath)
	if err != nil {
		return err
	}

	digest, err := hashFile(blobPath)
	if err != nil {
		return fmt.Errorf("hash blob %s: %w", blobPath, err)
	}

	switch key := pub.(type) {
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(key, digest, sigBytes) {
			return errors.New("signature verification failed: ECDSA signature does not match blob")
		}
	case *rsa.PublicKey:
		if err := rsa.VerifyPSS(key, crypto.SHA256, digest, sigBytes, nil); err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
	case ed25519.PublicKey:
		if !ed25519.Verify(key, digest, sigBytes) {
			return errors.New("signature verification failed: Ed25519 signature does not match blob")
		}
	default:
		return fmt.Errorf("unsupported public key type %T", pub)
	}
	return nil
}

// loadPublicKey reads a PEM-encoded public key from disk and parses it.
// Accepts the standard `PUBLIC KEY` block emitted by `cosign sign-blob`
// (SubjectPublicKeyInfo format).
func loadPublicKey(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key %s: %w", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("public key %s: no PEM block found", path)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key %s: %w", path, err)
	}
	return pub, nil
}

// loadSignature reads a detached signature file. The file content is
// base64-decoded; surrounding whitespace and trailing newlines are
// tolerated since `cosign sign-blob > sig` appends a newline.
func loadSignature(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read signature %s: %w", path, err)
	}
	trimmed := strings.TrimSpace(string(raw))
	sig, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("decode signature %s as base64: %w", path, err)
	}
	if len(sig) == 0 {
		return nil, fmt.Errorf("signature %s is empty after decode", path)
	}
	return sig, nil
}

func hashFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
