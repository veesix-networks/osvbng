// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package configmgr

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/vishvananda/netlink"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/netbind"
)

// PostVRFValidator runs after vrfmgr.Reconcile during Commit. Plugins
// register one per binding-capable config struct.
type PostVRFValidator func(cfg *config.Config, vrfMgr netbind.VRFResolver, nl netbind.LinkLister) error

var (
	postVRFValidatorsMu sync.RWMutex
	postVRFValidators   = map[string]PostVRFValidator{}
)

func RegisterPostVRFValidator(name string, fn PostVRFValidator) {
	postVRFValidatorsMu.Lock()
	defer postVRFValidatorsMu.Unlock()
	postVRFValidators[name] = fn
}

// SetVRFManager wires the runtime VRF manager and netlink handle.
// Called from cmd/osvbngd after the routing component constructs vrfmgr.
// Until called, runPostVRFValidators is a no-op.
func (cd *ConfigManager) SetVRFManager(vrfMgr netbind.VRFResolver, nl netbind.LinkLister) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.vrfMgr = vrfMgr
	cd.nlHandle = nl
}

// runPostVRFValidators must be called with cd.mu held by the caller
// (Commit holds the write lock for its full scope).
func (cd *ConfigManager) runPostVRFValidators(cfg *config.Config) error {
	if cd.vrfMgr == nil {
		return nil
	}
	vrfMgr := cd.vrfMgr
	nl := cd.nlHandle

	postVRFValidatorsMu.RLock()
	defer postVRFValidatorsMu.RUnlock()

	names := make([]string, 0, len(postVRFValidators))
	for name := range postVRFValidators {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []string
	for _, name := range names {
		if err := postVRFValidators[name](cfg, vrfMgr, nl); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", name, err.Error()))
		}
	}

	if len(errs) > 0 {
		return errors.New("control-plane VRF binding validation failed:\n  - " + strings.Join(errs, "\n  - "))
	}
	return nil
}

type netlinkHandle struct{ h *netlink.Handle }

func (n *netlinkHandle) LinkList() ([]netlink.Link, error) {
	if n.h == nil {
		return netlink.LinkList()
	}
	return n.h.LinkList()
}

func (n *netlinkHandle) LinkByName(name string) (netlink.Link, error) {
	if n.h == nil {
		return netlink.LinkByName(name)
	}
	return n.h.LinkByName(name)
}

func (n *netlinkHandle) AddrList(link netlink.Link, family int) ([]netlink.Addr, error) {
	if n.h == nil {
		return netlink.AddrList(link, family)
	}
	return n.h.AddrList(link, family)
}

func NetlinkLister(h *netlink.Handle) netbind.LinkLister {
	return &netlinkHandle{h: h}
}
