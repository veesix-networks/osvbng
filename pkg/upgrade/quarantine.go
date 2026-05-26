// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Quarantine copies an offending tarball plus a reason sidecar into
// /var/opt/osvbng/quarantine/<sha-prefix>/ for operator forensics. Used
// when signature verification or post-extraction sha256 cross-check
// rejects a tarball — we keep the bytes that triggered the reject so
// the operator can investigate without having to re-download.
//
// quarantineRoot is the parent directory (caller ensures it exists,
// typically /var/opt/osvbng/quarantine). tarballPath is the rejected
// tarball; reason is the operator-facing explanation that ends up in
// the .reason sidecar.
//
// The returned path is the directory under quarantineRoot holding the
// copied tarball and sidecar. Errors from the copy are returned but do
// not unwind the original rejection — the apply flow reports both.
func Quarantine(quarantineRoot, tarballPath, reason string) (string, error) {
	hash, err := sha256File(tarballPath)
	if err != nil {
		return "", fmt.Errorf("sha256 of %s: %w", tarballPath, err)
	}
	prefix := hash[:12]
	stamp := time.Now().UTC().Format("20060102T150405Z")
	dirName := fmt.Sprintf("%s-%s", stamp, prefix)
	target := filepath.Join(quarantineRoot, dirName)

	if err := os.MkdirAll(target, 0o750); err != nil {
		return "", fmt.Errorf("mkdir quarantine %s: %w", target, err)
	}

	dstTarball := filepath.Join(target, filepath.Base(tarballPath))
	if err := copyFile(tarballPath, dstTarball); err != nil {
		return target, fmt.Errorf("copy %s -> %s: %w", tarballPath, dstTarball, err)
	}

	reasonPath := filepath.Join(target, filepath.Base(tarballPath)+".reason")
	body := fmt.Sprintf("Quarantined: %s\nReason: %s\nOriginal path: %s\nSHA256: %s\n",
		stamp, reason, tarballPath, hash)
	if err := os.WriteFile(reasonPath, []byte(body), 0o640); err != nil {
		return target, fmt.Errorf("write reason sidecar: %w", err)
	}

	return target, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
