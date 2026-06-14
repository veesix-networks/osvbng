// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/logger"
)

const (
	defaultUDSPath  = "/run/osvbng/api.sock"
	defaultUDSMode  = "0660"
	defaultUDSGroup = "osvbng"
)

func applyUDSDefaults(cfg *Config) {
	if cfg.UDS.Path == "" {
		cfg.UDS.Path = defaultUDSPath
	}
	if cfg.UDS.Mode == "" {
		cfg.UDS.Mode = defaultUDSMode
	}
	if cfg.UDS.Group == "" {
		cfg.UDS.Group = defaultUDSGroup
	}
}

func listenUDS(cfg UDSConfig, log *logger.Logger) (net.Listener, error) {
	if err := prepareUDSPath(cfg.Path); err != nil {
		return nil, err
	}

	ln, err := net.Listen("unix", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", cfg.Path, err)
	}

	mode, err := parseUDSMode(cfg.Mode)
	if err != nil {
		_ = ln.Close()
		_ = os.Remove(cfg.Path)
		return nil, err
	}
	if err := os.Chmod(cfg.Path, mode); err != nil {
		_ = ln.Close()
		_ = os.Remove(cfg.Path)
		return nil, fmt.Errorf("chmod %s: %w", cfg.Path, err)
	}

	if gid, lookupErr := lookupGID(cfg.Group); lookupErr != nil {
		log.Warn("UDS group lookup failed; leaving socket as root:root",
			"group", cfg.Group, "error", lookupErr)
	} else if err := os.Chown(cfg.Path, 0, gid); err != nil {
		log.Warn("UDS chown failed; leaving socket as root:root",
			"group", cfg.Group, "error", err)
	}

	return ln, nil
}

func prepareUDSPath(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s exists and is not a socket; refusing to overwrite", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale socket %s: %w", path, err)
	}
	return nil
}

func parseUDSMode(s string) (os.FileMode, error) {
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("parse mode %q: %w", s, err)
	}
	return os.FileMode(v), nil
}

func lookupGID(group string) (int, error) {
	g, err := user.LookupGroup(group)
	if err != nil {
		return -1, err
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return -1, err
	}
	return gid, nil
}
