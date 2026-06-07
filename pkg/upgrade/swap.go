// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// SwapArtifact atomically replaces the file at targetPath with the
// content of srcPath, setting the supplied uid/gid/mode on the new
// file BEFORE the rename(2) call. The rename is atomic within a
// filesystem: a concurrent reader sees either the old inode or the
// new — never a partial file.
//
// The write lands at <target-dir>/.<basename>.new first; on any
// failure before the rename the temp file is removed. The new file is
// fsync'd before rename to ensure the bytes are durably on disk.
//
// uid/gid/mode are honoured strictly. Passing uid=-1 or gid=-1 skips
// the chown call (leaves whatever default ownership the temp file got
// from open(2)). modeStr is the YAML-style octal string from the
// manifest (e.g. "0755"); an empty string skips the chmod and leaves
// the temp file at the default 0644.
func SwapArtifact(srcPath, targetPath string, uid, gid int, modeStr string) error {
	dir := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)
	stagingName := filepath.Join(dir, "."+base+".new")

	// Defensively ensure the parent directory exists before staging the
	// swap. The image install already pre-creates every dir under the
	// known artifact roots, but a release tarball could introduce a
	// brand-new subdirectory (e.g. templates/some-new-subdir/) that
	// the running image hasn't seen — so the runner has to create it
	// or the apply fails midway. Inherit the mode from the nearest
	// existing ancestor instead of hardcoding 0755: that way we never
	// widen permissions when extending into /etc/osvbng/ (0750) or
	// /var/opt/osvbng/ (0750). Falls back to 0755 only if no ancestor
	// exists, which shouldn't happen on a real image.
	if err := mkdirInheritMode(dir); err != nil {
		return fmt.Errorf("ensure parent dir %s: %w", dir, err)
	}

	if err := writeStagingFile(srcPath, stagingName); err != nil {
		_ = os.Remove(stagingName)
		return err
	}

	if uid >= 0 || gid >= 0 {
		uChown := uid
		if uChown < 0 {
			uChown = -1
		}
		gChown := gid
		if gChown < 0 {
			gChown = -1
		}
		if err := os.Chown(stagingName, uChown, gChown); err != nil {
			_ = os.Remove(stagingName)
			return fmt.Errorf("chown %s -> uid=%d gid=%d: %w", stagingName, uChown, gChown, err)
		}
	}

	if modeStr != "" {
		mode, err := parseOctalMode(modeStr)
		if err != nil {
			_ = os.Remove(stagingName)
			return fmt.Errorf("parse mode %q: %w", modeStr, err)
		}
		if err := os.Chmod(stagingName, mode); err != nil {
			_ = os.Remove(stagingName)
			return fmt.Errorf("chmod %s -> %o: %w", stagingName, mode, err)
		}
	}

	if err := os.Rename(stagingName, targetPath); err != nil {
		_ = os.Remove(stagingName)
		return fmt.Errorf("rename %s -> %s: %w", stagingName, targetPath, err)
	}
	return nil
}

// mkdirInheritMode mkdirs `dir` (and missing ancestors) using the
// permissions of the nearest existing ancestor. Avoids hardcoding 0755
// because that would widen permissions if a future manifest's
// install_path drops into a 0750 root like /etc/osvbng/ or
// /var/opt/osvbng/. Falls back to 0755 only when no ancestor exists,
// which shouldn't happen on a real image and matches the standard
// /usr/-hierarchy default if it ever does.
func mkdirInheritMode(dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	ancestor := dir
	for {
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			break
		}
		ancestor = parent
		st, err := os.Stat(ancestor)
		if err == nil {
			return os.MkdirAll(dir, st.Mode().Perm())
		}
	}
	return os.MkdirAll(dir, 0o755)
}

// writeStagingFile copies src bytes to dst with fsync. Returns nil on
// success; leaves dst behind on failure for the caller's cleanup path
// to remove.
func writeStagingFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create staging %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("fsync staging %s: %w", dst, err)
	}
	return nil
}

// parseOctalMode parses a manifest mode string ("0755", "755", "0o755")
// into os.FileMode. Accepts the leading-zero, 0o-prefixed, and bare
// forms to match what YAML produces given different operator quoting
// styles.
func parseOctalMode(s string) (os.FileMode, error) {
	if s == "" {
		return 0, errors.New("empty mode string")
	}
	cleaned := s
	if len(cleaned) >= 2 && (cleaned[:2] == "0o" || cleaned[:2] == "0O") {
		cleaned = cleaned[2:]
	}
	mode64, err := strconv.ParseUint(cleaned, 8, 32)
	if err != nil {
		return 0, err
	}
	return os.FileMode(mode64) & os.ModePerm, nil
}
